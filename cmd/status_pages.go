package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var statusPagesCmd = &cobra.Command{
	Use:     "status-pages",
	Aliases: []string{"sp"},
	Short:   "Manage status pages",
}

var statusPagesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all status pages",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/status-pages/", nil)
		if err != nil {
			exitErr(err)
		}

		if output.IsJSON() {
			output.PrintJSONBytes(raw)
			return
		}

		data, err := client.ParseDataField(raw)
		if err != nil {
			exitErr(err)
		}

		var pages []map[string]interface{}
		if err := json.Unmarshal(data, &pages); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(pages))
		for _, page := range pages {
			public := "No"
			if b, ok := page["public"].(bool); ok && b {
				public = "Yes"
			}
			rows = append(rows, []string{
				str(page["uuid"]),
				output.Truncate(str(page["name"]), 30),
				output.ValueOrDash(str(page["slug"])),
				public,
			})
		}
		output.PrintTable([]string{"UUID", "Name", "Slug", "Public"}, rows)
	},
}

var statusPagesGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get status page details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/status-pages/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPagesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a status page from a JSON spec",
	Long: `Create a status page from a JSON spec.

The spec matches the API's status page payload:
  {
    "name": "My Status Page",
    "public": true,
    "whiteLabel": false,
    "theme": {},
    "branding": {},
    "componentsVisibility": {},
    "enableLiveUpdates": true
  }

Examples:
  uptimyctl status-pages create -f page.json
  echo '{...}' | uptimyctl status-pages create -f -`,
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Post("/v1/api/status-pages/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPagesUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update a status page from a JSON spec",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Put("/v1/api/status-pages/"+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPagesDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a status page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Delete("/v1/api/status-pages/" + args[0])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "status page", args[0])
	},
}

// --- Groups ---

var statusPageGroupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Manage status page groups",
}

func statusPageGroupsPath(statusPageUUID string) string {
	return "/v1/api/status-pages/" + statusPageUUID + "/groups/"
}

var statusPageGroupsListCmd = &cobra.Command{
	Use:   "list <status-page-uuid>",
	Short: "List groups on a status page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get(statusPageGroupsPath(args[0]), nil)
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

		var groups []map[string]interface{}
		if err := json.Unmarshal(results, &groups); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(groups))
		for _, group := range groups {
			hidden := "No"
			if b, ok := group["hidden"].(bool); ok && b {
				hidden = "Yes"
			}
			rows = append(rows, []string{
				str(group["uuid"]),
				output.Truncate(str(group["label"]), 30),
				str(group["order"]),
				hidden,
			})
		}
		output.PrintTable([]string{"UUID", "Label", "Order", "Hidden"}, rows)
	},
}

var statusPageGroupsGetCmd = &cobra.Command{
	Use:   "get <status-page-uuid> <group-uuid>",
	Short: "Get status page group details",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get(statusPageGroupsPath(args[0])+args[1], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPageGroupsCreateCmd = &cobra.Command{
	Use:   "create <status-page-uuid>",
	Short: "Create a status page group from a JSON spec",
	Long: `Create a status page group from a JSON spec.

The spec matches the API's group payload:
  {
    "label": "Core services",
    "hidden": false,
    "order": 0,
    "healthchecks": [
      { "applicationUuid": "<app-uuid>", "order": 0, "showDailies": true, "isVisible": true }
    ]
  }

Examples:
  uptimyctl status-pages groups create <status-page-uuid> -f group.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Post(statusPageGroupsPath(args[0]), body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPageGroupsUpdateCmd = &cobra.Command{
	Use:   "update <status-page-uuid> <group-uuid>",
	Short: "Update a status page group from a JSON spec",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Put(statusPageGroupsPath(args[0])+args[1], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var statusPageGroupsDeleteCmd = &cobra.Command{
	Use:   "delete <status-page-uuid> <group-uuid>",
	Short: "Delete a status page group",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Delete(statusPageGroupsPath(args[0]) + args[1])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "status page group", args[1])
	},
}

func init() {
	statusPagesCreateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = statusPagesCreateCmd.MarkFlagRequired("file")
	statusPagesUpdateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = statusPagesUpdateCmd.MarkFlagRequired("file")
	statusPageGroupsCreateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = statusPageGroupsCreateCmd.MarkFlagRequired("file")
	statusPageGroupsUpdateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = statusPageGroupsUpdateCmd.MarkFlagRequired("file")

	statusPageGroupsCmd.AddCommand(statusPageGroupsListCmd)
	statusPageGroupsCmd.AddCommand(statusPageGroupsGetCmd)
	statusPageGroupsCmd.AddCommand(statusPageGroupsCreateCmd)
	statusPageGroupsCmd.AddCommand(statusPageGroupsUpdateCmd)
	statusPageGroupsCmd.AddCommand(statusPageGroupsDeleteCmd)

	statusPagesCmd.AddCommand(statusPagesListCmd)
	statusPagesCmd.AddCommand(statusPagesGetCmd)
	statusPagesCmd.AddCommand(statusPagesCreateCmd)
	statusPagesCmd.AddCommand(statusPagesUpdateCmd)
	statusPagesCmd.AddCommand(statusPagesDeleteCmd)
	statusPagesCmd.AddCommand(statusPageGroupsCmd)
	rootCmd.AddCommand(statusPagesCmd)
}
