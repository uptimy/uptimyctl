package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/uptimy/uptimyctl/internal/client"
)

// pageChanges compares a file status page with the live one of that name.
func pageChanges(p StatusPage, live livePage, st *State) []string {
	var d []string
	if live.Public != p.Public {
		if p.Public {
			d = append(d, "publish")
		} else {
			d = append(d, "make private")
		}
	}
	if p.Groups == nil {
		return d
	}
	liveByLabel := map[string]liveGroup{}
	for _, g := range live.Groups {
		liveByLabel[strings.ToLower(g.Label)] = g
	}
	for i, g := range p.Groups {
		lg, ok := liveByLabel[strings.ToLower(g.Name)]
		if !ok {
			d = append(d, fmt.Sprintf("+group %q (%s)", g.Name, strings.Join(g.Monitors, ", ")))
			continue
		}
		names := make([]string, 0, len(lg.Entries))
		for _, e := range lg.Entries {
			names = append(names, st.subjectName(e.target()))
		}
		switch {
		case strings.Join(names, "\x00") != strings.Join(g.Monitors, "\x00"):
			d = append(d, fmt.Sprintf("group %q: %s → %s", g.Name, strings.Join(names, ", "), strings.Join(g.Monitors, ", ")))
		case lg.Hidden != g.Hidden || lg.Order != i:
			d = append(d, fmt.Sprintf("group %q: order or visibility", g.Name))
		}
	}
	for _, lg := range live.Groups {
		found := false
		for _, g := range p.Groups {
			if strings.EqualFold(g.Name, lg.Label) {
				found = true
			}
		}
		if !found {
			d = append(d, fmt.Sprintf("-group %q", lg.Label))
		}
	}
	return d
}

// checkPageRefs reports names a page shows that are neither in the file
// nor in the workspace.
func checkPageRefs(p StatusPage, f *File, st *State) error {
	var missing []string
	for _, g := range p.Groups {
		for _, name := range g.Monitors {
			if !fileHasSubject(f, name) {
				if _, _, ok := st.subject(name); !ok {
					missing = append(missing, name)
				}
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("shows %s, which no monitor or heartbeat is named", strings.Join(missing, ", "))
	}
	return nil
}

func fileHasSubject(f *File, name string) bool {
	for _, m := range f.Monitors {
		if strings.EqualFold(m.Name, name) {
			return true
		}
	}
	for _, h := range f.Heartbeats {
		if strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

func createPage(s Services, st *State, p StatusPage) error {
	// The app's defaults for a new status page.
	body := map[string]any{
		"name": p.Name, "public": p.Public, "whiteLabel": false, "theme": map[string]any{},
		"branding": map[string]any{"logoUrl": "", "logoDarkUrl": "", "websiteUrl": "", "faviconUrl": ""},
		"componentsVisibility": map[string]any{"showServices": true, "showIncidents": true, "showMaintenance": true,
			"showResponseTime": true, "showRegions": true, "hideEmptyIncidents": true, "hideEmptyMaintenance": true, "showSubscriptions": false},
		"enableLiveUpdates": false,
	}
	raw, err := s.API.Post("/v1/api/status-pages/", body)
	if err != nil {
		return err
	}
	data, err := client.ParseDataField(raw)
	if err != nil {
		return err
	}
	var created livePage
	if err := json.Unmarshal(data, &created); err != nil || created.UUID == "" {
		return errors.New("the API didn't return the new status page")
	}
	return syncGroups(s, st, p, created)
}

func updatePage(s Services, st *State, p StatusPage, live livePage) error {
	if live.Public != p.Public {
		// The update takes the whole page: send it back with public changed.
		var page map[string]any
		if err := get(s.API, "/v1/api/status-pages/"+url.PathEscape(live.UUID), &page); err != nil {
			return err
		}
		body := map[string]any{"public": p.Public}
		for _, k := range []string{"name", "slug", "whiteLabel", "theme", "branding", "componentsVisibility", "enableLiveUpdates", "language", "translations"} {
			if v, ok := page[k]; ok && v != nil {
				body[k] = v
			}
		}
		if _, err := s.API.Put("/v1/api/status-pages/"+url.PathEscape(live.UUID), body); err != nil {
			return err
		}
	}
	if p.Groups == nil {
		return nil
	}
	return syncGroups(s, st, p, live)
}

// syncGroups makes the page show exactly the file's groups. Rows that stay
// keep their own label and settings from the app.
func syncGroups(s Services, st *State, p StatusPage, live livePage) error {
	base := "/v1/api/status-pages/" + url.PathEscape(live.UUID) + "/groups/"
	liveByLabel := map[string]liveGroup{}
	for _, g := range live.Groups {
		liveByLabel[strings.ToLower(g.Label)] = g
	}
	for i, g := range p.Groups {
		lg, exists := liveByLabel[strings.ToLower(g.Name)]
		kept := map[string]liveEntry{}
		for _, e := range lg.Entries {
			kept[e.target()] = e
		}
		entries := make([]map[string]any, 0, len(g.Monitors))
		for j, name := range g.Monitors {
			kind, uuid, ok := st.subject(name)
			if !ok {
				return fmt.Errorf("group %q: no monitor or heartbeat named %q", g.Name, name)
			}
			e := map[string]any{"kind": kind, "order": j, "showDailies": true, "isVisible": true}
			if old, ok := kept[uuid]; ok {
				e["showDailies"], e["isVisible"] = old.ShowDailies, old.IsVisible
				if old.CustomLabel != nil {
					e["customLabel"] = *old.CustomLabel
				}
			}
			if kind == "service" {
				e["applicationUuid"] = uuid
			} else {
				e["heartbeatMonitorUuid"] = uuid
			}
			entries = append(entries, e)
		}
		body := map[string]any{"label": g.Name, "hidden": g.Hidden, "order": i, "entries": entries}
		var err error
		if exists {
			_, err = s.API.Put(base+url.PathEscape(lg.UUID), body)
		} else {
			_, err = s.API.Post(base, body)
		}
		if err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
	}
	for _, lg := range live.Groups {
		keep := false
		for _, g := range p.Groups {
			if strings.EqualFold(g.Name, lg.Label) {
				keep = true
			}
		}
		if !keep {
			if _, err := s.API.Delete(base + url.PathEscape(lg.UUID)); err != nil {
				return fmt.Errorf("group %q: %w", lg.Label, err)
			}
		}
	}
	return nil
}

func exportPage(p livePage, st *State) StatusPage {
	out := StatusPage{Name: p.Name, Public: p.Public, Groups: []PageGroup{}}
	for _, g := range p.Groups {
		pg := PageGroup{Name: g.Label, Hidden: g.Hidden, Monitors: []string{}}
		for _, e := range g.Entries {
			if name := st.subjectName(e.target()); name != "" {
				pg.Monitors = append(pg.Monitors, name)
			}
		}
		out.Groups = append(out.Groups, pg)
	}
	return out
}
