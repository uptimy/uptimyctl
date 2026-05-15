package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var healthchecksCmd = &cobra.Command{
	Use:     "healthchecks",
	Aliases: []string{"hc"},
	Short:   "Manage healthchecks",
}

var healthchecksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all healthchecks",
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/healthchecks/", nil)
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

		var healthchecks []map[string]interface{}
		if err := json.Unmarshal(results, &healthchecks); err != nil {
			exitErr(err)
		}

		rows := make([][]string, 0, len(healthchecks))
		for _, hc := range healthchecks {
			appName := ""
			if app, ok := hc["application"].(map[string]interface{}); ok {
				appName = str(app["name"])
			}
			active := "No"
			if b, ok := hc["active"].(bool); ok && b {
				active = "Yes"
			}
			rows = append(rows, []string{
				str(hc["uuid"]),
				appName,
				fmt.Sprintf("%vs", str(hc["intervalSeconds"])),
				active,
			})
		}
		output.PrintTable([]string{"UUID", "Application", "Interval", "Active"}, rows)
	},
}

var healthchecksGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get healthcheck details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		raw, err := c.Get("/v1/api/healthchecks/"+args[0], nil)
		if err != nil {
			exitErr(err)
		}
		data, _ := client.ParseDataField(raw)
		output.PrintRawJSON(data)
	},
}

var healthchecksTriggerCmd = &cobra.Command{
	Use:   "trigger <uuid>",
	Short: "Trigger an immediate healthcheck execution",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Patch("/v1/api/healthchecks/"+args[0]+"/trigger", nil)
		if err != nil {
			exitErr(err)
		}
		printActionResult("triggered", "healthcheck", args[0])
	},
}

var healthchecksPauseCmd = &cobra.Command{
	Use:   "pause <uuid>",
	Short: "Pause a healthcheck",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := setHealthcheckActive(args[0], false); err != nil {
			exitErr(err)
		}
		printActionResult("paused", "healthcheck", args[0])
	},
}

var healthchecksResumeCmd = &cobra.Command{
	Use:   "resume <uuid>",
	Short: "Resume a paused healthcheck",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := setHealthcheckActive(args[0], true); err != nil {
			exitErr(err)
		}
		printActionResult("resumed", "healthcheck", args[0])
	},
}

var healthchecksDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a healthcheck",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		_, err := c.Delete("/v1/api/healthchecks/" + args[0])
		if err != nil {
			exitErr(err)
		}
		printActionResult("deleted", "healthcheck", args[0])
	},
}

func init() {
	healthchecksCmd.AddCommand(healthchecksListCmd)
	healthchecksCmd.AddCommand(healthchecksGetCmd)
	healthchecksCmd.AddCommand(healthchecksTriggerCmd)
	healthchecksCmd.AddCommand(healthchecksPauseCmd)
	healthchecksCmd.AddCommand(healthchecksResumeCmd)
	healthchecksCmd.AddCommand(healthchecksDeleteCmd)
	rootCmd.AddCommand(healthchecksCmd)
}

func setHealthcheckActive(uuid string, active bool) error {
	c := newClient()

	raw, err := c.Get("/v1/api/healthchecks/"+uuid, nil)
	if err != nil {
		return err
	}

	data, err := client.ParseDataField(raw)
	if err != nil {
		return err
	}

	var hc map[string]interface{}
	if err := json.Unmarshal(data, &hc); err != nil {
		return err
	}

	healthcheckTypeID := hc["healthcheckTypeId"]
	if healthcheckTypeID == nil {
		if healthcheckType, ok := hc["healthcheckType"].(map[string]interface{}); ok {
			healthcheckTypeID = healthcheckType["id"]
		}
	}
	if healthcheckTypeID == nil {
		return fmt.Errorf("missing healthcheckTypeId in healthcheck payload")
	}

	body := map[string]interface{}{
		"intervalSeconds":   hc["intervalSeconds"],
		"timeoutSeconds":    hc["timeoutSeconds"],
		"config":            hc["config"],
		"active":            active,
		"healthcheckTypeId": healthcheckTypeID,
		"schedulers":        hc["schedulers"],
	}

	if tags, ok := hc["tags"].([]interface{}); ok {
		normalizedTags := make([]map[string]interface{}, 0, len(tags))
		for _, tagRaw := range tags {
			if tag, ok := tagRaw.(map[string]interface{}); ok {
				if id, exists := tag["id"]; exists {
					normalizedTags = append(normalizedTags, map[string]interface{}{"id": id})
				}
			}
		}
		body["tags"] = normalizedTags
	}

	_, err = c.Put("/v1/api/healthchecks/"+uuid, body)
	return err
}
