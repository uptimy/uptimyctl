package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var healthchecksCmd = &cobra.Command{
	Use:     "healthchecks",
	Aliases: []string{"hc"},
	Short:   "Manage healthchecks",
}

var healthchecksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all healthchecks",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/healthchecks/", nil)
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

		var healthchecks []map[string]interface{}
		if err := json.Unmarshal(results, &healthchecks); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(healthchecks))
		for _, hc := range healthchecks {
			appName := ""
			if app, ok := hc["application"].(map[string]interface{}); ok {
				appName = str(app["name"])
			}
			active := "No"
			if b, ok := hc["active"].(bool); ok && b {
				active = "Yes"
			}
			rows = append(rows, []string{
				str(hc["uuid"]),
				appName,
				fmt.Sprintf("%vs", str(hc["intervalSeconds"])),
				active,
			})
		}
		output.PrintTable([]string{"UUID", "Application", "Interval", "Active"}, rows)
	},
}

var healthchecksGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get healthcheck details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/healthchecks/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var healthchecksTriggerCmd = &cobra.Command{
	Use:   "trigger <uuid>",
	Short: "Trigger an immediate healthcheck execution",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Patch("/v1/api/healthchecks/"+args[0]+"/trigger", nil)
		if err != nil {
			exitErr(err)
		}
		fmt.Println("Healthcheck triggered.")
	},
}

func init() {
	healthchecksCmd.AddCommand(healthchecksListCmd)
	healthchecksCmd.AddCommand(healthchecksGetCmd)
	healthchecksCmd.AddCommand(healthchecksTriggerCmd)
	rootCmd.AddCommand(healthchecksCmd)
}
