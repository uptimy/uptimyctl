package workspace

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/uptimy/uptimyctl/internal/client"
)

// Caller is one service's API (client.Client is one).
type Caller interface {
	Get(path string, query url.Values) ([]byte, error)
	Post(path string, body interface{}) ([]byte, error)
	Put(path string, body interface{}) ([]byte, error)
	Patch(path string, body interface{}) ([]byte, error)
	Delete(path string) ([]byte, error)
}

// Services are the APIs a workspace lives in: monitors, tags and status
// pages on api, cron monitors on heartbeat, alerts on workflows, reports on
// reports.
type Services struct {
	API, Workflows, Heartbeats, Reports Caller
}

// State is the live workspace, as far as the file describes it.
type State struct {
	Workspace     string
	IntervalFloor int
	Regions       []region
	Tags          map[int]string
	Monitors      []liveMonitor
	Heartbeats    []liveHeartbeat
	StatusPages   []livePage
	Workflows     []liveWorkflow
	Integrations  []integration
	Reports       []liveReport

	// Not in the file (defined in an Uptimy Agent, or a check type the file
	// can't express); status pages and alerts can still name them.
	agentMonitors   []liveMonitor
	agentHeartbeats []liveHeartbeat
}

type region struct {
	UUID string
	Name string
	raw  map[string]any
}

type liveMonitor struct {
	UUID            string         `json:"uuid"`
	ApplicationUUID string         `json:"applicationUuid"`
	TypeID          int            `json:"healthcheckTypeId"`
	Config          map[string]any `json:"config"`
	IntervalSeconds int            `json:"intervalSeconds"`
	TimeoutSeconds  int            `json:"timeoutSeconds"`
	Active          bool           `json:"active"`
	ManagedBy       *string        `json:"managedByLocationUuid"`
	Application     struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"application"`
	Schedulers []struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"schedulers"`
	Tags []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"tags"`
}

func (m liveMonitor) name() string { return m.Application.Name }

func (m liveMonitor) appUUID() string {
	if m.Application.UUID != "" {
		return m.Application.UUID
	}
	return m.ApplicationUUID
}

type liveHeartbeat struct {
	UUID              string           `json:"uuid"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	IntervalSeconds   int              `json:"intervalSeconds"`
	Cron              string           `json:"cron"`
	Timezone          string           `json:"timezone"`
	NoSchedule        bool             `json:"noSchedule"`
	MaxRunSeconds     int              `json:"maxRunSeconds"`
	GraceSeconds      int              `json:"graceSeconds"`
	FailureThreshold  int              `json:"failureThreshold"`
	RecoveryThreshold int              `json:"recoveryThreshold"`
	EnforceOrder      bool             `json:"enforceOrder"`
	PausedAt          *string          `json:"pausedAt"`
	Steps             []map[string]any `json:"steps"`
	TagIDs            []int            `json:"tagIds"`
	Source            string           `json:"source"`
}

type livePage struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
	Groups []liveGroup
}

type liveGroup struct {
	UUID    string      `json:"uuid"`
	Label   string      `json:"label"`
	Hidden  bool        `json:"hidden"`
	Order   int         `json:"order"`
	Entries []liveEntry `json:"entries"`
}

type liveEntry struct {
	Kind                 string  `json:"kind"`
	ApplicationUUID      *string `json:"applicationUuid"`
	HeartbeatMonitorUUID *string `json:"heartbeatMonitorUuid"`
	Order                int     `json:"order"`
	ShowDailies          bool    `json:"showDailies"`
	IsVisible            bool    `json:"isVisible"`
	CustomLabel          *string `json:"customLabel"`
}

func (e liveEntry) target() string {
	if e.ApplicationUUID != nil {
		return *e.ApplicationUUID
	}
	if e.HeartbeatMonitorUUID != nil {
		return *e.HeartbeatMonitorUUID
	}
	return ""
}

type liveWorkflow struct {
	UUID    string     `json:"uuid"`
	Name    string     `json:"name"`
	Enabled bool       `json:"enabled"`
	Alert   *liveAlert `json:"alert"`
}

type liveAlert struct {
	Destination struct {
		Type            string   `json:"type"`
		IntegrationUUID string   `json:"integrationUuid"`
		Recipients      []string `json:"recipients"`
		URL             string   `json:"url"`
	} `json:"destination"`
	Events      []string `json:"events"`
	TagIDs      []int    `json:"tagIds"`
	Severities  []string `json:"severities"`
	SourceUUIDs []string `json:"sourceUuids"`
}

type integration struct {
	UUID     string         `json:"uuid"`
	Provider string         `json:"provider"`
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
}

var alertProviders = map[string]string{"slack": "Slack", "discord": "Discord", "teams": "Teams", "telegram": "Telegram", "pagerduty": "PagerDuty"}

// Label names an integration the way the app shows it: "Slack #ops (Acme)".
func (i integration) Label() string {
	p := alertProviders[i.Provider]
	if p == "" {
		p = i.Provider
	}
	if ch, _ := i.Config["channelName"].(string); ch != "" {
		return fmt.Sprintf("%s #%s (%s)", p, strings.TrimPrefix(ch, "#"), i.Name)
	}
	return fmt.Sprintf("%s (%s)", p, i.Name)
}

func get(c Caller, path string, out any) error {
	raw, err := c.Get(path, nil)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

type results[T any] struct {
	Results []T `json:"results"`
}

// Fetch reads the live workspace.
func Fetch(s Services) (*State, error) {
	st := &State{Tags: map[int]string{}}

	var key struct {
		Workspace struct {
			Name string `json:"name"`
		} `json:"workspace"`
	}
	if err := get(s.API, "/v1/api/api-key", &key); err != nil {
		return nil, err
	}
	st.Workspace = key.Workspace.Name

	var quota struct {
		HealthcheckFrequency int `json:"healthcheckFrequency"`
	}
	if err := get(s.API, "/v1/api/quota", &quota); err != nil {
		return nil, err
	}
	st.IntervalFloor = quota.HealthcheckFrequency

	var regions results[map[string]any]
	if err := get(s.API, "/v1/api/schedulers/", &regions); err != nil {
		return nil, err
	}
	for _, r := range regions.Results {
		uuid, _ := r["uuid"].(string)
		name, _ := r["name"].(string)
		st.Regions = append(st.Regions, region{UUID: uuid, Name: name, raw: r})
	}

	var tags results[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}]
	if err := get(s.API, "/v1/api/tags/", &tags); err != nil {
		return nil, err
	}
	for _, t := range tags.Results {
		st.Tags[t.ID] = t.Name
	}

	var monitors results[liveMonitor]
	if err := get(s.API, "/v1/api/healthchecks/", &monitors); err != nil {
		return nil, err
	}
	for _, m := range monitors.Results {
		// Agent monitors are defined in the agent, and the file has no name
		// for the old heartbeat check type: they're not this file's.
		if m.ManagedBy == nil && typeName(m.TypeID) != "" {
			st.Monitors = append(st.Monitors, m)
		} else {
			st.agentMonitors = append(st.agentMonitors, m)
		}
	}
	sort.SliceStable(st.Monitors, func(i, j int) bool { return st.Monitors[i].name() < st.Monitors[j].name() })

	var heartbeats results[liveHeartbeat]
	if err := get(s.Heartbeats, "/v1/api/heartbeat-monitors/", &heartbeats); err != nil {
		return nil, err
	}
	for _, h := range heartbeats.Results {
		switch h.Source {
		case "", "cloud":
			st.Heartbeats = append(st.Heartbeats, h)
		case "agent":
			st.agentHeartbeats = append(st.agentHeartbeats, h)
		}
	}
	sort.SliceStable(st.Heartbeats, func(i, j int) bool { return st.Heartbeats[i].Name < st.Heartbeats[j].Name })

	var pages results[livePage]
	if err := get(s.API, "/v1/api/status-pages/", &pages); err != nil {
		return nil, err
	}
	for _, p := range pages.Results {
		var groups results[liveGroup]
		if err := get(s.API, "/v1/api/status-pages/"+url.PathEscape(p.UUID)+"/groups/", &groups); err != nil {
			return nil, err
		}
		p.Groups = groups.Results
		sort.SliceStable(p.Groups, func(i, j int) bool { return p.Groups[i].Order < p.Groups[j].Order })
		for _, g := range p.Groups {
			sort.SliceStable(g.Entries, func(i, j int) bool { return g.Entries[i].Order < g.Entries[j].Order })
		}
		st.StatusPages = append(st.StatusPages, p)
	}

	var workflows results[liveWorkflow]
	if err := get(s.Workflows, "/v1/api/workflows/", &workflows); err != nil {
		return nil, err
	}
	st.Workflows = workflows.Results

	var integrations results[integration]
	if err := get(s.Workflows, "/v1/api/integrations/", &integrations); err != nil {
		return nil, err
	}
	st.Integrations = integrations.Results

	var reports results[liveReport]
	if err := get(s.Reports, "/v1/api/reports/definitions", &reports); err != nil {
		return nil, err
	}
	st.Reports = reports.Results
	return st, nil
}

// Names the file uses for things the services know by id.

func (st *State) tagNames(ids []int) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := st.Tags[id]; ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func (st *State) tagID(name string) (int, bool) {
	for id, n := range st.Tags {
		if strings.EqualFold(n, name) {
			return id, true
		}
	}
	return 0, false
}

// subjectName names an application or heartbeat UUID.
func (st *State) subjectName(uuid string) string {
	for _, m := range slices.Concat(st.Monitors, st.agentMonitors) {
		if m.appUUID() == uuid {
			return m.name()
		}
	}
	for _, h := range slices.Concat(st.Heartbeats, st.agentHeartbeats) {
		if h.UUID == uuid {
			return h.Name
		}
	}
	return ""
}

// subject finds a monitor or heartbeat by name or UUID: how status pages and
// alerts refer to them (an application UUID for a monitor).
func (st *State) subject(name string) (kind, uuid string, ok bool) {
	for _, m := range slices.Concat(st.Monitors, st.agentMonitors) {
		if strings.EqualFold(m.name(), name) || m.appUUID() == name {
			return "service", m.appUUID(), true
		}
	}
	for _, h := range slices.Concat(st.Heartbeats, st.agentHeartbeats) {
		if strings.EqualFold(h.Name, name) || h.UUID == name {
			return "heartbeat", h.UUID, true
		}
	}
	return "", "", false
}

// regionByName matches a region name the way people write it: case and
// leading flags don't matter.
func (st *State) regionByName(name string) (region, bool) {
	want := plainRegion(name)
	for _, r := range st.Regions {
		if r.UUID == name || plainRegion(r.Name) == want {
			return r, true
		}
	}
	return region{}, false
}

func plainRegion(name string) string {
	return strings.ToLower(strings.TrimLeftFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	}))
}

// isUUID reports whether s is written like a UUID: how the file keeps an
// alert's reference to a monitor that's been deleted.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (st *State) allRegionNames() []string {
	out := make([]string, 0, len(st.Regions))
	for _, r := range st.Regions {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}
