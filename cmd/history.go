package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

// Check history (results, summaries, exports) comes from Uptimy's analytics
// service, as far back as the plan's history window: --analytics-api-url or
// UPTIMYCTL_ANALYTICS_API_URL to override.

// exportTimeout is how long an export may take: up to 31 days of results.
const exportTimeout = 5 * time.Minute

// parseSince reads a time flag: RFC3339, or a duration back from now like
// 30m, 12h or 7d.
func parseSince(v string, now time.Time) (string, error) {
	if v == "" {
		return "", nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if days, ok := strings.CutSuffix(v, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil && n > 0 {
			return now.Add(-time.Duration(n) * 24 * time.Hour).UTC().Format(time.RFC3339), nil
		}
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return now.Add(-d).UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("%q is neither RFC3339 nor a duration like 30m, 12h or 7d", v)
}

// windowQuery is --from and --to as query parameters (omitted when unset:
// the service's defaults apply).
func windowQuery(cmd *cobra.Command) url.Values {
	now := time.Now()
	q := url.Values{}
	for _, name := range []string{"from", "to"} {
		v, _ := cmd.Flags().GetString(name)
		t, err := parseSince(v, now)
		if err != nil {
			exitErr(fmt.Errorf("--%s: %w", name, err))
		}
		if t != "" {
			q.Set(name, t)
		}
	}
	return q
}

// regionNames maps region (scheduler) UUIDs to names; a lookup that fails
// leaves the UUIDs.
func regionNames() map[string]string {
	names := map[string]string{}
	raw, err := newClient().Get("/v1/api/schedulers/", nil)
	if err != nil {
		return names
	}
	results, err := client.ParseResultsField(raw)
	if err != nil {
		return names
	}
	var schedulers []struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	if json.Unmarshal(results, &schedulers) == nil {
		for _, s := range schedulers {
			names[s.UUID] = s.Name
		}
	}
	return names
}

func regionName(names map[string]string, uuid string) string {
	if n := names[uuid]; n != "" {
		return n
	}
	return uuid
}

func msOrDash(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f", *v)
}

type historyResult struct {
	CheckedAt      string   `json:"checkedAt"`
	SchedulerUUID  string   `json:"schedulerUuid"`
	Trigger        string   `json:"trigger"`
	Success        bool     `json:"success"`
	ResponseTimeMs *float64 `json:"responseTimeMs"`
	StatusCode     *int     `json:"statusCode"`
	ErrorClass     *string  `json:"errorClass"`
	ErrorMessage   *string  `json:"errorMessage"`
}

type resultsPage struct {
	Results    []json.RawMessage `json:"results"`
	NextCursor *string           `json:"nextCursor"`
}

var analyticsResultsCmd = &cobra.Command{
	Use:   "results <healthcheck-uuid>",
	Short: "A healthcheck's check results, newest first",
	Long: `Every check run (scheduled, confirmation rechecks and manual runs), newest
first, with its status code, error and response time. History goes back as
far as your plan's window.

Examples:
  uptimyctl analytics results <hc-uuid>
  uptimyctl analytics results <hc-uuid> --failures --from 7d
  uptimyctl analytics results <hc-uuid> --region <scheduler-uuid> --all -o json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		q := windowQuery(cmd)
		if failures, _ := cmd.Flags().GetBool("failures"); failures {
			q.Set("outcome", "failure")
		}
		if region, _ := cmd.Flags().GetString("region"); region != "" {
			q.Set("scheduler", region)
		}
		limit, _ := cmd.Flags().GetInt("limit")
		q.Set("limit", strconv.Itoa(limit))
		all, _ := cmd.Flags().GetBool("all")

		c := newAnalyticsClient()
		path := "/v1/api/healthchecks/" + url.PathEscape(args[0]) + "/results"
		var rows []json.RawMessage
		var next *string
		for {
			raw, err := c.Get(path, q)
			if err != nil {
				exitErr(err)
			}
			data, err := client.ParseDataField(raw)
			if err != nil {
				exitErr(err)
			}
			var page resultsPage
			if err := json.Unmarshal(data, &page); err != nil {
				exitErr(err)
			}
			rows, next = append(rows, page.Results...), page.NextCursor
			if !all || next == nil {
				break
			}
			q.Set("cursor", *next)
		}

		if output.IsJSON() {
			output.PrintJSON(map[string]interface{}{"results": rows, "nextCursor": next})
			return
		}
		names := regionNames()
		table := make([][]string, 0, len(rows))
		for _, raw := range rows {
			var r historyResult
			if err := json.Unmarshal(raw, &r); err != nil {
				exitErr(err)
			}
			ok, status, errText := "yes", "-", ""
			if !r.Success {
				ok = "NO"
			}
			if r.StatusCode != nil {
				status = strconv.Itoa(*r.StatusCode)
			}
			if r.ErrorMessage != nil {
				errText = *r.ErrorMessage
			} else if r.ErrorClass != nil {
				errText = *r.ErrorClass
			}
			table = append(table, []string{r.CheckedAt, regionName(names, r.SchedulerUUID), r.Trigger, ok, msOrDash(r.ResponseTimeMs), status, output.Truncate(errText, 60)})
		}
		output.PrintTable([]string{"Checked at", "Region", "Trigger", "OK", "ms", "Status", "Error"}, table)
		if next != nil {
			fmt.Fprintln(os.Stderr, "More results: --all, or narrow --from/--to.")
		}
	},
}

type windowFigures struct {
	Checks       int      `json:"checks"`
	Failures     int      `json:"failures"`
	Uptime       float64  `json:"uptime"`
	LatencyP50Ms *float64 `json:"latencyP50Ms"`
	LatencyP95Ms *float64 `json:"latencyP95Ms"`
	LatencyP99Ms *float64 `json:"latencyP99Ms"`
}

type historySummary struct {
	From string `json:"from"`
	To   string `json:"to"`
	windowFigures
	LastFailureAt *string `json:"lastFailureAt"`
	FailureCauses []struct {
		ErrorClass   *string `json:"errorClass"`
		StatusCode   *int    `json:"statusCode"`
		Failures     int     `json:"failures"`
		Regions      int     `json:"regions"`
		LastAt       string  `json:"lastAt"`
		ErrorMessage *string `json:"errorMessage"`
	} `json:"failureCauses"`
	Regions []struct {
		SchedulerUUID string `json:"schedulerUuid"`
		windowFigures
		LastFailureAt *string `json:"lastFailureAt"`
	} `json:"regions"`
	Previous *struct {
		From string `json:"from"`
		To   string `json:"to"`
		windowFigures
	} `json:"previous"`
}

func uptimeOrDash(f windowFigures) string {
	if f.Checks == 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f%%", f.Uptime)
}

var analyticsSummaryCmd = &cobra.Command{
	Use:   "summary <healthcheck-uuid>",
	Short: "A healthcheck's window: uptime, exact percentiles, failure causes, per region",
	Long: `Uptime, exact p50/p95/p99 and the failure causes of a window (default: the
last 24 hours), counting scheduled checks. --by-region compares the regions
side by side; --compare adds the window of the same length before it.

Examples:
  uptimyctl analytics summary <hc-uuid> --by-region
  uptimyctl analytics summary <hc-uuid> --from 7d --compare -o json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		q := windowQuery(cmd)
		if by, _ := cmd.Flags().GetBool("by-region"); by {
			q.Set("by", "scheduler")
		}
		if cmp, _ := cmd.Flags().GetBool("compare"); cmp {
			q.Set("compare", "previous")
		}
		raw, err := newAnalyticsClient().Get("/v1/api/healthchecks/"+url.PathEscape(args[0])+"/summary", q)
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
		var s historySummary
		if err := json.Unmarshal(data, &s); err != nil {
			exitErr(err)
		}
		names := regionNames()
		rows := [][]string{{"All regions", strconv.Itoa(s.Checks), strconv.Itoa(s.Failures), uptimeOrDash(s.windowFigures),
			msOrDash(s.LatencyP50Ms), msOrDash(s.LatencyP95Ms), msOrDash(s.LatencyP99Ms), output.ValueOrDash(deref(s.LastFailureAt))}}
		for _, r := range s.Regions {
			rows = append(rows, []string{regionName(names, r.SchedulerUUID), strconv.Itoa(r.Checks), strconv.Itoa(r.Failures), uptimeOrDash(r.windowFigures),
				msOrDash(r.LatencyP50Ms), msOrDash(r.LatencyP95Ms), msOrDash(r.LatencyP99Ms), output.ValueOrDash(deref(r.LastFailureAt))})
		}
		fmt.Printf("%s to %s\n", s.From, s.To)
		output.PrintTable([]string{"Region", "Checks", "Failures", "Uptime", "p50 ms", "p95 ms", "p99 ms", "Last failure"}, rows)
		if p := s.Previous; p != nil {
			fmt.Printf("\nBefore (%s to %s): %d checks, uptime %s, p95 %s ms\n", p.From, p.To, p.Checks, uptimeOrDash(p.windowFigures), msOrDash(p.LatencyP95Ms))
		}
		if len(s.FailureCauses) > 0 {
			causes := make([][]string, 0, len(s.FailureCauses))
			for _, c := range s.FailureCauses {
				name := deref(c.ErrorClass)
				if c.StatusCode != nil {
					name = "HTTP " + strconv.Itoa(*c.StatusCode)
				}
				causes = append(causes, []string{name, strconv.Itoa(c.Failures), strconv.Itoa(c.Regions), c.LastAt, output.Truncate(deref(c.ErrorMessage), 50)})
			}
			fmt.Println()
			output.PrintTable([]string{"Failure", "Count", "Regions", "Last seen", "Example"}, causes)
		}
	},
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// runExport downloads an export (at most 31 days per file) to --file or
// stdout.
func runExport(cmd *cobra.Command, path string) {
	q := windowQuery(cmd)
	format, _ := cmd.Flags().GetString("format")
	q.Set("format", format)
	if failures, _ := cmd.Flags().GetBool("failures"); failures {
		q.Set("outcome", "failure")
	}
	c := newAnalyticsClient()
	c.HTTPClient.Timeout = exportTimeout
	raw, err := c.Get(path, q)
	if err != nil {
		exitErr(err)
	}
	file, _ := cmd.Flags().GetString("file")
	var w io.Writer = os.Stdout
	if file != "" && file != "-" {
		f, err := os.Create(file)
		if err != nil {
			exitErr(err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	if _, err := w.Write(raw); err != nil {
		exitErr(err)
	}
	if file != "" && file != "-" {
		fmt.Fprintf(os.Stderr, "Wrote %d lines to %s\n", strings.Count(string(raw), "\n"), file)
	}
}

var analyticsExportCmd = &cobra.Command{
	Use:   "export <healthcheck-uuid>",
	Short: "Download a healthcheck's results as CSV or NDJSON",
	Long: `Every check run of a window, oldest first, as CSV (with region names) or
NDJSON. One file covers at most 31 days, within your plan's window; the
default is the last 24 hours.

Examples:
  uptimyctl analytics export <hc-uuid> --from 7d --file results.csv
  uptimyctl analytics export <hc-uuid> --from 2026-11-01T00:00:00Z --to 2026-12-01T00:00:00Z --format ndjson > november.ndjson`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runExport(cmd, "/v1/api/healthchecks/"+url.PathEscape(args[0])+"/results/export")
	},
}

var heartbeatsExportCmd = &cobra.Command{
	Use:   "export <heartbeat-uuid>",
	Short: "Download a heartbeat's runs (ended cycles) as CSV or NDJSON",
	Long: `Every ended cycle of a window (received, failed or missed), oldest first.
One file covers at most 31 days, within your plan's window; the default is
the last 24 hours.

Example:
  uptimyctl heartbeats export <heartbeat-uuid> --from 30d --file runs.csv`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runExport(cmd, "/v1/api/heartbeats/"+url.PathEscape(args[0])+"/cycles/export")
	},
}

func addWindowFlags(cmd *cobra.Command, fromHelp string) {
	cmd.Flags().String("from", "", fromHelp)
	cmd.Flags().String("to", "", "Range end: RFC3339 or a duration back from now (default: now)")
}

func init() {
	addWindowFlags(analyticsResultsCmd, "Range start: RFC3339 or a duration back from now like 12h or 7d (default: your plan's window)")
	analyticsResultsCmd.Flags().Bool("failures", false, "Only failed checks")
	analyticsResultsCmd.Flags().String("region", "", "Only this region (scheduler UUID)")
	analyticsResultsCmd.Flags().Int("limit", 100, "Results per page (1-1000)")
	analyticsResultsCmd.Flags().Bool("all", false, "Follow every page")

	addWindowFlags(analyticsSummaryCmd, "Range start: RFC3339 or a duration back from now like 12h or 7d (default: 24 hours before --to)")
	analyticsSummaryCmd.Flags().Bool("by-region", false, "Each region side by side")
	analyticsSummaryCmd.Flags().Bool("compare", false, "Add the window of the same length before")

	for _, c := range []*cobra.Command{analyticsExportCmd, heartbeatsExportCmd} {
		addWindowFlags(c, "Range start: RFC3339 or a duration back from now like 12h or 7d (default: 24 hours before --to)")
		c.Flags().String("format", "csv", "csv or ndjson")
		c.Flags().String("file", "", "Write to this file instead of stdout")
	}
	analyticsExportCmd.Flags().Bool("failures", false, "Only failed checks")

	analyticsCmd.AddCommand(analyticsResultsCmd, analyticsSummaryCmd, analyticsExportCmd)
	heartbeatsCmd.AddCommand(heartbeatsExportCmd)
}
