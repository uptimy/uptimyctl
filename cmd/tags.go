package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

// workspaceTag is one of the workspace's tags (upti.my-api).
type workspaceTag struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List the workspace's tags",
	Long: `List the workspace's tags. Tag healthchecks and heartbeats with
'healthchecks tag' and 'heartbeats tag'; create tags in the app.`,
}

var tagsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tags",
	Run: func(cmd *cobra.Command, args []string) {
		raw, err := newClient().Get("/v1/api/tags/", nil)
		if err != nil {
			exitErr(err)
		}
		if output.IsJSON() {
			output.PrintJSONBytes(raw)
			return
		}
		tags, err := parseTags(raw)
		if err != nil {
			exitErr(err)
		}
		rows := make([][]string, 0, len(tags))
		for _, t := range tags {
			rows = append(rows, []string{strconv.Itoa(t.ID), t.Name, output.ValueOrDash(t.Color)})
		}
		output.PrintTable([]string{"ID", "Name", "Color"}, rows)
	},
}

func parseTags(raw []byte) ([]workspaceTag, error) {
	results, err := client.ParseResultsField(raw)
	if err != nil {
		return nil, err
	}
	var tags []workspaceTag
	if err := json.Unmarshal(results, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// fetchTags reads the workspace's tags.
func fetchTags() ([]workspaceTag, error) {
	raw, err := newClient().Get("/v1/api/tags/", nil)
	if err != nil {
		return nil, err
	}
	return parseTags(raw)
}

// resolveTagIDs turns tag names (any case) or ids into the workspace's tag
// ids, each once and in the order given. Unknown tags are an error that lists
// the known ones.
func resolveTagIDs(refs []string, tags []workspaceTag) ([]int, error) {
	ids := make([]int, 0, len(refs))
	seen := map[int]bool{}
	var unknown []string
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		id, found := 0, false
		for _, t := range tags {
			if strings.EqualFold(t.Name, ref) || strconv.Itoa(t.ID) == ref {
				id, found = t.ID, true
				break
			}
		}
		if !found {
			unknown = append(unknown, ref)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(unknown) > 0 {
		names := make([]string, 0, len(tags))
		for _, t := range tags {
			names = append(names, t.Name)
		}
		sort.Strings(names)
		known := "none yet; create tags in the app"
		if len(names) > 0 {
			known = strings.Join(names, ", ")
		}
		return nil, fmt.Errorf("unknown tag(s): %s (known: %s)", strings.Join(unknown, ", "), known)
	}
	return ids, nil
}

// tagIDsFromFlags reads --tag/--clear: the tag ids to set, which may be none
// only with --clear.
func tagIDsFromFlags(cmd *cobra.Command) ([]int, error) {
	refs, _ := cmd.Flags().GetStringSlice("tag")
	clearAll, _ := cmd.Flags().GetBool("clear")
	if clearAll == (len(refs) > 0) {
		return nil, fmt.Errorf("give the tags with --tag, or --clear to remove them all")
	}
	if clearAll {
		return []int{}, nil
	}
	tags, err := fetchTags()
	if err != nil {
		return nil, err
	}
	return resolveTagIDs(refs, tags)
}

const tagCommandHelp = `Replaces the %[1]s's tags with the ones given, by name or id; --clear
removes them all. Tags filter lists and workflow Filter nodes.

Examples:
  uptimyctl %[1]ss tag <uuid> --tag production --tag api
  uptimyctl %[1]ss tag <uuid> --tag production,api
  uptimyctl %[1]ss tag <uuid> --clear`

var healthchecksTagCmd = &cobra.Command{
	Use:   "tag <uuid>",
	Short: "Set a healthcheck's tags",
	Long:  fmt.Sprintf(tagCommandHelp, "healthcheck"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ids, err := tagIDsFromFlags(cmd)
		if err != nil {
			exitErr(err)
		}
		c := newClient()
		// Tags are set per application, so find the healthcheck's.
		raw, err := c.Get("/v1/api/healthchecks/"+url.PathEscape(args[0]), nil)
		if err != nil {
			exitErr(err)
		}
		data, err := client.ParseDataField(raw)
		if err != nil {
			exitErr(err)
		}
		var hc struct {
			ApplicationUUID string `json:"applicationUuid"`
		}
		if err := json.Unmarshal(data, &hc); err != nil || hc.ApplicationUUID == "" {
			exitErr(fmt.Errorf("healthcheck %s has no application", args[0]))
		}
		if _, err := c.Put("/v1/api/applications/"+url.PathEscape(hc.ApplicationUUID)+"/healthchecks/tags/", map[string]interface{}{"tagIds": ids}); err != nil {
			exitErr(err)
		}
		printActionResult("tagged", "healthcheck", args[0])
	},
}

var heartbeatsTagCmd = &cobra.Command{
	Use:   "tag <uuid>",
	Short: "Set a heartbeat's tags",
	Long:  fmt.Sprintf(tagCommandHelp, "heartbeat"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ids, err := tagIDsFromFlags(cmd)
		if err != nil {
			exitErr(err)
		}
		if _, err := newHeartbeatsClient().Put("/v1/api/heartbeat-monitors/"+url.PathEscape(args[0])+"/tags", map[string]interface{}{"tagIds": ids}); err != nil {
			exitErr(err)
		}
		printActionResult("tagged", "heartbeat", args[0])
	},
}

func init() {
	tagsCmd.AddCommand(tagsListCmd)
	rootCmd.AddCommand(tagsCmd)

	for _, c := range []*cobra.Command{healthchecksTagCmd, heartbeatsTagCmd} {
		c.Flags().StringSlice("tag", nil, "Tag name or id; repeat or comma-separate for several")
		c.Flags().Bool("clear", false, "Remove all tags")
	}
	healthchecksCmd.AddCommand(healthchecksTagCmd)
	heartbeatsCmd.AddCommand(heartbeatsTagCmd)
}
