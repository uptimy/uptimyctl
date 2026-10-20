package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestParseSeconds(t *testing.T) {
	ok := map[string]int{"300": 300, "90s": 90, "5m": 300, "1h": 3600, "24h": 86400, "1h30m": 5400}
	for in, want := range ok {
		got, err := parseSeconds("interval", in, false)
		if err != nil || got != want {
			t.Errorf("%q: got %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "soon", "0", "-5m", "1.5s", "500ms"} {
		if _, err := parseSeconds("interval", bad, false); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	if got, err := parseSeconds("grace", "0", true); err != nil || got != 0 {
		t.Errorf("grace may be 0: got %d, %v", got, err)
	}
}

func TestHeartbeatRequestKeepsEverything(t *testing.T) {
	steps := []interface{}{map[string]interface{}{"key": "extract", "name": "Extract", "required": true}}
	hb := map[string]interface{}{
		"uuid": "hb-1", "name": "ETL", "description": "nightly", "intervalSeconds": 3600.0, "graceSeconds": 300.0,
		"failureThreshold": 2.0, "recoveryThreshold": 1.0, "enforceOrder": true, "steps": steps,
		"pausedAt": "2026-09-30T10:00:00Z", "statusSnapshot": map[string]interface{}{"status": "Healthy"},
	}
	body := heartbeatRequest(hb)
	if body["paused"] != true {
		t.Fatal("a paused heartbeat must stay paused")
	}
	for _, k := range []string{"name", "description", "intervalSeconds", "graceSeconds", "failureThreshold", "recoveryThreshold", "enforceOrder", "steps"} {
		if _, ok := body[k]; !ok {
			t.Errorf("update body lost %q; updates replace the whole heartbeat", k)
		}
	}
	cron := heartbeatRequest(map[string]interface{}{"name": "Report", "intervalSeconds": 0.0, "cron": "0 9 * * 1-5", "timezone": "Europe/Berlin"})
	if cron["cron"] != "0 9 * * 1-5" || cron["timezone"] != "Europe/Berlin" {
		t.Errorf("a cron heartbeat's schedule must be carried over: %v", cron)
	}
	for _, k := range []string{"uuid", "statusSnapshot", "pausedAt"} {
		if _, ok := body[k]; ok {
			t.Errorf("update body should not send read-only %q", k)
		}
	}
}

func TestApplyHeartbeatFlags(t *testing.T) {
	newCmd := func(args ...string) *cobra.Command {
		c := &cobra.Command{}
		c.Flags().String("name", "", "")
		c.Flags().String("description", "", "")
		c.Flags().String("interval", "", "")
		c.Flags().String("cron", "", "")
		c.Flags().String("timezone", "", "")
		c.Flags().String("grace", "", "")
		c.Flags().Int("failure-threshold", 0, "")
		c.Flags().Int("recovery-threshold", 0, "")
		if err := c.Flags().Parse(args); err != nil {
			t.Fatal(err)
		}
		return c
	}

	// Create: web app defaults for anything not given.
	body := map[string]interface{}{"name": "x"}
	if err := applyHeartbeatFlags(newCmd(), body, true); err != nil {
		t.Fatal(err)
	}
	if body["intervalSeconds"] != 3600 || body["graceSeconds"] != 300 {
		t.Fatalf("create defaults: %v", body)
	}

	// A -f spec's own values win over defaults.
	body = map[string]interface{}{"name": "x", "intervalSeconds": 60.0}
	_ = applyHeartbeatFlags(newCmd(), body, true)
	if body["intervalSeconds"] != 60.0 {
		t.Fatalf("spec interval overwritten: %v", body)
	}

	// Update: only passed flags change.
	body = map[string]interface{}{"name": "old", "intervalSeconds": 60.0, "graceSeconds": 10.0}
	if err := applyHeartbeatFlags(newCmd("--interval", "30m"), body, false); err != nil {
		t.Fatal(err)
	}
	if body["intervalSeconds"] != 1800 || body["graceSeconds"] != 10.0 || body["name"] != "old" {
		t.Fatalf("partial update: %v", body)
	}

	if err := applyHeartbeatFlags(newCmd("--failure-threshold", "0"), map[string]interface{}{}, false); err == nil {
		t.Fatal("threshold 0 accepted")
	}

	// Cron: no default interval, and the zone goes along.
	body = map[string]interface{}{"name": "x"}
	if err := applyHeartbeatFlags(newCmd("--cron", "0 9 * * 1-5", "--timezone", "Europe/Berlin"), body, true); err != nil {
		t.Fatal(err)
	}
	if body["cron"] != "0 9 * * 1-5" || body["timezone"] != "Europe/Berlin" || body["intervalSeconds"] != 0 || body["graceSeconds"] != 300 {
		t.Fatalf("cron create: %v", body)
	}

	// A -f spec with a cron schedule gets no default interval either.
	body = map[string]interface{}{"name": "x", "cron": "@daily"}
	_ = applyHeartbeatFlags(newCmd(), body, true)
	if _, set := body["intervalSeconds"]; set {
		t.Fatalf("cron spec got an interval: %v", body)
	}

	// --interval on a cron heartbeat switches it back.
	body = map[string]interface{}{"name": "x", "intervalSeconds": 0.0, "cron": "@daily", "timezone": "UTC"}
	if err := applyHeartbeatFlags(newCmd("--interval", "1h"), body, false); err != nil {
		t.Fatal(err)
	}
	if body["cron"] != "" || body["intervalSeconds"] != 3600 {
		t.Fatalf("switch to interval: %v", body)
	}

	for _, args := range [][]string{{"--cron", "@daily", "--interval", "1h"}, {"--cron", " "}} {
		if err := applyHeartbeatFlags(newCmd(args...), map[string]interface{}{}, true); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}

func TestHeartbeatSchedule(t *testing.T) {
	if got := heartbeatSchedule(map[string]interface{}{"intervalSeconds": 3600.0}); got != "1h" {
		t.Errorf("interval: %q", got)
	}
	if got := heartbeatSchedule(map[string]interface{}{"intervalSeconds": 0.0, "cron": "0 3 * * *", "timezone": "Europe/Berlin"}); got != "0 3 * * * (Europe/Berlin)" {
		t.Errorf("cron: %q", got)
	}
}

func TestFormatSeconds(t *testing.T) {
	for in, want := range map[float64]string{43200: "12h", 5400: "1h30m", 1800: "30m", 90: "1m30s", 45: "45s", 3601: "1h1s", 0: "-"} {
		if got := formatSeconds(in); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}
