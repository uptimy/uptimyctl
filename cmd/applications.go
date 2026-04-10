package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var applicationsCmd = &cobra.Command{
	Use:     "applications",
	Aliases: []string{"apps"},
	Short:   "List and inspect applications",
}

var applicationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all applications",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/applications/", nil)
		if err != nil {
			exitErr(err)
		}

		if output.Format == "json" {
			fmt.Println(string(raw))
			return
		}

		results, err := client.ParseResultsField(raw)
		if err != nil {
			exitErr(err)
		}

		var apps []map[string]interface{}
		if err := json.Unmarshal(results, &apps); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(apps))
		for _, app := range apps {
			rows = append(rows, []string{
				str(app["uuid"]),
				str(app["name"]),
				str(app["status"]),
				output.Truncate(str(app["description"]), 30),
			})
		}
		output.PrintTable([]string{"UUID", "Name", "Status", "Description"}, rows)
	},
}

var applicationsGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get application details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/applications/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

func init() {
	applicationsCmd.AddCommand(applicationsListCmd)
	applicationsCmd.AddCommand(applicationsGetCmd)
	rootCmd.AddCommand(applicationsCmd)
}
