package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var schedulersCmd = &cobra.Command{
	Use:     "schedulers",
	Aliases: []string{"regions"},
	Short:   "List monitoring regions (schedulers)",
}

var schedulersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available schedulers/regions",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/schedulers/", nil)
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

		var schedulers []map[string]interface{}
		if err := json.Unmarshal(results, &schedulers); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(schedulers))
		for _, s := range schedulers {
			premium := "No"
			if b, ok := s["premium"].(bool); ok && b {
				premium = "Yes"
			}
			rows = append(rows, []string{
				str(s["uuid"]),
				str(s["name"]),
				premium,
			})
		}
		output.PrintTable([]string{"UUID", "Name", "Premium"}, rows)
	},
}

var schedulersGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get scheduler details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/schedulers/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

func init() {
	schedulersCmd.AddCommand(schedulersListCmd)
	schedulersCmd.AddCommand(schedulersGetCmd)
	rootCmd.AddCommand(schedulersCmd)
}
