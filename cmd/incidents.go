package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Manage incidents",
}

var incidentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List incidents",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		q := url.Values{}
		if v, _ := cmd.Flags().GetString("status"); v != "" {
			q.Set("status", v)
		}
		if v, _ := cmd.Flags().GetString("severity"); v != "" {
			q.Set("severity", v)
		}
		if v, _ := cmd.Flags().GetString("application"); v != "" {
			q.Set("applicationUuid", v)
		}
		if v, _ := cmd.Flags().GetInt("page"); v > 0 {
			q.Set("page", fmt.Sprintf("%d", v))
		}
		if v, _ := cmd.Flags().GetInt("per-page"); v > 0 {
			q.Set("perPage", fmt.Sprintf("%d", v))
		}

		raw, err := c.Get("/v1/api/incidents/", q)
		if err != nil {
			exitErr(err)
		}

		if output.Format == "json" {
			data, _ := json.RawMessage(raw).MarshalJSON()
			fmt.Println(string(data))
			return
		}

		results, err := client.ParseResultsField(raw)
		if err != nil {
			exitErr(err)
		}

		var incidents []map[string]interface{}
		if err := json.Unmarshal(results, &incidents); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(incidents))
		for _, inc := range incidents {
			rows = append(rows, []string{
				str(inc["uuid"]),
				output.Truncate(str(inc["title"]), 40),
				str(inc["status"]),
				str(inc["severity"]),
				output.ValueOrDash(str(inc["resolvedAt"])),
			})
		}
		output.PrintTable([]string{"UUID", "Title", "Status", "Severity", "Resolved At"}, rows)
	},
}

var incidentsGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get incident details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/incidents/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Get incident statistics",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/incidents/stats", nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new incident",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		severity, _ := cmd.Flags().GetString("severity")
		public, _ := cmd.Flags().GetBool("public")
		applicationUuid, _ := cmd.Flags().GetString("application")

		body := map[string]interface{}{
			"title":    title,
			"public":   public,
			"severity": severity,
		}
		if description != "" {
			body["description"] = description
		}
		if applicationUuid != "" {
			body["applicationUuid"] = applicationUuid
		}

		raw, err := c.Post("/v1/api/incidents/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update an incident",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		severity, _ := cmd.Flags().GetString("severity")
		status, _ := cmd.Flags().GetString("status")
		public, _ := cmd.Flags().GetBool("public")

		body := map[string]interface{}{
			"title":    title,
			"public":   public,
			"severity": severity,
			"status":   status,
		}
		if description != "" {
			body["description"] = description
		}

		raw, err := c.Put("/v1/api/incidents/"+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsResolveCmd = &cobra.Command{
	Use:   "resolve <uuid>",
	Short: "Resolve an incident (shortcut for update --status Resolved)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		// First fetch the current incident to get required fields
		raw, err := c.Get("/v1/api/incidents/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		var inc map[string]interface{}
		if err := json.Unmarshal(data, &inc); err != nil {
			exitErr(err)
		}

		body := map[string]interface{}{
			"title":    inc["title"],
			"public":   inc["public"],
			"severity": inc["severity"],
			"status":   "Resolved",
		}
		if desc, ok := inc["description"]; ok && desc != nil {
			body["description"] = desc
		}

		raw, err = c.Put("/v1/api/incidents/"+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ = client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsAddUpdateCmd = &cobra.Command{
	Use:   "add-update <incident-uuid>",
	Short: "Add an update to an incident",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		message, _ := cmd.Flags().GetString("message")
		public, _ := cmd.Flags().GetBool("public")

		body := map[string]interface{}{
			"message": message,
			"public":  public,
		}

		raw, err := c.Post("/v1/api/incidents/"+args[0]+"/updates/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

func init() {
	// list flags
	incidentsListCmd.Flags().String("status", "", "Filter: Ongoing, Investigating, Resolved")
	incidentsListCmd.Flags().String("severity", "", "Filter: critical, high, medium, low")
	incidentsListCmd.Flags().String("application", "", "Filter by application UUID")
	incidentsListCmd.Flags().Int("page", 1, "Page number")
	incidentsListCmd.Flags().Int("per-page", 20, "Results per page")

	// create flags
	incidentsCreateCmd.Flags().String("title", "", "Incident title (required)")
	incidentsCreateCmd.Flags().String("description", "", "Incident description")
	incidentsCreateCmd.Flags().String("severity", "high", "Severity: critical, high, medium, low")
	incidentsCreateCmd.Flags().Bool("public", true, "Whether the incident is public")
	incidentsCreateCmd.Flags().String("application", "", "Application UUID to associate with")
	_ = incidentsCreateCmd.MarkFlagRequired("title")

	// update flags
	incidentsUpdateCmd.Flags().String("title", "", "Incident title (required)")
	incidentsUpdateCmd.Flags().String("description", "", "Incident description")
	incidentsUpdateCmd.Flags().String("severity", "high", "Severity: critical, high, medium, low")
	incidentsUpdateCmd.Flags().String("status", "Ongoing", "Status: Ongoing, Investigating, Identified, Monitoring, Resolved")
	incidentsUpdateCmd.Flags().Bool("public", true, "Whether the incident is public")
	_ = incidentsUpdateCmd.MarkFlagRequired("title")
	_ = incidentsUpdateCmd.MarkFlagRequired("status")

	// add-update flags
	incidentsAddUpdateCmd.Flags().String("message", "", "Update message (required)")
	incidentsAddUpdateCmd.Flags().Bool("public", true, "Whether the update is public")
	_ = incidentsAddUpdateCmd.MarkFlagRequired("message")

	incidentsCmd.AddCommand(incidentsListCmd)
	incidentsCmd.AddCommand(incidentsGetCmd)
	incidentsCmd.AddCommand(incidentsStatsCmd)
	incidentsCmd.AddCommand(incidentsCreateCmd)
	incidentsCmd.AddCommand(incidentsUpdateCmd)
	incidentsCmd.AddCommand(incidentsResolveCmd)
	incidentsCmd.AddCommand(incidentsAddUpdateCmd)
	rootCmd.AddCommand(incidentsCmd)
}

// helper to safely convert interface{} to string
func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
