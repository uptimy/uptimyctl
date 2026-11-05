package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/uptimy/uptimyctl/internal/client"
)

// Healthcheck type ids (upti.my-api healthcheck_type) by the names the file uses.
var typeIDs = map[string]int{"http": 1, "grpc": 2, "graphql": 3, "ping": 4, "tcp": 5, "ssl": 6, "dns": 7, "browser": 8}

func typeName(id int) string {
	for name, i := range typeIDs {
		if i == id {
			return name
		}
	}
	return ""
}

func monitorType(m Monitor) string {
	if m.Type == "" {
		return "http"
	}
	return strings.ToLower(m.Type)
}

// desiredConfig is the config a monitor asks for: what its target builds,
// overridden by its config key by key. The same rules as the Uptimy MCP's
// add_monitors and the app's Setup.
func desiredConfig(m Monitor) (map[string]any, error) {
	t := monitorType(m)
	if _, ok := typeIDs[t]; !ok {
		return nil, fmt.Errorf("unknown type %q: use http, ssl, tcp, ping, dns, grpc, graphql or browser", m.Type)
	}
	cfg := map[string]any{}
	if target := strings.TrimSpace(m.Target); target != "" {
		built, err := buildConfig(t, m)
		if err != nil {
			return nil, err
		}
		cfg = built
	} else if len(m.Config) == 0 {
		return nil, errors.New("give a target or a config")
	}
	for k, v := range m.Config {
		cfg[k] = v
	}
	out, _ := normalized(cfg).(map[string]any)
	return out, nil
}

func buildConfig(t string, m Monitor) (map[string]any, error) {
	target := strings.TrimSpace(m.Target)
	switch t {
	case "http":
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		u, err := url.Parse(target)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("%q isn't an http(s) URL", m.Target)
		}
		status := m.ExpectedStatus
		if status == 0 {
			status = 200
		}
		return map[string]any{"method": "GET", "uri": u.String(), "expectedStatusCode": status,
			"body": map[string]any{}, "headers": map[string]any{}, "expectedResponseBody": map[string]any{}, "proxyUrl": ""}, nil
	case "ssl", "tcp", "ping", "dns":
		if !strings.Contains(target, "://") {
			target = "//" + target
		}
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return nil, fmt.Errorf("%q isn't a host", m.Target)
		}
		host := strings.ToLower(u.Hostname())
		port := m.Port
		if p, err := strconv.Atoi(u.Port()); err == nil && port == 0 {
			port = p
		}
		switch t {
		case "ssl":
			if port == 0 {
				port = 443
			}
			return map[string]any{"host": host, "port": port, "expiryWarningDays": 30}, nil
		case "tcp":
			if port < 1 || port > 65535 {
				return nil, errors.New("a tcp monitor needs a port")
			}
			return map[string]any{"host": host, "port": port}, nil
		case "ping":
			return map[string]any{"host": host}, nil
		default:
			rt := strings.ToUpper(m.RecordType)
			if rt == "" {
				rt = "A"
			}
			switch rt {
			case "A", "AAAA", "CNAME", "MX", "TXT", "NS":
			default:
				return nil, fmt.Errorf("record type %s isn't supported: use A, AAAA, CNAME, MX, TXT or NS", rt)
			}
			cfg := map[string]any{"domain": host, "recordType": rt}
			if len(m.ExpectedValues) > 0 {
				cfg["expectedRecords"] = m.ExpectedValues
			}
			return cfg, nil
		}
	default:
		return nil, fmt.Errorf("a %s monitor takes a config, not a target", t)
	}
}

// shorthand writes a live monitor's config as a target when the target
// builds exactly that config, and as the config itself otherwise.
func shorthand(t string, cfg map[string]any) Monitor {
	m := Monitor{Type: t}
	switch t {
	case "http":
		m.Target, _ = cfg["uri"].(string)
		if s, ok := cfg["expectedStatusCode"].(float64); ok && s != 200 {
			m.ExpectedStatus = int(s)
		}
	case "ssl", "tcp", "ping":
		m.Target, _ = cfg["host"].(string)
		if p, ok := cfg["port"].(float64); ok && !(t == "ssl" && p == 443) {
			m.Port = int(p)
		}
	case "dns":
		m.Target, _ = cfg["domain"].(string)
		if rt, _ := cfg["recordType"].(string); rt != "A" {
			m.RecordType = rt
		}
		if values, ok := cfg["expectedRecords"].([]any); ok {
			for _, v := range values {
				if s, ok := v.(string); ok {
					m.ExpectedValues = append(m.ExpectedValues, s)
				}
			}
		}
	}
	if m.Target != "" {
		if built, err := buildConfig(t, m); err == nil && reflect.DeepEqual(normalized(built), normalized(cfg)) {
			return m
		}
	}
	return Monitor{Type: t, Config: cfg}
}

func defaultInterval(t string, floor int) int {
	i := 60
	if t == "ssl" {
		i = 3600
	}
	return max(i, floor)
}

// monitorWant is a file monitor resolved against the workspace.
type monitorWant struct {
	Monitor
	typeID   int
	config   map[string]any
	interval int
	timeout  int
	regions  []region
}

func resolveMonitor(m Monitor, st *State) (monitorWant, error) {
	w := monitorWant{Monitor: m, typeID: typeIDs[monitorType(m)]}
	cfg, err := desiredConfig(m)
	if err != nil {
		return w, err
	}
	w.config = cfg
	w.interval = m.Interval
	if w.interval == 0 {
		w.interval = defaultInterval(monitorType(m), st.IntervalFloor)
	} else if w.interval < st.IntervalFloor {
		return w, fmt.Errorf("interval %ds is below the plan's minimum of %ds", w.interval, st.IntervalFloor)
	}
	w.timeout = m.Timeout
	if w.timeout == 0 {
		w.timeout = 30
	}
	if len(m.Regions) == 0 {
		w.regions = st.Regions
	} else {
		for _, name := range m.Regions {
			r, ok := st.regionByName(name)
			if !ok {
				return w, fmt.Errorf("no region %q; the plan allows: %s", name, strings.Join(st.allRegionNames(), ", "))
			}
			w.regions = append(w.regions, r)
		}
	}
	if len(w.regions) == 0 {
		return w, errors.New("the workspace has no regions to check from")
	}
	return w, nil
}

func (w monitorWant) regionNames() []string {
	out := make([]string, 0, len(w.regions))
	for _, r := range w.regions {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}

func (w monitorWant) describe() string {
	target := w.Target
	if target == "" {
		target = "custom config"
	}
	return fmt.Sprintf("%s %s, every %ds from %d regions", monitorType(w.Monitor), target, w.interval, len(w.regions))
}

// monitorChanges compares one file monitor with the live one of that name
// (nil: none yet).
func monitorChanges(w monitorWant, live *liveMonitor, st *State) []string {
	var d []string
	if live.TypeID != w.typeID {
		d = append(d, fmt.Sprintf("type %s → %s", typeName(live.TypeID), monitorType(w.Monitor)))
	}
	var keys []string
	for k, v := range w.config {
		if !reflect.DeepEqual(normalized(v), normalized(live.Config[k])) {
			keys = append(keys, k)
		}
	}
	if len(keys) > 0 {
		sort.Strings(keys)
		d = append(d, "config: "+strings.Join(keys, ", "))
	}
	if live.IntervalSeconds != w.interval {
		d = append(d, fmt.Sprintf("interval %ds → %ds", live.IntervalSeconds, w.interval))
	}
	if live.TimeoutSeconds != w.timeout {
		d = append(d, fmt.Sprintf("timeout %ds → %ds", live.TimeoutSeconds, w.timeout))
	}
	liveRegions := make([]string, 0, len(live.Schedulers))
	for _, s := range live.Schedulers {
		liveRegions = append(liveRegions, s.Name)
	}
	if diff := setDiff("regions", liveRegions, w.regionNames()); diff != "" {
		d = append(d, diff)
	}
	liveTags := make([]string, 0, len(live.Tags))
	for _, t := range live.Tags {
		liveTags = append(liveTags, t.Name)
	}
	if diff := setDiff("tags", liveTags, w.Tags); diff != "" {
		d = append(d, diff)
	}
	if live.Active == w.Paused {
		if w.Paused {
			d = append(d, "pause")
		} else {
			d = append(d, "resume")
		}
	}
	return d
}

// setDiff describes what changes between two name lists ("" when none).
func setDiff(label string, have, want []string) string {
	in := func(list []string, s string) bool {
		for _, x := range list {
			if strings.EqualFold(x, s) {
				return true
			}
		}
		return false
	}
	var parts []string
	for _, w := range want {
		if !in(have, w) {
			parts = append(parts, "+"+w)
		}
	}
	for _, h := range have {
		if !in(want, h) {
			parts = append(parts, "-"+h)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return label + " " + strings.Join(parts, " ")
}

func (w monitorWant) healthcheck(live map[string]any) map[string]any {
	cfg := map[string]any{}
	// Keys the file doesn't set keep their live values.
	for k, v := range live {
		cfg[k] = v
	}
	for k, v := range w.config {
		cfg[k] = v
	}
	schedulers := make([]map[string]any, 0, len(w.regions))
	for _, r := range w.regions {
		schedulers = append(schedulers, r.raw)
	}
	return map[string]any{"intervalSeconds": w.interval, "timeoutSeconds": w.timeout, "active": !w.Paused,
		"healthcheckTypeId": w.typeID, "config": cfg, "schedulers": schedulers}
}

func tagIDsFor(st *State, names []string) ([]int, error) {
	ids := []int{}
	for _, n := range names {
		id, ok := st.tagID(n)
		if !ok {
			return nil, fmt.Errorf("no tag %q", n)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func createMonitor(s Services, st *State, w monitorWant) error {
	item := map[string]any{"applicationName": w.Name, "healthcheck": w.healthcheck(nil)}
	raw, err := s.API.Post("/v1/api/healthchecks/bulk", map[string]any{"items": []any{item}})
	if err != nil {
		return err
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		return err
	}
	var created results[struct {
		UUID string `json:"uuid"`
	}]
	if err := json.Unmarshal(data, &created); err != nil || len(created.Results) == 0 {
		return errors.New("the API didn't return the new monitor")
	}
	if len(w.Tags) == 0 {
		return nil
	}
	return setMonitorTags(s, st, created.Results[0].UUID, w.Tags)
}

func updateMonitor(s Services, st *State, w monitorWant, live liveMonitor) error {
	if _, err := s.API.Put("/v1/api/healthchecks/"+url.PathEscape(live.UUID), w.healthcheck(live.Config)); err != nil {
		return err
	}
	liveTags := make([]string, 0, len(live.Tags))
	for _, t := range live.Tags {
		liveTags = append(liveTags, t.Name)
	}
	if setDiff("", liveTags, w.Tags) == "" {
		return nil
	}
	return setMonitorTags(s, st, live.appUUID(), w.Tags)
}

func setMonitorTags(s Services, st *State, appUUID string, names []string) error {
	if len(names) == 0 {
		names = []string{}
	}
	ids, err := tagIDsFor(st, names)
	if err != nil {
		return err
	}
	_, err = s.API.Put("/v1/api/applications/"+url.PathEscape(appUUID)+"/healthchecks/tags/", map[string]any{"tagIds": ids})
	return err
}
