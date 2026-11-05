// Package workspace keeps a whole Uptimy workspace in one file: monitors,
// cron monitors (heartbeats), status pages, alerts and reports, matched by
// name.
// export writes the file from the live workspace, plan shows what applying
// it would change, apply makes those changes. Each service keeps its own
// data; this package only calls their /v1/api/* routes.
//
// Things made elsewhere stay where they were made: agent-managed monitors
// and workflows built in the editor are never exported, changed or removed.
package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Version is the file format this package reads and writes.
const Version = 2

type File struct {
	Version     int          `yaml:"version" json:"version"`
	Monitors    []Monitor    `yaml:"monitors,omitempty" json:"monitors,omitempty"`
	Heartbeats  []Heartbeat  `yaml:"heartbeats,omitempty" json:"heartbeats,omitempty"`
	StatusPages []StatusPage `yaml:"statusPages,omitempty" json:"statusPages,omitempty"`
	Alerts      []Alert      `yaml:"alerts,omitempty" json:"alerts,omitempty"`
	// Left out, reports are left as they are, even with --prune.
	Reports []Report `yaml:"reports,omitempty" json:"reports,omitempty"`
}

// Monitor is a website/API check. The simple types take a target; any type
// can give its config as the API stores it instead (gRPC, GraphQL and
// browser checks must).
type Monitor struct {
	Name string `yaml:"name" json:"name"`
	// http (default), ssl, tcp, ping, dns, grpc, graphql or browser.
	Type           string   `yaml:"type,omitempty" json:"type,omitempty"`
	Target         string   `yaml:"target,omitempty" json:"target,omitempty"`
	Port           int      `yaml:"port,omitempty" json:"port,omitempty"`
	ExpectedStatus int      `yaml:"expectedStatus,omitempty" json:"expectedStatus,omitempty"`
	RecordType     string   `yaml:"recordType,omitempty" json:"recordType,omitempty"`
	ExpectedValues []string `yaml:"expectedValues,omitempty" json:"expectedValues,omitempty"`
	// Config overrides what the target builds, key by key.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
	// Seconds. Default 60 (ssl 3600), raised to the plan's minimum.
	Interval int `yaml:"interval,omitempty" json:"interval,omitempty"`
	// Seconds. Default 30.
	Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// Region names. Default: every region the plan allows.
	Regions []string `yaml:"regions,omitempty" json:"regions,omitempty"`
	Tags    []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Paused  bool     `yaml:"paused,omitempty" json:"paused,omitempty"`
}

// Heartbeat is a cron monitor: exactly one of every, cron or noSchedule.
type Heartbeat struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Seconds between runs.
	Every      int    `yaml:"every,omitempty" json:"every,omitempty"`
	Cron       string `yaml:"cron,omitempty" json:"cron,omitempty"`
	Timezone   string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	NoSchedule bool   `yaml:"noSchedule,omitempty" json:"noSchedule,omitempty"`
	// Seconds; a no-schedule run with steps fails after this. 0: no limit.
	MaxRun int `yaml:"maxRun,omitempty" json:"maxRun,omitempty"`
	// Seconds a run may be late. Default 300 (0 with no schedule).
	Grace int `yaml:"grace,omitempty" json:"grace,omitempty"`
	// Default 1 each.
	FailureThreshold  int `yaml:"failureThreshold,omitempty" json:"failureThreshold,omitempty"`
	RecoveryThreshold int `yaml:"recoveryThreshold,omitempty" json:"recoveryThreshold,omitempty"`
	// Steps must ping in order. Default true.
	EnforceOrder *bool `yaml:"enforceOrder,omitempty" json:"enforceOrder,omitempty"`
	// Multi-step runs, as the heartbeat API takes them.
	Steps  []map[string]any `yaml:"steps,omitempty" json:"steps,omitempty"`
	Tags   []string         `yaml:"tags,omitempty" json:"tags,omitempty"`
	Paused bool             `yaml:"paused,omitempty" json:"paused,omitempty"`
}

type StatusPage struct {
	Name   string `yaml:"name" json:"name"`
	Public bool   `yaml:"public,omitempty" json:"public,omitempty"`
	// When given, the page shows exactly these groups, in this order.
	// Left out, the page's groups are left as they are.
	Groups []PageGroup `yaml:"groups,omitempty" json:"groups,omitempty"`
}

type PageGroup struct {
	Name   string `yaml:"name" json:"name"`
	Hidden bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	// Monitor and cron monitor names, in order.
	Monitors []string `yaml:"monitors" json:"monitors"`
}

// Alert sends incidents somewhere (a template alert in workflows).
type Alert struct {
	Name string  `yaml:"name" json:"name"`
	To   AlertTo `yaml:"to" json:"to"`
	// opened and/or resolved. Default both.
	Events []string `yaml:"events,omitempty" json:"events,omitempty"`
	// Scope: one of each that's set must match; none = every incident.
	Tags       []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Severities []string `yaml:"severities,omitempty" json:"severities,omitempty"`
	Monitors   []string `yaml:"monitors,omitempty" json:"monitors,omitempty"`
	Paused     bool     `yaml:"paused,omitempty" json:"paused,omitempty"`
}

// AlertTo is exactly one destination.
type AlertTo struct {
	// A connected Slack, Discord, Teams, Telegram or PagerDuty integration:
	// its label ("Slack #ops (Acme)"), name, UUID, or provider when only one
	// of that provider is connected.
	Integration string   `yaml:"integration,omitempty" json:"integration,omitempty"`
	Emails      []string `yaml:"emails,omitempty" json:"emails,omitempty"`
	Webhook     string   `yaml:"webhook,omitempty" json:"webhook,omitempty"`
}

// Report is an uptime report: availability against an SLA target, read any
// time and, with emails, sent on a schedule. More than the default report
// ("All monitors", every monitor), and anything but weekly, need Growth and
// above.
type Report struct {
	Name string `yaml:"name" json:"name"`
	// Monitor and cron monitor names, or tags; neither: every monitor.
	Monitors []string `yaml:"monitors,omitempty" json:"monitors,omitempty"`
	Tags     []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Percent. Default 99.9.
	SLA float64 `yaml:"sla,omitempty" json:"sla,omitempty"`
	// Days with a p95 above this many ms count as slow (shown, never downtime).
	SlowAboveMs int `yaml:"slowAboveMs,omitempty" json:"slowAboveMs,omitempty"`
	// weekly, monthly, quarterly or none. Default monthly with emails, none
	// (read on demand) without.
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	// Weekly: monday … sunday. Default monday.
	Weekday string `yaml:"weekday,omitempty" json:"weekday,omitempty"`
	// Monthly and quarterly: 1-28. Default 1.
	DayOfMonth int `yaml:"dayOfMonth,omitempty" json:"dayOfMonth,omitempty"`
	// HH:MM. Default 09:00.
	At string `yaml:"at,omitempty" json:"at,omitempty"`
	// Default: the default report's (the workspace's).
	Timezone string   `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	Emails   []string `yaml:"emails,omitempty" json:"emails,omitempty"`
	// Not emailed; the schedule and emails are kept.
	Paused bool `yaml:"paused,omitempty" json:"paused,omitempty"`
	// A logo is uploaded in the app and kept.
	Brand *ReportBrand `yaml:"brand,omitempty" json:"brand,omitempty"`
}

type ReportBrand struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// #rrggbb
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Load reads a file in YAML (or JSON, which is YAML too). "-" reads stdin.
func Load(path string) (*File, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = os.ReadFile("/dev/stdin")
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse reads a file's contents and checks what can be checked offline.
func Parse(data []byte) (*File, error) {
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("version is %d; this uptimyctl reads version %d files (uptimyctl export writes one)", f.Version, Version)
	}
	return &f, f.validate()
}

func (f *File) validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	unique := func(kind string, names []string) {
		seen := map[string]bool{}
		for _, n := range names {
			if strings.TrimSpace(n) == "" {
				add("a %s has no name", kind)
				continue
			}
			if seen[strings.ToLower(n)] {
				add("two %ss are named %q: names identify them", kind, n)
			}
			seen[strings.ToLower(n)] = true
		}
	}

	names := make([]string, 0, len(f.Monitors))
	for _, m := range f.Monitors {
		names = append(names, m.Name)
		if _, err := desiredConfig(m); err != nil {
			add("monitor %q: %v", m.Name, err)
		}
	}
	unique("monitor", names)

	names = names[:0]
	for _, h := range f.Heartbeats {
		names = append(names, h.Name)
		set := 0
		for _, s := range []bool{h.Every > 0, h.Cron != "", h.NoSchedule} {
			if s {
				set++
			}
		}
		if set != 1 {
			add("heartbeat %q: give exactly one of every, cron or noSchedule", h.Name)
		}
	}
	unique("heartbeat", names)

	names = names[:0]
	for _, p := range f.StatusPages {
		names = append(names, p.Name)
	}
	unique("status page", names)

	names = names[:0]
	for _, a := range f.Alerts {
		names = append(names, a.Name)
		set := 0
		for _, s := range []bool{a.To.Integration != "", len(a.To.Emails) > 0, a.To.Webhook != ""} {
			if s {
				set++
			}
		}
		if set != 1 {
			add("alert %q: give exactly one of to.integration, to.emails or to.webhook", a.Name)
		}
		for _, e := range a.Events {
			if e != "opened" && e != "resolved" {
				add("alert %q: event %q: use opened or resolved", a.Name, e)
			}
		}
		for _, s := range a.Severities {
			switch s {
			case "critical", "high", "medium", "low":
			default:
				add("alert %q: severity %q: use critical, high, medium or low", a.Name, s)
			}
		}
	}
	unique("alert", names)

	names = names[:0]
	for _, r := range f.Reports {
		names = append(names, r.Name)
		errs = append(errs, validateReport(r)...)
	}
	unique("report", names)

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// Marshal writes the file as YAML under a header comment.
func (f *File) Marshal(header string) ([]byte, error) {
	var buf bytes.Buffer
	for _, line := range strings.Split(strings.TrimSpace(header), "\n") {
		buf.WriteString("# " + line + "\n")
	}
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

// normalized turns YAML/JSON values into the shapes JSON decoding gives
// (map[string]any, []any, float64), so configs from the file and from the
// API compare equal.
func normalized(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}
