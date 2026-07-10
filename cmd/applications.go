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
	Short:   "Manage applications",
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

		if output.IsJSON() {
			output.PrintJSONBytes(raw)
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

var applicationsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an application",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")

		c := newClient()
		raw, err := c.Post("/v1/api/applications/", map[string]interface{}{
			"name":        name,
			"description": description,
		})
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var applicationsUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update an application",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		body := map[string]interface{}{}
		if cmd.Flags().Changed("name") {
			name, _ := cmd.Flags().GetString("name")
			body["name"] = name
		}
		if cmd.Flags().Changed("description") {
			description, _ := cmd.Flags().GetString("description")
			body["description"] = description
		}
		if len(body) == 0 {
			exitErr(fmt.Errorf("nothing to update: set --name and/or --description"))
		}

		c := newClient()
		raw, err := c.Put("/v1/api/applications/"+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var applicationsDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete an application and everything under it (healthchecks, alert rules)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Delete("/v1/api/applications/" + args[0])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "application", args[0])
	},
}

func init() {
	applicationsCreateCmd.Flags().String("name", "", "Application name (required)")
	applicationsCreateCmd.Flags().String("description", "", "Application description")
	_ = applicationsCreateCmd.MarkFlagRequired("name")

	applicationsUpdateCmd.Flags().String("name", "", "New application name")
	applicationsUpdateCmd.Flags().String("description", "", "New application description")

	applicationsCmd.AddCommand(applicationsListCmd)
	applicationsCmd.AddCommand(applicationsGetCmd)
	applicationsCmd.AddCommand(applicationsCreateCmd)
	applicationsCmd.AddCommand(applicationsUpdateCmd)
	applicationsCmd.AddCommand(applicationsDeleteCmd)
	rootCmd.AddCommand(applicationsCmd)
}
