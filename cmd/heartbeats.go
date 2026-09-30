package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/config"
	"github.com/uptimy/uptimyctl/internal/output"
)

var heartbeatsCmd = &cobra.Command{
	Use:     "heartbeats",
	Aliases: []string{"heartbeat", "hb"},
	Short:   "Manage heartbeat monitors",
	Long: `Manage heartbeat monitors: dead man's switches for cron jobs, workers and
anything else that should check in on a schedule.

A heartbeat expects a check-in (an HTTP request to its ping URL) at least
every --interval, plus --grace. When check-ins stop, it goes down and alerts.

Heartbeats use a separate domain (default https://heartbeats.upti.my),
overridable with --heartbeats-api-url or UPTIMYCTL_HEARTBEATS_API_URL.`,
}

// Defaults match the web app's "Create heartbeat".
const (
	defaultHeartbeatInterval = time.Hour
	defaultHeartbeatGrace    = 5 * time.Minute
)

// parseSeconds accepts a duration ("90s", "5m", "1h", "24h") or plain seconds ("300").
func parseSeconds(flag, value string, allowZero bool) (int, error) {
	value = strings.TrimSpace(value)
	var seconds int
	if n, err := strconv.Atoi(value); err == nil {
		seconds = n
	} else {
		d, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid --%s %q: use a duration like 90s, 5m, 1h, or seconds", flag, value)
		}
		if d%time.Second != 0 {
			return 0, fmt.Errorf("invalid --%s %q: must be whole seconds", flag, value)
		}
		seconds = int(d / time.Second)
	}
	if seconds < 0 || (seconds == 0 && !allowZero) {
		return 0, fmt.Errorf("invalid --%s %q: must be greater than 0", flag, value)
	}
	return seconds, nil
}

// formatSeconds renders an interval compactly: 43200 -> "12h", 5400 -> "1h30m", 90 -> "1m30s".
func formatSeconds(v interface{}) string {
	n, ok := v.(float64)
	if !ok || n <= 0 {
		return "-"
	}
	total := int(n)
	var b strings.Builder
	for _, unit := range []struct {
		size   int
		suffix string
	}{{3600, "h"}, {60, "m"}, {1, "s"}} {
		if q := total / unit.size; q > 0 {
			fmt.Fprintf(&b, "%d%s", q, unit.suffix)
			total -= q * unit.size
		}
	}
	return b.String()
}

// heartbeatPingURL is where the monitored job sends its check-ins.
func heartbeatPingURL(hb map[string]interface{}) string {
	cred, _ := hb["credential"].(map[string]interface{})
	publicID := str(cred["publicId"])
	if publicID == "" {
		return ""
	}
	return strings.TrimRight(config.GetHeartbeatsURL(flagHeartbeatsURL), "/") + "/v1/monitors/" + url.PathEscape(publicID)
}

func heartbeatStatus(hb map[string]interface{}) string {
	if hb["pausedAt"] != nil {
		return "Paused"
	}
	snap, _ := hb["statusSnapshot"].(map[string]interface{})
	return output.ValueOrDash(str(snap["status"]))
}

func getHeartbeat(c *client.Client, uuid string) map[string]interface{} {
	raw, err := c.Get("/v1/api/heartbeat-monitors/"+url.PathEscape(uuid), nil)
	if err != nil {
		exitErr(err)
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		exitErr(err)
	}
	var hb map[string]interface{}
	if err := json.Unmarshal(data, &hb); err != nil {
		exitErr(err)
	}
	return hb
}

// heartbeatRequest turns a heartbeat as returned by the API into an update
// body. Updates replace the whole configuration (steps included), so every
// field is carried over and callers change only what they mean to.
func heartbeatRequest(hb map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{"paused": hb["pausedAt"] != nil}
	for _, k := range []string{"name", "description", "intervalSeconds", "graceSeconds", "failureThreshold", "recoveryThreshold", "enforceOrder", "steps"} {
		if v, ok := hb[k]; ok {
			body[k] = v
		}
	}
	return body
}

func putHeartbeat(c *client.Client, uuid string, body map[string]interface{}) json.RawMessage {
	raw, err := c.Put("/v1/api/heartbeat-monitors/"+url.PathEscape(uuid), body)
	if err != nil {
		exitErr(err)
	}
	data, _ := client.ParseDataField(raw)
	return data
}

// printHeartbeat shows a created or updated heartbeat: its ping URL for
// humans, the full object for -o json.
func printHeartbeat(action string, data json.RawMessage) {
	if output.IsJSON() {
		output.PrintRawJSON(data)
		return
	}
	var hb map[string]interface{}
	_ = json.Unmarshal(data, &hb)
	fmt.Printf("Heartbeat %s: %s\n", action, str(hb["uuid"]))
	if u := heartbeatPingURL(hb); u != "" {
		fmt.Printf("Ping URL:  %s\n", u)
	}
}

var heartbeatsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List heartbeats",
	Run: func(cmd *cobra.Command, args []string) {
		c := newHeartbeatsClient()
		raw, err := c.Get("/v1/api/heartbeat-monitors/", nil)
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
		var heartbeats []map[string]interface{}
		if err := json.Unmarshal(results, &heartbeats); err != nil {
			exitErr(err)
		}
		rows := make([][]string, 0, len(heartbeats))
		for _, hb := range heartbeats {
			snap, _ := hb["statusSnapshot"].(map[string]interface{})
			rows = append(rows, []string{
				str(hb["uuid"]),
				output.Truncate(str(hb["name"]), 40),
				heartbeatStatus(hb),
				formatSeconds(hb["intervalSeconds"]),
				formatSeconds(hb["graceSeconds"]),
				output.ValueOrDash(str(snap["lastPingAt"])),
			})
		}
		output.PrintTable([]string{"UUID", "Name", "Status", "Every", "Grace", "Last Check-in"}, rows)
	},
}

var heartbeatsGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get heartbeat details, including its ping URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hb := getHeartbeat(newHeartbeatsClient(), args[0])
		hb["pingUrl"] = heartbeatPingURL(hb)
		output.PrintJSON(hb)
	},
}

var heartbeatsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a heartbeat",
	Long: `Create a heartbeat and print its ping URL.

Defaults match the web app: expected every 1h, with 5m of grace.

For multi-step heartbeats or validation rules, pass the full JSON spec with
-f (or - for stdin), in the same shape as 'heartbeats get' returns.

--client-ref makes creation idempotent: creating again with the same ref
returns the existing heartbeat instead of a duplicate, which suits scripts
and infrastructure-as-code.

Examples:
  uptimyctl heartbeats create --name "Nightly backup" --interval 24h --grace 30m
  uptimyctl heartbeats create --name "Queue worker" --interval 1m --grace 30s -o json
  uptimyctl heartbeats create -f heartbeat.json -o json

Then call the ping URL at the end of every run:
  curl -fsS https://heartbeats.upti.my/v1/monitors/<id>`,
	Run: func(cmd *cobra.Command, args []string) {
		var body map[string]interface{}
		if file, _ := cmd.Flags().GetString("file"); file != "" {
			payload, err := readJSONInput(file)
			if err != nil {
				exitErr(err)
			}
			spec, ok := payload.(map[string]interface{})
			if !ok {
				exitErr(fmt.Errorf("the JSON spec must be an object"))
			}
			body = spec
		} else {
			name, _ := cmd.Flags().GetString("name")
			if strings.TrimSpace(name) == "" {
				exitErr(fmt.Errorf("--name is required (or pass a spec with -f)"))
			}
			body = map[string]interface{}{"name": name}
		}
		if err := applyHeartbeatFlags(cmd, body, true); err != nil {
			exitErr(err)
		}
		if ref, _ := cmd.Flags().GetString("client-ref"); ref != "" {
			body["clientRef"] = ref
		}

		raw, err := newHeartbeatsClient().Post("/v1/api/heartbeat-monitors/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		printHeartbeat("created", data)
	},
}

// applyHeartbeatFlags sets body fields from the flags the user passed. With
// withDefaults, unset interval/grace fall back to the web app's defaults
// unless the body (e.g. a -f spec) already has them.
func applyHeartbeatFlags(cmd *cobra.Command, body map[string]interface{}, withDefaults bool) error {
	for _, f := range []struct {
		flag, field string
		def         time.Duration
		allowZero   bool
	}{
		{"interval", "intervalSeconds", defaultHeartbeatInterval, false},
		{"grace", "graceSeconds", defaultHeartbeatGrace, true},
	} {
		if cmd.Flags().Changed(f.flag) {
			v, _ := cmd.Flags().GetString(f.flag)
			seconds, err := parseSeconds(f.flag, v, f.allowZero)
			if err != nil {
				return err
			}
			body[f.field] = seconds
		} else if _, set := body[f.field]; withDefaults && !set {
			body[f.field] = int(f.def / time.Second)
		}
	}
	for flag, field := range map[string]string{"name": "name", "description": "description"} {
		if cmd.Flags().Changed(flag) {
			body[field], _ = cmd.Flags().GetString(flag)
		}
	}
	for flag, field := range map[string]string{"failure-threshold": "failureThreshold", "recovery-threshold": "recoveryThreshold"} {
		if cmd.Flags().Changed(flag) {
			n, _ := cmd.Flags().GetInt(flag)
			if n < 1 {
				return fmt.Errorf("--%s must be at least 1", flag)
			}
			body[field] = n
		}
	}
	return nil
}

var heartbeatsUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update a heartbeat",
	Long: `Update a heartbeat. Only the flags you pass change; everything else,
including steps, is kept.

Example:
  uptimyctl heartbeats update <uuid> --interval 30m --grace 10m`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newHeartbeatsClient()
		body := heartbeatRequest(getHeartbeat(c, args[0]))
		if err := applyHeartbeatFlags(cmd, body, false); err != nil {
			exitErr(err)
		}
		printHeartbeat("updated", putHeartbeat(c, args[0], body))
	},
}

func setHeartbeatPaused(uuid string, paused bool) {
	c := newHeartbeatsClient()
	body := heartbeatRequest(getHeartbeat(c, uuid))
	body["paused"] = paused
	putHeartbeat(c, uuid, body)
	if paused {
		printActionResult("paused", "Heartbeat", uuid)
	} else {
		printActionResult("resumed", "Heartbeat", uuid)
	}
}

var heartbeatsPauseCmd = &cobra.Command{
	Use:   "pause <uuid>",
	Short: "Pause a heartbeat (stops alerting, e.g. during maintenance)",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { setHeartbeatPaused(args[0], true) },
}

var heartbeatsResumeCmd = &cobra.Command{
	Use:   "resume <uuid>",
	Short: "Resume a paused heartbeat",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { setHeartbeatPaused(args[0], false) },
}

var heartbeatsPingCmd = &cobra.Command{
	Use:   "ping <uuid>",
	Short: "Send a check-in, e.g. to test a new heartbeat",
	Long: `Send one check-in to a heartbeat, the same request a monitored job makes.

In scripts, prefer calling the ping URL directly (see 'heartbeats get'): it
needs no API key.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hb := getHeartbeat(newHeartbeatsClient(), args[0])
		pingURL := heartbeatPingURL(hb)
		if pingURL == "" {
			exitErr(fmt.Errorf("heartbeat has no ping URL"))
		}
		httpClient := &http.Client{Timeout: 15 * time.Second}
		resp, err := httpClient.Post(pingURL, "application/json", nil)
		if err != nil {
			exitErr(fmt.Errorf("check-in failed: %w", err))
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			if hb["pausedAt"] != nil {
				exitErr(fmt.Errorf("check-in rejected (HTTP %d): the heartbeat is paused", resp.StatusCode))
			}
			exitErr(fmt.Errorf("check-in rejected (HTTP %d)", resp.StatusCode))
		}
		printActionResult("pinged", "Heartbeat", args[0])
	},
}

var heartbeatsDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a heartbeat",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := newHeartbeatsClient().Delete("/v1/api/heartbeat-monitors/" + url.PathEscape(args[0])); err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "Heartbeat", args[0])
	},
}

func init() {
	for _, c := range []*cobra.Command{heartbeatsCreateCmd, heartbeatsUpdateCmd} {
		c.Flags().String("name", "", "Heartbeat name")
		c.Flags().String("description", "", "Description")
		c.Flags().String("interval", "", "Expected check-in interval, e.g. 5m, 1h, 24h (default 1h on create)")
		c.Flags().String("grace", "", "Extra time allowed after the interval, e.g. 30s, 5m (default 5m on create)")
		c.Flags().Int("failure-threshold", 0, "Missed check-ins before it goes down (default 1)")
		c.Flags().Int("recovery-threshold", 0, "Check-ins needed to recover (default 1)")
	}
	heartbeatsCreateCmd.Flags().StringP("file", "f", "", "Full JSON spec (path, or - for stdin); flags override its fields")
	heartbeatsCreateCmd.Flags().String("client-ref", "", "Idempotency key: reuse the heartbeat created earlier with this ref")

	heartbeatsCmd.AddCommand(heartbeatsListCmd)
	heartbeatsCmd.AddCommand(heartbeatsGetCmd)
	heartbeatsCmd.AddCommand(heartbeatsCreateCmd)
	heartbeatsCmd.AddCommand(heartbeatsUpdateCmd)
	heartbeatsCmd.AddCommand(heartbeatsPauseCmd)
	heartbeatsCmd.AddCommand(heartbeatsResumeCmd)
	heartbeatsCmd.AddCommand(heartbeatsPingCmd)
	heartbeatsCmd.AddCommand(heartbeatsDeleteCmd)
	rootCmd.AddCommand(heartbeatsCmd)
}
