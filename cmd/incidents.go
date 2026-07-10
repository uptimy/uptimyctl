package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Manage incidents",
	Long: `Manage incidents and their public communication.

Incident lifecycle: Created -> Acknowledged -> Investigating -> Identified -> Monitoring -> Resolved.

An incident becomes visible on a status page once it is attached to it
(--status-page on create, or 'incidents publish'). Individual updates are
shown publicly only when posted with --public=true.`,
}

// fetchIncident retrieves the current state of an incident as a generic map.
func fetchIncident(c *client.Client, uuid string) (map[string]interface{}, error) {
	raw, err := c.Get("/v1/api/incidents/"+uuid, nil)
	if err != nil {
		return nil, err
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		return nil, err
	}
	var inc map[string]interface{}
	if err := json.Unmarshal(data, &inc); err != nil {
		return nil, err
	}
	return inc, nil
}

// incidentStatusPageUuids extracts the status page UUIDs an incident is published on.
func incidentStatusPageUuids(inc map[string]interface{}) []string {
	uuids := []string{}
	pages, ok := inc["incidentStatusPages"].([]interface{})
	if !ok {
		return uuids
	}
	for _, p := range pages {
		if page, ok := p.(map[string]interface{}); ok {
			if uuid := str(page["statusPageUuid"]); uuid != "" {
				uuids = append(uuids, uuid)
			}
		}
	}
	return uuids
}

// incidentPutBody builds a full PUT payload from the incident's current state.
// The API's incident update is a full replace of title/description/severity/status,
// so every PUT must carry all of them. statusPageUuids is intentionally omitted:
// when absent the server leaves the published set untouched.
func incidentPutBody(inc map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"title":       inc["title"],
		"severity":    inc["severity"],
		"status":      inc["status"],
		"description": inc["description"],
	}
	return body
}

func putIncident(c *client.Client, uuid string, body map[string]interface{}) {
	raw, err := c.Put("/v1/api/incidents/"+uuid, body)
	if err != nil {
		exitErr(err)
	}
	data, _ := client.ParseDataField(raw)
	output.PrintRawJSON(data)
}

var incidentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List incidents",
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		q := url.Values{}
		if v, _ := cmd.Flags().GetString("status"); v != "" {
			q.Set("status", v)
		}
		if v, _ := cmd.Flags().GetString("severity"); v != "" {
			q.Set("severity", v)
		}
		if v, _ := cmd.Flags().GetString("source"); v != "" {
			q.Set("sourceUuid", v)
		}
		if cmd.Flags().Changed("manual") {
			v, _ := cmd.Flags().GetBool("manual")
			q.Set("manual", fmt.Sprintf("%t", v))
		}
		if v, _ := cmd.Flags().GetInt("page"); v > 0 {
			q.Set("page", fmt.Sprintf("%d", v))
		}
		if v, _ := cmd.Flags().GetInt("per-page"); v > 0 {
			q.Set("perPage", fmt.Sprintf("%d", v))
		}

		raw, err := c.Get("/v1/api/incidents/", q)
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

		var incidents []map[string]interface{}
		if err := json.Unmarshal(results, &incidents); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(incidents))
		for _, inc := range incidents {
			rows = append(rows, []string{
				str(inc["uuid"]),
				output.Truncate(str(inc["title"]), 40),
				str(inc["status"]),
				str(inc["severity"]),
				fmt.Sprintf("%d", len(incidentStatusPageUuids(inc))),
				output.ValueOrDash(str(inc["resolvedAt"])),
			})
		}
		output.PrintTable([]string{"UUID", "Title", "Status", "Severity", "Pages", "Resolved At"}, rows)
	},
}

var incidentsGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get incident details (includes updates, status history, and status pages)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		raw, err := c.Get("/v1/api/incidents/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Get incident statistics",
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		raw, err := c.Get("/v1/api/incidents/stats", nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new incident",
	Long: `Create a new incident.

The incident starts in status "Created". Attach it to one or more status
pages with --status-page to make it publicly visible.

Example:
  uptimyctl incidents create --title "API latency" --severity high \
    --description "Elevated p99 latency on /v1/search" \
    --status-page <status-page-uuid> -o json`,
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		severity, _ := cmd.Flags().GetString("severity")
		errorMessage, _ := cmd.Flags().GetString("error-message")
		statusPages, _ := cmd.Flags().GetStringSlice("status-page")

		body := map[string]interface{}{
			"title":    title,
			"severity": severity,
		}
		if description != "" {
			body["description"] = description
		}
		if errorMessage != "" {
			body["errorMessage"] = errorMessage
		}
		if cmd.Flags().Changed("status-page") {
			body["statusPageUuids"] = statusPages
		}

		raw, err := c.Post("/v1/api/incidents/", body)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update an incident (only the flags you pass are changed)",
	Long: `Update an incident. Only the flags you pass are changed; everything else
keeps its current value.

--status-page replaces the full set of status pages the incident is shown
on; use 'incidents publish'/'incidents unpublish' to add or remove pages
without replacing the set.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}
		body := incidentPutBody(inc)

		if cmd.Flags().Changed("title") {
			body["title"], _ = cmd.Flags().GetString("title")
		}
		if cmd.Flags().Changed("description") {
			body["description"], _ = cmd.Flags().GetString("description")
		}
		if cmd.Flags().Changed("severity") {
			body["severity"], _ = cmd.Flags().GetString("severity")
		}
		if cmd.Flags().Changed("status") {
			body["status"], _ = cmd.Flags().GetString("status")
		}
		if cmd.Flags().Changed("status-page") {
			pages, _ := cmd.Flags().GetStringSlice("status-page")
			body["statusPageUuids"] = pages
		}

		putIncident(c, args[0], body)
	},
}

var incidentsResolveCmd = &cobra.Command{
	Use:   "resolve <uuid>",
	Short: "Resolve an incident, optionally posting a final public update",
	Long: `Resolve an incident. With --message, a public update is posted first so
status page followers see the resolution note.

Note: only manually created incidents can be resolved this way; incidents
opened automatically by monitoring resolve themselves.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		if message, _ := cmd.Flags().GetString("message"); message != "" {
			_, err := c.Post("/v1/api/incidents/"+args[0]+"/updates/", map[string]interface{}{
				"message": message,
				"public":  true,
			})
			if err != nil {
				exitErr(err)
			}
		}

		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}
		body := incidentPutBody(inc)
		body["status"] = "Resolved"

		putIncident(c, args[0], body)
	},
}

var incidentsAddUpdateCmd = &cobra.Command{
	Use:   "add-update <incident-uuid>",
	Short: "Post an update to an incident's timeline",
	Long: `Post an update to an incident's timeline. Public updates are shown on
every status page the incident is published on.

Posting an update to an incident still in status "Created" automatically
moves it to "Acknowledged". Pass --status to transition the incident to a
specific status in the same command.

Example:
  uptimyctl incidents add-update <uuid> \
    --message "Root cause identified, rolling out a fix." \
    --status Identified -o json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		message, _ := cmd.Flags().GetString("message")
		public, _ := cmd.Flags().GetBool("public")

		body := map[string]interface{}{
			"message": message,
			"public":  public,
		}

		raw, err := c.Post("/v1/api/incidents/"+args[0]+"/updates/", body)
		if err != nil {
			exitErr(err)
		}

		if status, _ := cmd.Flags().GetString("status"); status != "" {
			inc, err := fetchIncident(c, args[0])
			if err != nil {
				exitErr(err)
			}
			putBody := incidentPutBody(inc)
			putBody["status"] = status
			putIncident(c, args[0], putBody)
			return
		}

		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var incidentsUpdatesCmd = &cobra.Command{
	Use:   "updates <incident-uuid>",
	Short: "List an incident's timeline updates",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()
		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}

		updates, _ := inc["incidentUpdates"].([]interface{})

		if output.IsJSON() {
			output.PrintJSON(updates)
			return
		}

		rows := make([][]string, 0, len(updates))
		for _, u := range updates {
			upd, ok := u.(map[string]interface{})
			if !ok {
				continue
			}
			public := "No"
			if b, ok := upd["public"].(bool); ok && b {
				public = "Yes"
			}
			rows = append(rows, []string{
				str(upd["uuid"]),
				str(upd["createdAt"]),
				public,
				output.Truncate(strings.ReplaceAll(str(upd["message"]), "\n", " "), 60),
			})
		}
		output.PrintTable([]string{"UUID", "Created At", "Public", "Message"}, rows)
	},
}

var incidentsPublishCmd = &cobra.Command{
	Use:   "publish <incident-uuid>",
	Short: "Publish an incident to one or more status pages",
	Long: `Publish an incident to one or more status pages, keeping any pages it is
already published on.

Example:
  uptimyctl incidents publish <uuid> --status-page <status-page-uuid>`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		pages, _ := cmd.Flags().GetStringSlice("status-page")

		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}

		current := incidentStatusPageUuids(inc)
		seen := make(map[string]bool, len(current))
		merged := make([]string, 0, len(current)+len(pages))
		for _, uuid := range append(current, pages...) {
			if !seen[uuid] {
				seen[uuid] = true
				merged = append(merged, uuid)
			}
		}

		body := incidentPutBody(inc)
		body["statusPageUuids"] = merged
		putIncident(c, args[0], body)
	},
}

var incidentsUnpublishCmd = &cobra.Command{
	Use:   "unpublish <incident-uuid>",
	Short: "Remove an incident from status pages",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		pages, _ := cmd.Flags().GetStringSlice("status-page")
		all, _ := cmd.Flags().GetBool("all")
		if !all && len(pages) == 0 {
			exitErr(fmt.Errorf("pass --status-page <uuid> to remove specific pages, or --all to remove all"))
		}

		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}

		remaining := []string{}
		if !all {
			remove := make(map[string]bool, len(pages))
			for _, uuid := range pages {
				remove[uuid] = true
			}
			for _, uuid := range incidentStatusPageUuids(inc) {
				if !remove[uuid] {
					remaining = append(remaining, uuid)
				}
			}
		}

		body := incidentPutBody(inc)
		body["statusPageUuids"] = remaining
		putIncident(c, args[0], body)
	},
}

var incidentsPostMortemCmd = &cobra.Command{
	Use:   "post-mortem <incident-uuid>",
	Short: "Attach a post-mortem to an incident",
	Long: `Attach a post-mortem to an incident. The text is appended to the incident
description under a "## Post-mortem" heading and, unless --no-public-update
is set, also posted as a public update so status page followers see it.

Provide the text with --message, or --file to read it from a file
(use --file - for stdin).

Example:
  uptimyctl incidents post-mortem <uuid> --file post-mortem.md -o json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newIncidentsClient()

		message, _ := cmd.Flags().GetString("message")
		file, _ := cmd.Flags().GetString("file")
		noPublicUpdate, _ := cmd.Flags().GetBool("no-public-update")

		if (message == "") == (file == "") {
			exitErr(fmt.Errorf("provide the post-mortem via exactly one of --message or --file"))
		}
		if file != "" {
			path := file
			if path == "-" {
				path = "/dev/stdin"
			}
			data, err := os.ReadFile(path)
			if err != nil {
				exitErr(fmt.Errorf("read post-mortem: %w", err))
			}
			message = strings.TrimSpace(string(data))
		}
		if message == "" {
			exitErr(fmt.Errorf("post-mortem text is empty"))
		}

		inc, err := fetchIncident(c, args[0])
		if err != nil {
			exitErr(err)
		}

		description := str(inc["description"])
		if description != "" {
			description += "\n\n"
		}
		description += "## Post-mortem\n\n" + message

		if !noPublicUpdate {
			_, err := c.Post("/v1/api/incidents/"+args[0]+"/updates/", map[string]interface{}{
				"message": message,
				"public":  true,
			})
			if err != nil {
				exitErr(err)
			}
		}

		body := incidentPutBody(inc)
		body["description"] = description
		putIncident(c, args[0], body)
	},
}

func init() {
	// list flags
	incidentsListCmd.Flags().String("status", "", "Filter: Ongoing, Investigating, Resolved")
	incidentsListCmd.Flags().String("severity", "", "Filter: critical, high, medium, low")
	incidentsListCmd.Flags().String("source", "", "Filter by source entity UUID (e.g. application)")
	incidentsListCmd.Flags().Bool("manual", false, "Filter by manually created (true) or automatic (false)")
	incidentsListCmd.Flags().Int("page", 1, "Page number")
	incidentsListCmd.Flags().Int("per-page", 20, "Results per page")

	// create flags
	incidentsCreateCmd.Flags().String("title", "", "Incident title (required)")
	incidentsCreateCmd.Flags().String("description", "", "Incident description")
	incidentsCreateCmd.Flags().String("severity", "high", "Severity: critical, high, medium, low")
	incidentsCreateCmd.Flags().String("error-message", "", "Technical error message to record on the incident")
	incidentsCreateCmd.Flags().StringSlice("status-page", nil, "Status page UUID to publish on (repeatable)")
	_ = incidentsCreateCmd.MarkFlagRequired("title")

	// update flags
	incidentsUpdateCmd.Flags().String("title", "", "Incident title")
	incidentsUpdateCmd.Flags().String("description", "", "Incident description")
	incidentsUpdateCmd.Flags().String("severity", "", "Severity: critical, high, medium, low")
	incidentsUpdateCmd.Flags().String("status", "", "Status: Created, Acknowledged, Investigating, Identified, Monitoring, Resolved")
	incidentsUpdateCmd.Flags().StringSlice("status-page", nil, "Replace the set of status pages (repeatable)")

	// resolve flags
	incidentsResolveCmd.Flags().String("message", "", "Post this as a final public update before resolving")

	// add-update flags
	incidentsAddUpdateCmd.Flags().String("message", "", "Update message (required)")
	incidentsAddUpdateCmd.Flags().Bool("public", true, "Show the update on status pages")
	incidentsAddUpdateCmd.Flags().String("status", "", "Also transition the incident to this status")
	_ = incidentsAddUpdateCmd.MarkFlagRequired("message")

	// publish/unpublish flags
	incidentsPublishCmd.Flags().StringSlice("status-page", nil, "Status page UUID to publish on (repeatable, required)")
	_ = incidentsPublishCmd.MarkFlagRequired("status-page")
	incidentsUnpublishCmd.Flags().StringSlice("status-page", nil, "Status page UUID to remove (repeatable)")
	incidentsUnpublishCmd.Flags().Bool("all", false, "Remove the incident from all status pages")

	// post-mortem flags
	incidentsPostMortemCmd.Flags().String("message", "", "Post-mortem text")
	incidentsPostMortemCmd.Flags().StringP("file", "f", "", "Read post-mortem text from a file, or - for stdin")
	incidentsPostMortemCmd.Flags().Bool("no-public-update", false, "Only update the description; skip posting a public update")

	incidentsCmd.AddCommand(incidentsListCmd)
	incidentsCmd.AddCommand(incidentsGetCmd)
	incidentsCmd.AddCommand(incidentsStatsCmd)
	incidentsCmd.AddCommand(incidentsCreateCmd)
	incidentsCmd.AddCommand(incidentsUpdateCmd)
	incidentsCmd.AddCommand(incidentsResolveCmd)
	incidentsCmd.AddCommand(incidentsAddUpdateCmd)
	incidentsCmd.AddCommand(incidentsUpdatesCmd)
	incidentsCmd.AddCommand(incidentsPublishCmd)
	incidentsCmd.AddCommand(incidentsUnpublishCmd)
	incidentsCmd.AddCommand(incidentsPostMortemCmd)
	rootCmd.AddCommand(incidentsCmd)
}

// helper to safely convert interface{} to string
func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
