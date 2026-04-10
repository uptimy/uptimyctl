package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export workspace configuration (applications, healthchecks, tags, alert rules)",
	Long: `Export the entire workspace monitoring configuration as a JSON file.
The output can be saved to a file and later imported into the same or a different workspace.

Examples:
  uptimyctl export                         # Print to stdout
  uptimyctl export -f config.json          # Save to file
  uptimyctl export | jq .                  # Pipe to jq`,
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/export/", nil)
		if err != nil {
			exitErr(err)
		}

		data, err := client.ParseDataField(raw)
		if err != nil {
			exitErr(err)
		}

		pretty, _ := json.MarshalIndent(json.RawMessage(data), "", "  ")

		file, _ := cmd.Flags().GetString("file")
		if file != "" {
			if err := os.WriteFile(file, pretty, 0644); err != nil {
				exitErr(fmt.Errorf("write file: %w", err))
			}
			fmt.Printf("Exported workspace config to %s\n", file)
		} else {
			fmt.Println(string(pretty))
		}
	},
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import workspace configuration from a JSON file",
	Long: `Import applications, healthchecks, tags, and alert rules into the workspace.
The import is idempotent: existing items (matched by name) are skipped.

Examples:
  uptimyctl import config.json
  cat config.json | uptimyctl import -      # Read from stdin`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		var data []byte
		var err error

		if args[0] == "-" {
			data, err = os.ReadFile("/dev/stdin")
		} else {
			data, err = os.ReadFile(args[0])
		}
		if err != nil {
			exitErr(fmt.Errorf("read file: %w", err))
		}

		var config interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			exitErr(fmt.Errorf("invalid JSON: %w", err))
		}

		raw, err := c.Post("/v1/api/import/", config)
		if err != nil {
			exitErr(err)
		}

		result, err := client.ParseDataField(raw)
		if err != nil {
			exitErr(err)
		}

		if output.Format == "json" {
			output.PrintRawJSON(result)
			return
		}

		var counts struct {
			Created struct {
				Tags         int `json:"tags"`
				Applications int `json:"applications"`
				Healthchecks int `json:"healthchecks"`
				AlertRules   int `json:"alertRules"`
			} `json:"created"`
			Skipped struct {
				Tags         int `json:"tags"`
				Applications int `json:"applications"`
			} `json:"skipped"`
		}
		if err := json.Unmarshal(result, &counts); err != nil {
			output.PrintRawJSON(result)
			return
		}

		fmt.Println("Import complete:")
		fmt.Printf("  Created: %d tags, %d applications, %d healthchecks, %d alert rules\n",
			counts.Created.Tags, counts.Created.Applications, counts.Created.Healthchecks, counts.Created.AlertRules)
		fmt.Printf("  Skipped: %d tags, %d applications\n",
			counts.Skipped.Tags, counts.Skipped.Applications)
	},
}

func init() {
	exportCmd.Flags().StringP("file", "f", "", "Output file path (default: stdout)")

	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
