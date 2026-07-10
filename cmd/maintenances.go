package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var maintenancesCmd = &cobra.Command{
	Use:     "maintenances",
	Aliases: []string{"maint"},
	Short:   "Manage scheduled maintenances",
	Long: `Manage scheduled maintenance windows.

Maintenance lifecycle: Scheduled -> In Progress -> Completed (or Cancelled).

A maintenance becomes visible on a status page once it is attached to it
with --status-page on create or update.`,
}

func parseServicesFlag(cmd *cobra.Command) ([]interface{}, bool) {
	servicesJSON, _ := cmd.Flags().GetString("services")
	if !cmd.Flags().Changed("services") {
		return nil, false
	}
	var services []interface{}
	if servicesJSON != "" {
		if err := json.Unmarshal([]byte(servicesJSON), &services); err != nil {
			exitErr(fmt.Errorf("invalid --services JSON: %w", err))
		}
	} else {
		services = []interface{}{}
	}
	return services, true
}

func putMaintenance(c *client.Client, uuid string, body map[string]interface{}) {
	raw, err := c.Put("/v1/api/maintenances/"+uuid, body)
	if err != nil {
		exitErr(err)
	}
	data, _ := client.ParseDataField(raw)
	output.PrintRawJSON(data)
}

var maintenancesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List maintenances",
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		raw, err := c.Get("/v1/api/maintenances/", nil)
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

		var maintenances []map[string]interface{}
		if err := json.Unmarshal(results, &maintenances); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(maintenances))
		for _, m := range maintenances {
			title := str(m["title"])
			if title == "" {
				title = str(m["description"])
			}
			rows = append(rows, []string{
				str(m["uuid"]),
				output.Truncate(title, 40),
				output.ValueOrDash(str(m["status"])),
				str(m["startAt"]),
				str(m["finishAt"]),
			})
		}
		output.PrintTable([]string{"UUID", "Title", "Status", "Start At", "Finish At"}, rows)
	},
}

var maintenancesGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get maintenance details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		raw, err := c.Get("/v1/api/maintenances/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var maintenancesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Schedule a new maintenance window",
	Long: `Schedule a new maintenance window.

Attach it to one or more status pages with --status-page to make it
publicly visible.

Example:
  uptimyctl maintenances create --title "Database upgrade" \
    --description "Primary DB failover, expect brief write errors" \
    --start-at 2026-07-12T02:00:00Z --finish-at 2026-07-12T03:00:00Z \
    --services '[{"uuid":"<app-uuid>","name":"API","functionality":"Core"}]' \
    --status-page <status-page-uuid> -o json`,
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		title, _ := cmd.Flags().GetString("title")
		startAt, _ := cmd.Flags().GetString("start-at")
		finishAt, _ := cmd.Flags().GetString("finish-at")
		description, _ := cmd.Flags().GetString("description")
		statusPages, _ := cmd.Flags().GetStringSlice("status-page")

		services, ok := parseServicesFlag(cmd)
		if !ok {
			services = []interface{}{}
		}

		body := map[string]interface{}{
			"startAt":          startAt,
			"finishAt":         finishAt,
			"description":      description,
			"affectedServices": services,
		}
		if title != "" {
			body["title"] = title
		}
		if cmd.Flags().Changed("status-page") {
			body["statusPageUuids"] = statusPages
		}

		raw, err := c.Post("/v1/api/maintenances/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var maintenancesUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update a maintenance (only the flags you pass are changed)",
	Long: `Update a maintenance window. Only the flags you pass are changed;
everything else keeps its current value.

--status-page replaces the full set of status pages the maintenance is
shown on.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		body := map[string]interface{}{}
		for flag, field := range map[string]string{
			"title":       "title",
			"description": "description",
			"status":      "status",
			"start-at":    "startAt",
			"finish-at":   "finishAt",
		} {
			if cmd.Flags().Changed(flag) {
				body[field], _ = cmd.Flags().GetString(flag)
			}
		}
		if services, ok := parseServicesFlag(cmd); ok {
			body["affectedServices"] = services
		}
		if cmd.Flags().Changed("status-page") {
			pages, _ := cmd.Flags().GetStringSlice("status-page")
			body["statusPageUuids"] = pages
		}

		if len(body) == 0 {
			exitErr(fmt.Errorf("nothing to update: pass at least one flag"))
		}

		putMaintenance(c, args[0], body)
	},
}

var maintenancesStartCmd = &cobra.Command{
	Use:   "start <uuid>",
	Short: "Mark a maintenance as In Progress",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		putMaintenance(c, args[0], map[string]interface{}{"status": "In Progress"})
	},
}

var maintenancesResolveCmd = &cobra.Command{
	Use:     "resolve <uuid>",
	Aliases: []string{"complete"},
	Short:   "Mark a maintenance as Completed",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		putMaintenance(c, args[0], map[string]interface{}{"status": "Completed"})
	},
}

var maintenancesCancelCmd = &cobra.Command{
	Use:   "cancel <uuid>",
	Short: "Mark a maintenance as Cancelled",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		putMaintenance(c, args[0], map[string]interface{}{"status": "Cancelled"})
	},
}

var maintenancesDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a maintenance",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		_, err := c.Delete("/v1/api/maintenances/" + args[0])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "maintenance", args[0])
	},
}

func init() {
	// create flags
	maintenancesCreateCmd.Flags().String("title", "", "Maintenance title")
	maintenancesCreateCmd.Flags().String("start-at", "", "Start time (ISO 8601, required)")
	maintenancesCreateCmd.Flags().String("finish-at", "", "Finish time (ISO 8601, required)")
	maintenancesCreateCmd.Flags().String("description", "", "Description (required)")
	maintenancesCreateCmd.Flags().String("services", "", `Affected services JSON array: '[{"uuid":"...","name":"API","functionality":"Core"}]'`)
	maintenancesCreateCmd.Flags().StringSlice("status-page", nil, "Status page UUID to publish on (repeatable)")
	_ = maintenancesCreateCmd.MarkFlagRequired("start-at")
	_ = maintenancesCreateCmd.MarkFlagRequired("finish-at")
	_ = maintenancesCreateCmd.MarkFlagRequired("description")

	// update flags
	maintenancesUpdateCmd.Flags().String("title", "", "Maintenance title")
	maintenancesUpdateCmd.Flags().String("description", "", "Description")
	maintenancesUpdateCmd.Flags().String("status", "", "Status: Scheduled, In Progress, Completed, Cancelled")
	maintenancesUpdateCmd.Flags().String("start-at", "", "Start time (ISO 8601)")
	maintenancesUpdateCmd.Flags().String("finish-at", "", "Finish time (ISO 8601)")
	maintenancesUpdateCmd.Flags().String("services", "", `Affected services JSON array: '[{"uuid":"...","name":"API","functionality":"Core"}]'`)
	maintenancesUpdateCmd.Flags().StringSlice("status-page", nil, "Replace the set of status pages (repeatable)")

	maintenancesCmd.AddCommand(maintenancesListCmd)
	maintenancesCmd.AddCommand(maintenancesGetCmd)
	maintenancesCmd.AddCommand(maintenancesCreateCmd)
	maintenancesCmd.AddCommand(maintenancesUpdateCmd)
	maintenancesCmd.AddCommand(maintenancesStartCmd)
	maintenancesCmd.AddCommand(maintenancesResolveCmd)
	maintenancesCmd.AddCommand(maintenancesCancelCmd)
	maintenancesCmd.AddCommand(maintenancesDeleteCmd)
	rootCmd.AddCommand(maintenancesCmd)
}
