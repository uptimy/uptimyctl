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
}

var maintenancesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List maintenances",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/maintenances/", nil)
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

		var maintenances []map[string]interface{}
		if err := json.Unmarshal(results, &maintenances); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(maintenances))
		for _, m := range maintenances {
			rows = append(rows, []string{
				str(m["uuid"]),
				output.Truncate(str(m["description"]), 40),
				str(m["startAt"]),
				str(m["finishAt"]),
				output.ValueOrDash(str(m["resolvedAt"])),
			})
		}
		output.PrintTable([]string{"UUID", "Description", "Start At", "Finish At", "Resolved At"}, rows)
	},
}

var maintenancesGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get maintenance details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
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
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		startAt, _ := cmd.Flags().GetString("start-at")
		finishAt, _ := cmd.Flags().GetString("finish-at")
		description, _ := cmd.Flags().GetString("description")
		servicesJSON, _ := cmd.Flags().GetString("services")

		var services []interface{}
		if servicesJSON != "" {
			if err := json.Unmarshal([]byte(servicesJSON), &services); err != nil {
				exitErr(fmt.Errorf("invalid --services JSON: %w", err))
			}
		} else {
			services = []interface{}{}
		}

		body := map[string]interface{}{
			"startAt":          startAt,
			"finishAt":         finishAt,
			"description":      description,
			"affectedServices": services,
		}

		raw, err := c.Post("/v1/api/maintenances/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var maintenancesResolveCmd = &cobra.Command{
	Use:   "resolve <uuid>",
	Short: "Resolve (finish) a maintenance window",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()

		resolvedAt, _ := cmd.Flags().GetString("resolved-at")
		if resolvedAt == "" {
			resolvedAt = "now"
		}

		// If "now", use current time
		if resolvedAt == "now" {
			resolvedAt = nowISO()
		}

		body := map[string]interface{}{
			"resolvedAt": resolvedAt,
		}

		raw, err := c.Put("/v1/api/maintenances/"+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var maintenancesDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a maintenance",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Delete("/v1/api/maintenances/" + args[0])
		if err != nil {
			exitErr(err)
		}
		fmt.Println("Maintenance deleted.")
	},
}

func init() {
	// create flags
	maintenancesCreateCmd.Flags().String("start-at", "", "Start time (ISO 8601, required)")
	maintenancesCreateCmd.Flags().String("finish-at", "", "Finish time (ISO 8601, required)")
	maintenancesCreateCmd.Flags().String("description", "", "Description (required)")
	maintenancesCreateCmd.Flags().String("services", "", `Affected services JSON array: '[{"uuid":"...","name":"API","functionality":"Core"}]'`)
	_ = maintenancesCreateCmd.MarkFlagRequired("start-at")
	_ = maintenancesCreateCmd.MarkFlagRequired("finish-at")
	_ = maintenancesCreateCmd.MarkFlagRequired("description")

	// resolve flags
	maintenancesResolveCmd.Flags().String("resolved-at", "now", "Resolution time (ISO 8601 or 'now')")

	maintenancesCmd.AddCommand(maintenancesListCmd)
	maintenancesCmd.AddCommand(maintenancesGetCmd)
	maintenancesCmd.AddCommand(maintenancesCreateCmd)
	maintenancesCmd.AddCommand(maintenancesResolveCmd)
	maintenancesCmd.AddCommand(maintenancesDeleteCmd)
	rootCmd.AddCommand(maintenancesCmd)
}
