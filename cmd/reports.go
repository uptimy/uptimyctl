package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var reportsCmd = &cobra.Command{
	Use:   "reports",
	Short: "Read uptime reports",
	Long: `Read uptime reports: availability against the SLA target, downtime,
incidents and planned maintenance for any period, from the reports service.

A report covers every monitor (the workspace's default) or chosen ones (e.g.
one client's); 'reports list' shows them. Months, quarters, ranges and the
current week need a paid plan; the sent weekly report (--week) is free.

Examples:
  uptimyctl reports list
  uptimyctl reports get                          # last 30 days, every monitor
  uptimyctl reports get --last 7d --report Acme
  uptimyctl reports get --month 2026-09
  uptimyctl reports get --quarter 2026-Q3 -o json
  uptimyctl reports get --from 2026-09-01 --to 2026-09-15 --csv days > september.csv`,
}

var reportsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the workspace's reports",
	Run: func(cmd *cobra.Command, args []string) {
		c := newReportsClient()
		raw, err := c.Get("/v1/api/reports/definitions", nil)
		if err != nil {
			exitErr(err)
		}
		if output.IsJSON() {
			output.PrintJSONBytes(raw)
			return
		}
		defs, err := parseDefinitions(raw)
		if err != nil {
			exitErr(err)
		}
		rows := make([][]string, 0, len(defs))
		for _, d := range defs {
			covers := "every monitor"
			if d.ScopeType == "monitors" {
				covers = fmt.Sprintf("%d monitors", len(d.MonitorUUIDs))
			}
			rows = append(rows, []string{d.Name, d.UUID, covers, fmt.Sprintf("%g%%", d.SLATarget), d.schedule()})
		}
		output.PrintTable([]string{"NAME", "UUID", "COVERS", "SLA TARGET", "SCHEDULE"}, rows)
	},
}

var reportsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show an uptime report for a period",
	Long: `Show an uptime report. Pick one period (default: the last 30 days):
  --last 7d|30d|90d     the last days, ending today
  --week                the latest sent weekly report
  --this-week           the week in progress
  --month YYYY-MM       a calendar month
  --quarter YYYY-Qn     a calendar quarter
  --from/--to           any range of days (YYYY-MM-DD, both included)

--csv monitors|days|incidents|maintenance prints the report as CSV instead.`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := reportFlagsFrom(cmd)
		c := newReportsClient()

		q := url.Values{}
		if flags.report != "" {
			uuid, err := resolveReport(c, flags.report)
			if err != nil {
				exitErr(err)
			}
			if uuid != "" {
				q.Set("definition", uuid)
			}
		}

		var today time.Time
		if flags.needsToday() {
			periods, err := reportPeriods(c, q.Get("definition"))
			if err != nil {
				exitErr(err)
			}
			today = periods.today()
		}
		path, err := resolveReportPeriod(flags, today, q)
		if err != nil {
			exitErr(err)
		}
		if path == "" { // --week: the latest sent week
			periods, err := reportPeriods(c, q.Get("definition"))
			if err != nil {
				exitErr(err)
			}
			if len(periods.Weeks) == 0 {
				exitErr(errors.New("this report hasn't sent a weekly report yet; try --last 7d"))
			}
			path = "/v1/api/reports/weekly/" + periods.Weeks[0].PeriodStart
		}

		if flags.csv != "" {
			q.Set("format", "csv")
			q.Set("rows", flags.csv)
		}
		raw, err := c.Get(path, q)
		if err != nil {
			exitErr(err)
		}
		if flags.csv != "" {
			fmt.Print(string(raw))
			return
		}
		if output.IsJSON() {
			output.PrintJSONBytes(raw)
			return
		}
		printReport(raw)
	},
}

// ── period flags ──

type reportFlags struct {
	report, last, month, quarter, from, to, csv string
	week, thisWeek                              bool
}

func reportFlagsFrom(cmd *cobra.Command) reportFlags {
	f := reportFlags{}
	f.report, _ = cmd.Flags().GetString("report")
	f.last, _ = cmd.Flags().GetString("last")
	f.month, _ = cmd.Flags().GetString("month")
	f.quarter, _ = cmd.Flags().GetString("quarter")
	f.from, _ = cmd.Flags().GetString("from")
	f.to, _ = cmd.Flags().GetString("to")
	f.csv, _ = cmd.Flags().GetString("csv")
	f.week, _ = cmd.Flags().GetBool("week")
	f.thisWeek, _ = cmd.Flags().GetBool("this-week")
	return f
}

// needsToday: only "the last N days" depends on today's date (in the report's timezone).
func (f reportFlags) needsToday() bool {
	return f.month == "" && f.quarter == "" && f.from == "" && f.to == "" && !f.week && !f.thisWeek
}

var (
	monthFlag   = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)
	quarterFlag = regexp.MustCompile(`^(\d{4})-[Qq]([1-4])$`)
	dayFlag     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	lastFlag    = regexp.MustCompile(`^(\d+)d$`)
)

// resolveReportPeriod turns the period flags into the endpoint (adding its
// query parameters to q). "" means the latest sent week, looked up separately.
func resolveReportPeriod(f reportFlags, today time.Time, q url.Values) (string, error) {
	set := 0
	for _, on := range []bool{f.last != "", f.week, f.thisWeek, f.month != "", f.quarter != "", f.from != "" || f.to != ""} {
		if on {
			set++
		}
	}
	if set > 1 {
		return "", errors.New("pick one period: --last, --week, --this-week, --month, --quarter or --from/--to")
	}
	if f.csv != "" && !map[string]bool{"monitors": true, "days": true, "incidents": true, "maintenance": true}[f.csv] {
		return "", errors.New("--csv must be monitors, days, incidents or maintenance")
	}
	switch {
	case f.week:
		return "", nil
	case f.thisWeek:
		return "/v1/api/reports/live", nil
	case f.month != "":
		m := monthFlag.FindStringSubmatch(f.month)
		if m == nil {
			return "", errors.New("--month must be YYYY-MM")
		}
		return "/v1/api/reports/monthly/" + m[1] + "-" + m[2] + "-01", nil
	case f.quarter != "":
		m := quarterFlag.FindStringSubmatch(f.quarter)
		if m == nil {
			return "", errors.New("--quarter must be YYYY-Qn, e.g. 2026-Q3")
		}
		n, _ := strconv.Atoi(m[2])
		return fmt.Sprintf("/v1/api/reports/quarterly/%s-%02d-01", m[1], (n-1)*3+1), nil
	case f.from != "" || f.to != "":
		if !dayFlag.MatchString(f.from) || !dayFlag.MatchString(f.to) {
			return "", errors.New("--from and --to must both be dates (YYYY-MM-DD)")
		}
		q.Set("from", f.from)
		q.Set("to", f.to)
		return "/v1/api/reports/range", nil
	}
	last := f.last
	if last == "" {
		last = "30d"
	}
	m := lastFlag.FindStringSubmatch(last)
	if m == nil {
		return "", errors.New("--last must be a number of days, e.g. 7d, 30d or 90d")
	}
	days, _ := strconv.Atoi(m[1])
	if days < 1 || days > 400 {
		return "", errors.New("--last must be 1d to 400d")
	}
	q.Set("from", today.AddDate(0, 0, -(days-1)).Format("2006-01-02"))
	q.Set("to", today.Format("2006-01-02"))
	return "/v1/api/reports/range", nil
}

// ── lookups ──

type reportDefinition struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	IsDefault    bool     `json:"isDefault"`
	ScopeType    string   `json:"scopeType"`
	MonitorUUIDs []string `json:"monitorUuids"`
	SLATarget    float64  `json:"slaTarget"`
	Cadence      string   `json:"cadence"`
	SendDay      int      `json:"sendDay"`
	SendHour     int      `json:"sendHour"`
	SendMinute   int      `json:"sendMinute"`
	SendTimezone string   `json:"sendTimezone"`
	EmailEnabled bool     `json:"emailEnabled"`
}

func (d reportDefinition) schedule() string {
	if d.Cadence == "none" {
		return "-"
	}
	day := fmt.Sprintf("day %d", d.SendDay)
	if d.Cadence == "weekly" {
		day = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}[d.SendDay%7]
	}
	s := fmt.Sprintf("%s, %s %02d:%02d %s", d.Cadence, day, d.SendHour, d.SendMinute, d.SendTimezone)
	if !d.EmailEnabled {
		s += " (email off)"
	}
	return s
}

func parseDefinitions(raw []byte) ([]reportDefinition, error) {
	data, err := client.ParseResultsField(raw)
	if err != nil {
		return nil, err
	}
	var defs []reportDefinition
	return defs, json.Unmarshal(data, &defs)
}

// resolveReport finds a report by UUID or name; "" is the default report.
func resolveReport(c *client.Client, report string) (string, error) {
	raw, err := c.Get("/v1/api/reports/definitions", nil)
	if err != nil {
		return "", err
	}
	defs, err := parseDefinitions(raw)
	if err != nil {
		return "", err
	}
	for _, d := range defs {
		if d.UUID == report || strings.EqualFold(d.Name, strings.TrimSpace(report)) {
			if d.IsDefault {
				return "", nil
			}
			return d.UUID, nil
		}
	}
	return "", fmt.Errorf("no report named %q; 'uptimyctl reports list' shows them", report)
}

type periodsInfo struct {
	Timezone string `json:"timezone"`
	Weeks    []struct {
		PeriodStart string `json:"periodStart"`
	} `json:"weeks"`
}

func (p periodsInfo) today() time.Time {
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func reportPeriods(c *client.Client, definition string) (periodsInfo, error) {
	q := url.Values{}
	if definition != "" {
		q.Set("definition", definition)
	}
	raw, err := c.Get("/v1/api/reports/periods", q)
	if err != nil {
		return periodsInfo{}, err
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		return periodsInfo{}, err
	}
	var p periodsInfo
	return p, json.Unmarshal(data, &p)
}

// ── printing ──

type reportResponse struct {
	Live        bool   `json:"live"`
	Methodology string `json:"methodology"`
	Report      struct {
		Period struct {
			Type, Start, End, Timezone string
		} `json:"period"`
		SLATarget float64 `json:"slaTarget"`
		Overall   struct {
			Availability    *float64 `json:"availability"`
			DowntimeSeconds float64  `json:"downtimeSeconds"`
			SLAMet          *bool    `json:"slaMet"`
			Trend           *float64 `json:"trend"`
		} `json:"overall"`
		Heartbeats struct {
			Count          int      `json:"count"`
			MissedCheckins int      `json:"missedCheckins"`
			CheckinRate    *float64 `json:"checkinRate"`
		} `json:"heartbeats"`
		Monitors []struct {
			UUID            string   `json:"uuid"`
			Kind            string   `json:"kind"`
			Name            string   `json:"name"`
			Availability    float64  `json:"availability"`
			DowntimeSeconds float64  `json:"downtimeSeconds"`
			P95LatencyMs    *float64 `json:"p95LatencyMs"`
			Incidents       int      `json:"incidents"`
			Deleted         bool     `json:"deleted"`
		} `json:"monitors"`
		Incidents struct {
			Count       int      `json:"count"`
			Resolved    int      `json:"resolved"`
			MTTRSeconds *float64 `json:"mttrSeconds"`
			List        []struct {
				Title       string  `json:"title"`
				MonitorUUID *string `json:"monitorUuid"`
				StartedAt   string  `json:"startedAt"`
				ResolvedAt  *string `json:"resolvedAt"`
				ErrorClass  *string `json:"errorClass"`
			} `json:"list"`
		} `json:"incidents"`
		Maintenance []struct {
			Title    string   `json:"title"`
			Start    string   `json:"start"`
			End      *string  `json:"end"`
			Seconds  float64  `json:"seconds"`
			Monitors []string `json:"monitors"`
		} `json:"maintenance"`
	} `json:"report"`
}

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return "none"
	}
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Round(time.Minute).Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Round(time.Minute).Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

func percent(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.3f%%", *v)
}

func printReport(raw []byte) {
	data, err := client.ParseDataField(raw)
	if err != nil {
		exitErr(err)
	}
	var r reportResponse
	if err := json.Unmarshal(data, &r); err != nil {
		exitErr(err)
	}
	rep := r.Report
	end, _ := time.Parse("2006-01-02", rep.Period.End)
	fmt.Printf("Period:        %s to %s (%s)", rep.Period.Start, end.AddDate(0, 0, -1).Format("2006-01-02"), rep.Period.Timezone)
	if r.Live {
		fmt.Print(", in progress")
	}
	fmt.Println()
	verdict := ""
	if rep.Overall.SLAMet != nil {
		verdict = map[bool]string{true: "  SLA met", false: "  SLA missed"}[*rep.Overall.SLAMet]
	}
	fmt.Printf("Availability:  %s vs %g%% target%s\n", percent(rep.Overall.Availability), rep.SLATarget, verdict)
	if rep.Overall.Trend != nil {
		fmt.Printf("Trend:         %+.3f pts vs the previous period\n", *rep.Overall.Trend)
	}
	fmt.Printf("Downtime:      %s\n", formatDuration(rep.Overall.DowntimeSeconds))
	mttr := "-"
	if rep.Incidents.MTTRSeconds != nil {
		mttr = formatDuration(*rep.Incidents.MTTRSeconds)
	}
	fmt.Printf("Incidents:     %d (%d resolved, time to resolve %s)\n", rep.Incidents.Count, rep.Incidents.Resolved, mttr)
	if rep.Heartbeats.Count > 0 {
		fmt.Printf("Heartbeats:    %s of check-ins received, %d missed\n", percent(rep.Heartbeats.CheckinRate), rep.Heartbeats.MissedCheckins)
	}

	names := map[string]string{}
	rows := [][]string{}
	monitors := rep.Monitors
	sort.SliceStable(monitors, func(i, j int) bool { return monitors[i].Availability < monitors[j].Availability })
	for _, m := range monitors {
		names[m.UUID] = m.Name
		if m.Kind == "heartbeat" {
			continue
		}
		name := m.Name
		if m.Deleted {
			name += " (deleted)"
		}
		p95 := "-"
		if m.P95LatencyMs != nil {
			p95 = fmt.Sprintf("%.0f ms", *m.P95LatencyMs)
		}
		rows = append(rows, []string{output.Truncate(name, 40), fmt.Sprintf("%.3f%%", m.Availability), formatDuration(m.DowntimeSeconds), strconv.Itoa(m.Incidents), p95})
	}
	fmt.Println()
	output.PrintTable([]string{"MONITOR", "AVAILABILITY", "DOWNTIME", "INCIDENTS", "P95"}, rows)

	if len(rep.Incidents.List) > 0 {
		rows = [][]string{}
		for i, inc := range rep.Incidents.List {
			if i == 10 {
				break
			}
			monitor := "-"
			if inc.MonitorUUID != nil {
				monitor = output.ValueOrDash(names[*inc.MonitorUUID])
			}
			duration := "ongoing"
			if inc.ResolvedAt != nil {
				s, _ := time.Parse(time.RFC3339, inc.StartedAt)
				e, _ := time.Parse(time.RFC3339, *inc.ResolvedAt)
				duration = formatDuration(e.Sub(s).Seconds())
			}
			cause := "-"
			if inc.ErrorClass != nil {
				cause = *inc.ErrorClass
			}
			rows = append(rows, []string{output.Truncate(inc.Title, 40), monitor, inc.StartedAt, duration, cause})
		}
		fmt.Println()
		output.PrintTable([]string{"LONGEST INCIDENTS", "MONITOR", "STARTED", "DURATION", "CAUSE"}, rows)
	}

	if len(rep.Maintenance) > 0 {
		rows = [][]string{}
		for _, m := range rep.Maintenance {
			end := "ongoing"
			if m.End != nil {
				end = *m.End
			}
			rows = append(rows, []string{output.Truncate(m.Title, 40), output.Truncate(strings.Join(m.Monitors, ", "), 40), m.Start, end, formatDuration(m.Seconds)})
		}
		fmt.Println()
		output.PrintTable([]string{"PLANNED MAINTENANCE", "MONITORS", "START", "END", "DURATION"}, rows)
	}
	if r.Methodology != "" {
		fmt.Println()
		fmt.Println(r.Methodology)
	}
}

func init() {
	reportsGetCmd.Flags().String("report", "", "report name or UUID (default: the report covering every monitor)")
	reportsGetCmd.Flags().String("last", "", "the last N days ending today, e.g. 7d, 30d, 90d (default 30d)")
	reportsGetCmd.Flags().Bool("week", false, "the latest sent weekly report")
	reportsGetCmd.Flags().Bool("this-week", false, "the week in progress")
	reportsGetCmd.Flags().String("month", "", "a calendar month, YYYY-MM")
	reportsGetCmd.Flags().String("quarter", "", "a calendar quarter, YYYY-Qn")
	reportsGetCmd.Flags().String("from", "", "first day of a range, YYYY-MM-DD")
	reportsGetCmd.Flags().String("to", "", "last day of a range (included), YYYY-MM-DD")
	reportsGetCmd.Flags().String("csv", "", "print CSV instead: monitors, days, incidents or maintenance")

	reportsCmd.AddCommand(reportsListCmd, reportsGetCmd)
	rootCmd.AddCommand(reportsCmd)
}
