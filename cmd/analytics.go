package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Read a healthcheck's check history",
	Long: `Read a healthcheck's history from Uptimy's check history service: every
result, as far back as your plan's window.

--from/--to take RFC3339 timestamps or a duration back from now (12h, 7d).

Examples:
  uptimyctl analytics results <hc-uuid> --failures --from 7d
  uptimyctl analytics series <hc-uuid> --from 30d --by-region
  uptimyctl analytics summary <hc-uuid> --by-region --compare
  uptimyctl analytics export <hc-uuid> --from 30d --file results.csv`,
}

// seriesStep picks a step for a range when --step isn't given: minutes up
// to 6 hours, hours up to 30 days, days beyond.
func seriesStep(span time.Duration) string {
	switch {
	case span <= 6*time.Hour:
		return "1m"
	case span <= 30*24*time.Hour:
		return "1h"
	default:
		return "1d"
	}
}

type seriesBucket struct {
	At            string   `json:"at"`
	SchedulerUUID string   `json:"schedulerUuid"`
	Checks        int      `json:"checks"`
	Failures      int      `json:"failures"`
	Uptime        float64  `json:"uptime"`
	LatencyAvgMs  *float64 `json:"latencyAvgMs"`
	LatencyP50Ms  *float64 `json:"latencyP50Ms"`
	LatencyP95Ms  *float64 `json:"latencyP95Ms"`
	LatencyP99Ms  *float64 `json:"latencyP99Ms"`
	Phases        map[string]struct {
		SumUs float64 `json:"sumUs"`
		Count float64 `json:"count"`
	} `json:"phases"`
}

// phaseColumns are the request phases a table shows, in request order.
var phaseColumns = []struct{ key, label string }{
	{"dnsLookupUs", "DNS ms"}, {"tcpConnectionUs", "TCP ms"}, {"tlsHandshakeUs", "TLS ms"},
	{"timeToFirstByteUs", "TTFB ms"}, {"contentTransferUs", "Transfer ms"},
	{"connectionUs", "Connect ms"}, {"rpcCallUs", "RPC ms"}, {"queryTimeUs", "Query ms"},
	{"pingRttUs", "Ping ms"}, {"bannerReadUs", "Banner ms"},
}

var analyticsSeriesCmd = &cobra.Command{
	Use:   "series <healthcheck-uuid>",
	Short: "A healthcheck's checks, uptime and exact percentiles per minute, hour, day or month",
	Long: `Buckets of a window with their checks, failures, uptime, average and exact
p50/p95/p99 response times (scheduled checks). --by-region splits every
bucket by region; --phases adds the average request phases (DNS, TCP, TLS,
TTFB, ...). The step follows the range unless --step says otherwise
(1m up to 6 hours, 1h up to 30 days, then 1d); a series has at most 2,880
buckets. Days and months follow --tz.

Examples:
  uptimyctl analytics series <hc-uuid>                          # last 24 hours, hourly
  uptimyctl analytics series <hc-uuid> --from 2h --by-region --phases
  uptimyctl analytics series <hc-uuid> --from 90d --step 1d --tz Europe/Berlin -o json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		q := windowQuery(cmd)
		now := time.Now()
		if q.Get("from") == "" {
			q.Set("from", now.Add(-24*time.Hour).UTC().Format(time.RFC3339))
		}
		from, _ := time.Parse(time.RFC3339, q.Get("from"))
		to := now
		if v := q.Get("to"); v != "" {
			to, _ = time.Parse(time.RFC3339, v)
		}
		step, _ := cmd.Flags().GetString("step")
		if step == "" {
			step = seriesStep(to.Sub(from))
		}
		q.Set("step", step)
		if tz, _ := cmd.Flags().GetString("tz"); tz != "" {
			q.Set("tz", tz)
		}
		byRegion, _ := cmd.Flags().GetBool("by-region")
		if byRegion {
			q.Set("by", "scheduler")
		}
		phases, _ := cmd.Flags().GetBool("phases")
		if phases {
			q.Set("fields", "phases")
		}

		raw, err := newAnalyticsClient().Get("/v1/api/healthchecks/"+url.PathEscape(args[0])+"/series", q)
		if err != nil {
			exitErr(err)
		}
		data, err := client.ParseDataField(raw)
		if err != nil {
			exitErr(err)
		}
		if output.IsJSON() {
			output.PrintRawJSON(data)
			return
		}
		var s struct {
			Step    string         `json:"step"`
			TZ      string         `json:"tz"`
			Buckets []seriesBucket `json:"buckets"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			exitErr(err)
		}

		// Phase columns: only those some bucket has.
		var shown []struct{ key, label string }
		for _, p := range phaseColumns {
			for _, b := range s.Buckets {
				if _, ok := b.Phases[p.key]; ok {
					shown = append(shown, p)
					break
				}
			}
		}
		headers := []string{"Bucket"}
		var names map[string]string
		if byRegion {
			headers = append(headers, "Region")
			names = regionNames()
		}
		headers = append(headers, "Checks", "Failures", "Uptime", "Avg ms", "p50 ms", "p95 ms", "p99 ms")
		for _, p := range shown {
			headers = append(headers, p.label)
		}
		rows := make([][]string, 0, len(s.Buckets))
		for _, b := range s.Buckets {
			row := []string{b.At}
			if byRegion {
				row = append(row, regionName(names, b.SchedulerUUID))
			}
			uptime := "-"
			if b.Checks > 0 {
				uptime = fmt.Sprintf("%.3f%%", b.Uptime)
			}
			row = append(row, strconv.Itoa(b.Checks), strconv.Itoa(b.Failures), uptime,
				msOrDash(b.LatencyAvgMs), msOrDash(b.LatencyP50Ms), msOrDash(b.LatencyP95Ms), msOrDash(b.LatencyP99Ms))
			for _, p := range shown {
				ph, ok := b.Phases[p.key]
				if !ok || ph.Count == 0 {
					row = append(row, "-")
					continue
				}
				row = append(row, fmt.Sprintf("%.1f", ph.SumUs/ph.Count/1000))
			}
			rows = append(rows, row)
		}
		fmt.Printf("Step %s, %s\n", s.Step, s.TZ)
		output.PrintTable(headers, rows)
	},
}

func init() {
	addWindowFlags(analyticsSeriesCmd, "Range start: RFC3339 or a duration back from now like 12h or 7d (default: 24 hours ago)")
	analyticsSeriesCmd.Flags().String("step", "", "1m, 1h, 1d or 1mo (default: by the range)")
	analyticsSeriesCmd.Flags().Bool("by-region", false, "Split every bucket by region")
	analyticsSeriesCmd.Flags().Bool("phases", false, "Add the average request phases")
	analyticsSeriesCmd.Flags().String("tz", "", "IANA time zone for day and month buckets, e.g. Europe/Berlin (default: UTC)")
	analyticsCmd.AddCommand(analyticsSeriesCmd)
	rootCmd.AddCommand(analyticsCmd)
}
