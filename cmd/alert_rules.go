package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var alertRulesCmd = &cobra.Command{
	Use:     "alert-rules",
	Aliases: []string{"ar"},
	Short:   "Manage application alert rules",
}

func alertRulesPath(appUUID string) string {
	return "/v1/api/applications/" + appUUID + "/alert-rules/"
}

var alertRulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List alert rules for an application",
	Run: func(cmd *cobra.Command, args []string) {
		appUUID, _ := cmd.Flags().GetString("application")

		c := newClient()
		raw, err := c.Get(alertRulesPath(appUUID), nil)
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

		var rules []map[string]interface{}
		if err := json.Unmarshal(results, &rules); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(rules))
		for _, rule := range rules {
			rows = append(rows, []string{
				str(rule["uuid"]),
				output.Truncate(str(rule["name"]), 30),
				str(rule["alertRuleType"]),
				str(rule["severity"]),
			})
		}
		output.PrintTable([]string{"UUID", "Name", "Type", "Severity"}, rows)
	},
}

var alertRulesGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get alert rule details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appUUID, _ := cmd.Flags().GetString("application")

		c := newClient()
		raw, err := c.Get(alertRulesPath(appUUID)+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var alertRulesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an alert rule for an application",
	Long: `Create an alert rule from a JSON spec.

The spec matches the API's alert rule payload:
  {
    "name": "High latency",
    "thresholdFailures": 3,
    "timeWindowMinutes": 5,
    "latencyThresholdMS": 2000,
    "slaTargetPercentage": 99,
    "alertRuleType": "latency",
    "severity": "warning"
  }

Examples:
  uptimyctl alert-rules create --application <app-uuid> -f rule.json
  echo '{...}' | uptimyctl alert-rules create --application <app-uuid> -f -`,
	Run: func(cmd *cobra.Command, args []string) {
		appUUID, _ := cmd.Flags().GetString("application")
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Post(alertRulesPath(appUUID), body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var alertRulesUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update an alert rule from a JSON spec",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appUUID, _ := cmd.Flags().GetString("application")
		file, _ := cmd.Flags().GetString("file")

		body, err := readJSONInput(file)
		if err != nil {
			exitErr(err)
		}

		c := newClient()
		raw, err := c.Put(alertRulesPath(appUUID)+args[0], body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var alertRulesDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete an alert rule",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appUUID, _ := cmd.Flags().GetString("application")

		c := newClient()
		_, err := c.Delete(alertRulesPath(appUUID) + args[0])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "alert rule", args[0])
	},
}

func init() {
	alertRulesCmd.PersistentFlags().String("application", "", "Application UUID (required)")
	_ = alertRulesCmd.MarkPersistentFlagRequired("application")

	alertRulesCreateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = alertRulesCreateCmd.MarkFlagRequired("file")
	alertRulesUpdateCmd.Flags().StringP("file", "f", "", "JSON spec file, or - for stdin (required)")
	_ = alertRulesUpdateCmd.MarkFlagRequired("file")

	alertRulesCmd.AddCommand(alertRulesListCmd)
	alertRulesCmd.AddCommand(alertRulesGetCmd)
	alertRulesCmd.AddCommand(alertRulesCreateCmd)
	alertRulesCmd.AddCommand(alertRulesUpdateCmd)
	alertRulesCmd.AddCommand(alertRulesDeleteCmd)
	rootCmd.AddCommand(alertRulesCmd)
}
