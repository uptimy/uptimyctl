package cmd

import (
	"net/url"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Read healthcheck analytics",
	Long: `Read execution history and rollup analytics for a healthcheck.

All subcommands take a healthcheck UUID and an optional time range.
When --from/--to are omitted, the last 24 hours are returned.

Examples:
  uptimyctl analytics executions <hc-uuid>
  uptimyctl analytics hour-region <hc-uuid> --from 2026-07-01T00:00:00Z --to 2026-07-08T00:00:00Z
  uptimyctl analytics minute-region <hc-uuid> --scheduler <scheduler-uuid> -o json`,
}

// newAnalyticsCmd builds one subcommand per analytics endpoint; they differ only in path suffix.
func newAnalyticsCmd(use, short, endpoint string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use + " <healthcheck-uuid>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			scheduler, _ := cmd.Flags().GetString("scheduler")

			if to == "" {
				to = nowISO()
			}
			if from == "" {
				from = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
			}

			q := url.Values{}
			q.Set("from", from)
			q.Set("to", to)
			if scheduler != "" {
				q.Set("schedulerUuid", scheduler)
			}

			c := newClient()
			raw, err := c.Get("/v1/api/healthchecks/"+args[0]+"/analytics/"+endpoint, q)
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
			output.PrintRawJSON(results)
		},
	}

	cmd.Flags().String("from", "", "Range start, RFC3339 (default: 24 hours ago)")
	cmd.Flags().String("to", "", "Range end, RFC3339 (default: now)")
	cmd.Flags().String("scheduler", "", "Filter by scheduler UUID")
	return cmd
}

func init() {
	analyticsCmd.AddCommand(newAnalyticsCmd("executions", "Individual healthcheck executions", "executions"))
	analyticsCmd.AddCommand(newAnalyticsCmd("minute-region", "Per-minute rollups by region", "minute-region"))
	analyticsCmd.AddCommand(newAnalyticsCmd("minute-breakdown", "Per-minute latency breakdown rollups", "minute-breakdown"))
	analyticsCmd.AddCommand(newAnalyticsCmd("hour-region", "Hourly rollups by region", "hour-region"))
	analyticsCmd.AddCommand(newAnalyticsCmd("daily-region", "Daily rollups by region", "daily-region"))
	analyticsCmd.AddCommand(newAnalyticsCmd("monthly-region", "Monthly rollups by region", "monthly-region"))
	rootCmd.AddCommand(analyticsCmd)
}
