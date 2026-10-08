package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/output"
)

var dependenciesCmd = &cobra.Command{
	Use:     "dependencies",
	Aliases: []string{"dependency", "deps"},
	Short:   "Follow third-party services (GitHub, Cloudflare, Stripe, ...) you depend on",
	Long: `Follow the public services Uptimy measures (www.upti.my/status). When a
followed service goes down, your workspace gets its own incident and your
alert workflows run, without adding monitors.

A service has several probes (e.g. GitHub's website, API and git operations).
Follow all of them, or only the ones you use with --probe.

The catalog is public; following uses the workflows service (default
https://workflows.upti.my, see --incidents-api-url).`,
}

// publicService is a catalog entry from the API's /v1/public/services.
type publicService struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	WorkspaceID int     `json:"workspaceId"`
	Status      string  `json:"status"`
	ProbeCount  int     `json:"probeCount"`
	Uptime30    float64 `json:"uptime30"`
}

// publicProbe is one probe of a service, from /v1/public/services/:slug.
type publicProbe struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Meaning         string `json:"meaning"`
	ApplicationUUID string `json:"applicationUuid"`
	Status          string `json:"status"`
	Uptime          struct {
		D7 *float64 `json:"d7"`
	} `json:"uptime"`
}

type sourceFollow struct {
	SourceWorkspaceID int    `json:"sourceWorkspaceId"`
	SourceUUID        string `json:"sourceUuid"`
}

func fetchCatalog(c *client.Client) []publicService {
	raw, err := c.Get("/v1/public/services", nil)
	if err != nil {
		exitErr(err)
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		exitErr(err)
	}
	var catalog struct {
		Services []publicService `json:"services"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		exitErr(err)
	}
	return catalog.Services
}

func fetchService(c *client.Client, slug string) (publicService, []publicProbe) {
	raw, err := c.Get("/v1/public/services/"+url.PathEscape(slug), nil)
	if err != nil {
		exitErr(fmt.Errorf("%w (see 'uptimyctl dependencies catalog' for the services you can follow)", err))
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		exitErr(err)
	}
	var detail struct {
		publicService
		Probes []publicProbe `json:"probes"`
	}
	if err := json.Unmarshal(data, &detail); err != nil {
		exitErr(err)
	}
	return detail.publicService, detail.Probes
}

// fetchFollows returns the followed probe UUIDs, grouped by service workspace.
func fetchFollows(c *client.Client) map[int]map[string]bool {
	raw, err := c.Get("/v1/api/source-follows/", nil)
	if err != nil {
		exitErr(err)
	}
	results, err := client.ParseResultsField(raw)
	if err != nil {
		exitErr(err)
	}
	var follows []sourceFollow
	if err := json.Unmarshal(results, &follows); err != nil {
		exitErr(err)
	}
	out := map[int]map[string]bool{}
	for _, f := range follows {
		if out[f.SourceWorkspaceID] == nil {
			out[f.SourceWorkspaceID] = map[string]bool{}
		}
		out[f.SourceWorkspaceID][f.SourceUUID] = true
	}
	return out
}

func followingLabel(followed, total int) string {
	switch {
	case followed == 0:
		return "-"
	case followed >= total:
		return "all " + strconv.Itoa(total)
	default:
		return fmt.Sprintf("%d of %d", followed, total)
	}
}

var dependenciesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the services this workspace follows, with their current status",
	Run: func(cmd *cobra.Command, args []string) {
		follows := fetchFollows(newIncidentsClient())
		catalog := fetchCatalog(newClient())

		type row struct {
			Slug           string `json:"slug"`
			Name           string `json:"name"`
			Status         string `json:"status"`
			FollowedProbes int    `json:"followedProbes"`
			Probes         int    `json:"probes"`
		}
		rows := []row{}
		for _, s := range catalog {
			if n := len(follows[s.WorkspaceID]); n > 0 {
				rows = append(rows, row{s.Slug, s.Name, s.Status, n, s.ProbeCount})
			}
		}
		if output.IsJSON() {
			output.PrintJSON(rows)
			return
		}
		if len(rows) == 0 {
			fmt.Println("Not following any services. See 'uptimyctl dependencies catalog'.")
			return
		}
		table := make([][]string, 0, len(rows))
		for _, r := range rows {
			table = append(table, []string{r.Slug, r.Name, r.Status, followingLabel(r.FollowedProbes, r.Probes)})
		}
		output.PrintTable([]string{"Slug", "Service", "Status", "Following"}, table)
	},
}

var dependenciesCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List every service you can follow, with its status and 30-day uptime",
	Run: func(cmd *cobra.Command, args []string) {
		follows := fetchFollows(newIncidentsClient())
		catalog := fetchCatalog(newClient())
		if output.IsJSON() {
			type row struct {
				publicService
				FollowedProbes int `json:"followedProbes"`
			}
			rows := make([]row, 0, len(catalog))
			for _, s := range catalog {
				rows = append(rows, row{s, len(follows[s.WorkspaceID])})
			}
			output.PrintJSON(rows)
			return
		}
		table := make([][]string, 0, len(catalog))
		for _, s := range catalog {
			table = append(table, []string{s.Slug, s.Name, s.Status, fmt.Sprintf("%.2f%%", s.Uptime30), followingLabel(len(follows[s.WorkspaceID]), s.ProbeCount)})
		}
		output.PrintTable([]string{"Slug", "Service", "Status", "Uptime 30d", "Following"}, table)
	},
}

var dependenciesGetCmd = &cobra.Command{
	Use:   "get <slug>",
	Short: "Show a service's probes, their status, and which ones you follow",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service, probes := fetchService(newClient(), args[0])
		followed := fetchFollows(newIncidentsClient())[service.WorkspaceID]
		if output.IsJSON() {
			type probe struct {
				publicProbe
				Followed bool `json:"followed"`
			}
			out := struct {
				publicService
				Probes []probe `json:"probes"`
			}{publicService: service}
			for _, p := range probes {
				out.Probes = append(out.Probes, probe{p, followed[p.ApplicationUUID]})
			}
			output.PrintJSON(out)
			return
		}
		fmt.Printf("%s (%s): %s\n\n", service.Name, service.Slug, service.Status)
		table := make([][]string, 0, len(probes))
		for _, p := range probes {
			uptime := "-"
			if p.Uptime.D7 != nil {
				uptime = fmt.Sprintf("%.2f%%", *p.Uptime.D7)
			}
			follows := "no"
			if followed[p.ApplicationUUID] {
				follows = "yes"
			}
			table = append(table, []string{p.Key, p.Label, p.Status, uptime, follows})
		}
		output.PrintTable([]string{"Probe", "Label", "Status", "Uptime 7d", "Followed"}, table)
	},
}

var dependenciesFollowCmd = &cobra.Command{
	Use:   "follow <slug>",
	Short: "Follow a service: all its probes, or only the ones given with --probe",
	Long: `Follow a service. Without --probe, every probe is followed; with --probe,
exactly the given probes are (replacing what was followed before for this
service). Probe keys are listed by 'uptimyctl dependencies get <slug>'.

Example:
  uptimyctl dependencies follow github
  uptimyctl dependencies follow github --probe rest-api --probe git-operations`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service, probes := fetchService(newClient(), args[0])
		keys, _ := cmd.Flags().GetStringSlice("probe")

		byKey := map[string]string{}
		all := make([]string, 0, len(probes))
		for _, p := range probes {
			byKey[p.Key] = p.ApplicationUUID
			all = append(all, p.Key)
		}
		uuids := []string{}
		if len(keys) == 0 {
			for _, p := range probes {
				uuids = append(uuids, p.ApplicationUUID)
			}
		} else {
			for _, k := range keys {
				uuid, ok := byKey[strings.TrimSpace(k)]
				if !ok {
					exitErr(fmt.Errorf("%s has no probe %q (probes: %s)", service.Name, k, strings.Join(all, ", ")))
				}
				uuids = append(uuids, uuid)
			}
		}

		raw, err := newIncidentsClient().Put("/v1/api/source-follows/", map[string]interface{}{
			"sourceWorkspaceId": service.WorkspaceID,
			"sourceUuids":       uuids,
		})
		if err != nil {
			exitErr(err)
		}
		if output.IsJSON() {
			output.PrintJSONBytes(raw)
			return
		}
		fmt.Printf("Following %s (%s).\n", service.Name, followingLabel(len(uuids), len(probes)))
	},
}

var dependenciesUnfollowCmd = &cobra.Command{
	Use:   "unfollow <slug>",
	Short: "Stop following a service (resolves its open incident in this workspace)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service, _ := fetchService(newClient(), args[0])
		if _, err := newIncidentsClient().Delete("/v1/api/source-follows/" + strconv.Itoa(service.WorkspaceID)); err != nil {
			exitErr(err)
		}
		printActionResult("unfollowed", "dependency", service.Slug)
	},
}

func init() {
	dependenciesFollowCmd.Flags().StringSlice("probe", nil, "Probe key to follow (repeatable); default: all probes")
	dependenciesCmd.AddCommand(dependenciesListCmd, dependenciesCatalogCmd, dependenciesGetCmd, dependenciesFollowCmd, dependenciesUnfollowCmd)
	rootCmd.AddCommand(dependenciesCmd)
}
