package workspace

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// heartbeatWant is a file heartbeat with its defaults filled in.
type heartbeatWant struct {
	Heartbeat
	timezone     string
	grace        int
	failure      int
	recovery     int
	enforceOrder bool
	steps        []any
}

func resolveHeartbeat(h Heartbeat) heartbeatWant {
	w := heartbeatWant{Heartbeat: h, grace: h.Grace, failure: h.FailureThreshold, recovery: h.RecoveryThreshold, enforceOrder: true}
	if h.Cron != "" {
		w.timezone = h.Timezone
		if w.timezone == "" {
			w.timezone = "UTC"
		}
	}
	// The app's defaults for a new heartbeat. Grace is how late a run may be;
	// with no schedule nothing is late, so it stays 0.
	if w.grace == 0 && !h.NoSchedule {
		w.grace = 300
	}
	if w.failure == 0 {
		w.failure = 1
	}
	if w.recovery == 0 {
		w.recovery = 1
	}
	if h.EnforceOrder != nil {
		w.enforceOrder = *h.EnforceOrder
	}
	w.steps = cleanSteps(h.Steps)
	return w
}

// cleanSteps drops the ids the API adds, so steps compare by what they say.
func cleanSteps(steps []map[string]any) []any {
	out := []any{}
	for _, s := range steps {
		out = append(out, dropUUIDs(normalized(s)))
	}
	return out
}

func dropUUIDs(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			if k != "uuid" {
				out[k] = dropUUIDs(x)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, x := range t {
			out = append(out, dropUUIDs(x))
		}
		return out
	}
	return v
}

func (w heartbeatWant) schedule() string {
	switch {
	case w.Cron != "":
		return fmt.Sprintf("cron %s (%s)", w.Cron, w.timezone)
	case w.NoSchedule:
		return "no schedule"
	default:
		return fmt.Sprintf("every %ds", w.Every)
	}
}

func liveSchedule(h liveHeartbeat) string {
	switch {
	case h.Cron != "":
		tz := h.Timezone
		if tz == "" {
			tz = "UTC"
		}
		return fmt.Sprintf("cron %s (%s)", h.Cron, tz)
	case h.NoSchedule:
		return "no schedule"
	default:
		return fmt.Sprintf("every %ds", h.IntervalSeconds)
	}
}

func heartbeatChanges(w heartbeatWant, live liveHeartbeat, st *State) []string {
	var d []string
	if s := liveSchedule(live); s != w.schedule() {
		d = append(d, fmt.Sprintf("schedule %s → %s", s, w.schedule()))
	}
	if live.MaxRunSeconds != w.MaxRun {
		d = append(d, fmt.Sprintf("max run %ds → %ds", live.MaxRunSeconds, w.MaxRun))
	}
	if live.GraceSeconds != w.grace {
		d = append(d, fmt.Sprintf("grace %ds → %ds", live.GraceSeconds, w.grace))
	}
	if live.FailureThreshold != w.failure || live.RecoveryThreshold != w.recovery {
		d = append(d, fmt.Sprintf("thresholds %d/%d → %d/%d", live.FailureThreshold, live.RecoveryThreshold, w.failure, w.recovery))
	}
	if live.EnforceOrder != w.enforceOrder {
		d = append(d, fmt.Sprintf("enforce order → %v", w.enforceOrder))
	}
	if !reflect.DeepEqual(cleanSteps(live.Steps), w.steps) {
		d = append(d, fmt.Sprintf("steps (%d → %d)", len(live.Steps), len(w.steps)))
	}
	if live.Description != w.Description {
		d = append(d, "description")
	}
	if diff := setDiff("tags", st.tagNames(live.TagIDs), w.Tags); diff != "" {
		d = append(d, diff)
	}
	if (live.PausedAt != nil) != w.Paused {
		if w.Paused {
			d = append(d, "pause")
		} else {
			d = append(d, "resume")
		}
	}
	return d
}

// body is the full monitor the heartbeat API takes on create and update:
// every field set, so an update leaves nothing as it was.
func (w heartbeatWant) body(st *State) (map[string]any, error) {
	tags, err := tagIDsFor(st, w.Tags)
	if err != nil {
		return nil, err
	}
	b := map[string]any{"name": w.Name, "description": w.Description, "intervalSeconds": w.Every,
		"cron": w.Cron, "noSchedule": w.NoSchedule, "maxRunSeconds": w.MaxRun,
		"graceSeconds": w.grace, "failureThreshold": w.failure, "recoveryThreshold": w.recovery,
		"enforceOrder": w.enforceOrder, "paused": w.Paused, "steps": w.steps, "tagIds": tags}
	if w.Cron != "" {
		b["timezone"] = w.timezone
	}
	return b, nil
}

func createHeartbeat(s Services, st *State, w heartbeatWant) error {
	b, err := w.body(st)
	if err != nil {
		return err
	}
	_, err = s.Heartbeats.Post("/v1/api/heartbeat-monitors/", b)
	return err
}

func updateHeartbeat(s Services, st *State, w heartbeatWant, live liveHeartbeat) error {
	b, err := w.body(st)
	if err != nil {
		return err
	}
	_, err = s.Heartbeats.Put("/v1/api/heartbeat-monitors/"+url.PathEscape(live.UUID), b)
	return err
}

// exportHeartbeat writes a live heartbeat as the file has it, defaults left out.
func exportHeartbeat(h liveHeartbeat, st *State) Heartbeat {
	out := Heartbeat{Name: h.Name, Description: h.Description, Tags: st.tagNames(h.TagIDs), Paused: h.PausedAt != nil}
	switch {
	case h.Cron != "":
		out.Cron = h.Cron
		if h.Timezone != "" && !strings.EqualFold(h.Timezone, "UTC") {
			out.Timezone = h.Timezone
		}
	case h.NoSchedule:
		out.NoSchedule = true
		out.MaxRun = h.MaxRunSeconds
	default:
		out.Every = h.IntervalSeconds
	}
	if defaultGrace := 300; h.NoSchedule && h.GraceSeconds != 0 || !h.NoSchedule && h.GraceSeconds != defaultGrace {
		out.Grace = h.GraceSeconds
	}
	if h.FailureThreshold != 1 {
		out.FailureThreshold = h.FailureThreshold
	}
	if h.RecoveryThreshold != 1 {
		out.RecoveryThreshold = h.RecoveryThreshold
	}
	if !h.EnforceOrder {
		f := false
		out.EnforceOrder = &f
	}
	for _, s := range cleanSteps(h.Steps) {
		if m, ok := s.(map[string]any); ok {
			out.Steps = append(out.Steps, m)
		}
	}
	return out
}
