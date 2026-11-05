package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/uptimy/uptimyctl/internal/client"
)

// Change is one thing apply would do.
type Change struct {
	Action  string   `json:"action"` // add, change, remove
	Kind    string   `json:"kind"`   // tag, monitor, heartbeat, status page, alert
	Name    string   `json:"name"`
	Details []string `json:"details,omitempty"`

	// Apply runs steps in order: tags; monitors and heartbeats; then, read
	// again so they can be named, status pages and alerts; removals last.
	step int
	run  func(Services, *State) error
}

func (c Change) String() string {
	mark := map[string]string{"add": "+", "change": "~", "remove": "-"}[c.Action]
	s := fmt.Sprintf("%s %s %q", mark, c.Kind, c.Name)
	if len(c.Details) > 0 {
		s += ": " + strings.Join(c.Details, "; ")
	}
	return s
}

type Plan struct {
	Workspace string   `json:"workspace"`
	Changes   []Change `json:"changes"`
	// In the workspace but not in the file: kept unless --prune.
	NotInFile []string `json:"notInFile"`
	// Workflows made or edited in the workflow editor: never touched.
	EditorWorkflows []string `json:"editorWorkflows"`
}

type Options struct {
	// Prune removes what the file doesn't have.
	Prune bool
}

// MakePlan compares the file with the live workspace.
func MakePlan(f *File, st *State, opt Options) (*Plan, error) {
	p := &Plan{Workspace: st.Workspace, Changes: []Change{}, NotInFile: []string{}, EditorWorkflows: []string{}}
	var errs []string
	fail := func(kind, name string, err error) { errs = append(errs, fmt.Sprintf("%s %q: %v", kind, name, err)) }
	keep := func(kind, name string) { p.NotInFile = append(p.NotInFile, kind+" "+name) }
	remove := func(kind, name string, run func(Services, *State) error) {
		p.Changes = append(p.Changes, Change{Action: "remove", Kind: kind, Name: name, step: 3, run: run})
	}

	// Tags the file uses that don't exist yet.
	seen := map[string]bool{}
	var tags []string
	for _, m := range f.Monitors {
		tags = append(tags, m.Tags...)
	}
	for _, h := range f.Heartbeats {
		tags = append(tags, h.Tags...)
	}
	for _, a := range f.Alerts {
		tags = append(tags, a.Tags...)
	}
	for _, name := range tags {
		if _, ok := st.tagID(name); ok || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		p.Changes = append(p.Changes, Change{Action: "add", Kind: "tag", Name: name, step: 0, run: func(s Services, st *State) error {
			raw, err := s.API.Post("/v1/api/tags/", map[string]any{"name": name})
			if err != nil {
				return err
			}
			data, err := client.ParseDataField(raw)
			if err != nil {
				return err
			}
			var tag struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(data, &tag); err != nil {
				return err
			}
			st.Tags[tag.ID] = name
			return nil
		}})
	}

	// Monitors.
	inFile := map[string]bool{}
	for _, m := range f.Monitors {
		inFile[strings.ToLower(m.Name)] = true
		w, err := resolveMonitor(m, st)
		if err != nil {
			fail("monitor", m.Name, err)
			continue
		}
		live := findMonitor(st, m.Name)
		if live == nil {
			p.Changes = append(p.Changes, Change{Action: "add", Kind: "monitor", Name: m.Name, Details: []string{w.describe()}, step: 1,
				run: func(s Services, st *State) error { return createMonitor(s, st, w) }})
			continue
		}
		if d := monitorChanges(w, live, st); len(d) > 0 {
			l := *live
			p.Changes = append(p.Changes, Change{Action: "change", Kind: "monitor", Name: m.Name, Details: d, step: 1,
				run: func(s Services, st *State) error { return updateMonitor(s, st, w, l) }})
		}
	}
	for _, m := range st.Monitors {
		if inFile[strings.ToLower(m.name())] {
			continue
		}
		if !opt.Prune {
			keep("monitor", m.name())
			continue
		}
		app := m.appUUID()
		remove("monitor", m.name(), func(s Services, _ *State) error {
			_, err := s.API.Delete("/v1/api/applications/" + url.PathEscape(app))
			return err
		})
	}

	// Heartbeats.
	inFile = map[string]bool{}
	for _, h := range f.Heartbeats {
		inFile[strings.ToLower(h.Name)] = true
		w := resolveHeartbeat(h)
		live := findHeartbeat(st, h.Name)
		if live == nil {
			p.Changes = append(p.Changes, Change{Action: "add", Kind: "heartbeat", Name: h.Name, Details: []string{w.schedule()}, step: 1,
				run: func(s Services, st *State) error { return createHeartbeat(s, st, w) }})
			continue
		}
		if d := heartbeatChanges(w, *live, st); len(d) > 0 {
			l := *live
			p.Changes = append(p.Changes, Change{Action: "change", Kind: "heartbeat", Name: h.Name, Details: d, step: 1,
				run: func(s Services, st *State) error { return updateHeartbeat(s, st, w, l) }})
		}
	}
	for _, h := range st.Heartbeats {
		if inFile[strings.ToLower(h.Name)] {
			continue
		}
		if !opt.Prune {
			keep("heartbeat", h.Name)
			continue
		}
		uuid := h.UUID
		remove("heartbeat", h.Name, func(s Services, _ *State) error {
			_, err := s.Heartbeats.Delete("/v1/api/heartbeat-monitors/" + url.PathEscape(uuid))
			return err
		})
	}

	// Status pages.
	inFile = map[string]bool{}
	for _, page := range f.StatusPages {
		inFile[strings.ToLower(page.Name)] = true
		if err := checkPageRefs(page, f, st); err != nil {
			fail("status page", page.Name, err)
			continue
		}
		live := findPage(st, page.Name)
		if live == nil {
			d := []string{"private"}
			if page.Public {
				d = []string{"public"}
			}
			for _, g := range page.Groups {
				d = append(d, fmt.Sprintf("group %q (%s)", g.Name, strings.Join(g.Monitors, ", ")))
			}
			p.Changes = append(p.Changes, Change{Action: "add", Kind: "status page", Name: page.Name, Details: d, step: 2,
				run: func(s Services, st *State) error { return createPage(s, st, page) }})
			continue
		}
		if d := pageChanges(page, *live, st); len(d) > 0 {
			l := *live
			p.Changes = append(p.Changes, Change{Action: "change", Kind: "status page", Name: page.Name, Details: d, step: 2,
				run: func(s Services, st *State) error { return updatePage(s, st, page, l) }})
		}
	}
	for _, page := range st.StatusPages {
		if inFile[strings.ToLower(page.Name)] {
			continue
		}
		if !opt.Prune {
			keep("status page", page.Name)
			continue
		}
		uuid := page.UUID
		remove("status page", page.Name, func(s Services, _ *State) error {
			_, err := s.API.Delete("/v1/api/status-pages/" + url.PathEscape(uuid))
			return err
		})
	}

	// Alerts.
	inFile = map[string]bool{}
	for _, a := range f.Alerts {
		inFile[strings.ToLower(a.Name)] = true
		if a.To.Integration != "" {
			if _, err := pickIntegration(st, a.To.Integration); err != nil {
				fail("alert", a.Name, err)
				continue
			}
		}
		var missing []string
		for _, name := range a.Monitors {
			if _, _, ok := st.subject(name); !ok && !fileHasSubject(f, name) {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			fail("alert", a.Name, fmt.Errorf("no monitor or heartbeat named %s", strings.Join(missing, ", ")))
			continue
		}
		w := findWorkflow(st, a.Name)
		switch {
		case w == nil:
			d := []string{"to " + describeTo(a.To)}
			if len(a.Tags) > 0 {
				d = append(d, "tags "+strings.Join(a.Tags, ", "))
			}
			p.Changes = append(p.Changes, Change{Action: "add", Kind: "alert", Name: a.Name, Details: d, step: 2,
				run: func(s Services, st *State) error { return createAlert(s, st, a) }})
		case w.Alert == nil:
			fail("alert", a.Name, errors.New("a workflow with this name was made in the workflow editor; rename the alert, or change the workflow there"))
		default:
			if d := alertChanges(a, *w, st); len(d) > 0 {
				l := *w
				p.Changes = append(p.Changes, Change{Action: "change", Kind: "alert", Name: a.Name, Details: d, step: 2,
					run: func(s Services, st *State) error { return updateAlert(s, st, a, l) }})
			}
		}
	}
	for _, w := range st.Workflows {
		if w.Alert == nil {
			if !inFile[strings.ToLower(w.Name)] {
				p.EditorWorkflows = append(p.EditorWorkflows, w.Name)
			}
			continue
		}
		if inFile[strings.ToLower(w.Name)] {
			continue
		}
		if !opt.Prune {
			keep("alert", w.Name)
			continue
		}
		uuid := w.UUID
		remove("alert", w.Name, func(s Services, _ *State) error {
			_, err := s.Workflows.Delete("/v1/api/alerts/" + url.PathEscape(uuid))
			return err
		})
	}

	if len(errs) > 0 {
		return p, errors.New(strings.Join(errs, "\n"))
	}
	// Removals in reverse dependency order: alerts and pages before what they show.
	order := map[string]int{"alert": 0, "status page": 1, "heartbeat": 2, "monitor": 3}
	sort.SliceStable(p.Changes, func(i, j int) bool {
		a, b := p.Changes[i], p.Changes[j]
		if a.step != b.step {
			return a.step < b.step
		}
		if a.step == 3 {
			return order[a.Kind] < order[b.Kind]
		}
		return false
	})
	return p, nil
}

func describeTo(t AlertTo) string {
	switch {
	case t.Integration != "":
		return t.Integration
	case len(t.Emails) > 0:
		return strings.Join(t.Emails, ", ")
	default:
		return t.Webhook
	}
}

func findMonitor(st *State, name string) *liveMonitor {
	for i := range st.Monitors {
		if strings.EqualFold(st.Monitors[i].name(), name) {
			return &st.Monitors[i]
		}
	}
	return nil
}

func findHeartbeat(st *State, name string) *liveHeartbeat {
	for i := range st.Heartbeats {
		if strings.EqualFold(st.Heartbeats[i].Name, name) {
			return &st.Heartbeats[i]
		}
	}
	return nil
}

func findPage(st *State, name string) *livePage {
	for i := range st.StatusPages {
		if strings.EqualFold(st.StatusPages[i].Name, name) {
			return &st.StatusPages[i]
		}
	}
	return nil
}

func findWorkflow(st *State, name string) *liveWorkflow {
	for i := range st.Workflows {
		if strings.EqualFold(st.Workflows[i].Name, name) {
			return &st.Workflows[i]
		}
	}
	return nil
}

// Summary counts a plan's changes: "2 to add, 1 to change, 0 to remove".
func (p *Plan) Summary() string {
	n := map[string]int{}
	for _, c := range p.Changes {
		n[c.Action]++
	}
	return fmt.Sprintf("%d to add, %d to change, %d to remove", n["add"], n["change"], n["remove"])
}

// Apply makes the plan's changes, reading the workspace again before the
// steps that name what earlier steps made. done is called after each change.
// It stops at the first error: applying again carries on from there.
func Apply(s Services, f *File, st *State, p *Plan, opt Options, done func(Change)) error {
	run := func(changes []Change, st *State, steps ...int) error {
		for _, c := range changes {
			for _, step := range steps {
				if c.step == step {
					if err := c.run(s, st); err != nil {
						return fmt.Errorf("%s: %w", c.String(), err)
					}
					done(c)
				}
			}
		}
		return nil
	}
	if err := run(p.Changes, st, 0, 1); err != nil {
		return err
	}
	later := false
	for _, c := range p.Changes {
		if c.step >= 2 {
			later = true
		}
	}
	if !later {
		return nil
	}
	fresh, err := Fetch(s)
	if err != nil {
		return err
	}
	again, err := MakePlan(f, fresh, opt)
	if err != nil {
		return err
	}
	return run(again.Changes, fresh, 2, 3)
}

// Export writes the live workspace as a file. Defaults are left out, so the
// file reads like one written by hand.
func Export(st *State) *File {
	f := &File{Version: Version, Monitors: []Monitor{}, Heartbeats: []Heartbeat{}, StatusPages: []StatusPage{}, Alerts: []Alert{}}
	all := st.allRegionNames()
	for _, live := range st.Monitors {
		t := typeName(live.TypeID)
		m := shorthand(t, live.Config)
		m.Name = live.name()
		if m.Type == "http" {
			m.Type = ""
		}
		if live.IntervalSeconds != defaultInterval(t, st.IntervalFloor) {
			m.Interval = live.IntervalSeconds
		}
		if live.TimeoutSeconds != 30 {
			m.Timeout = live.TimeoutSeconds
		}
		regions := make([]string, 0, len(live.Schedulers))
		for _, s := range live.Schedulers {
			regions = append(regions, s.Name)
		}
		sort.Strings(regions)
		if strings.Join(regions, "\x00") != strings.Join(all, "\x00") {
			m.Regions = regions
		}
		for _, tag := range live.Tags {
			m.Tags = append(m.Tags, tag.Name)
		}
		sort.Strings(m.Tags)
		m.Paused = !live.Active
		f.Monitors = append(f.Monitors, m)
	}
	for _, h := range st.Heartbeats {
		f.Heartbeats = append(f.Heartbeats, exportHeartbeat(h, st))
	}
	for _, p := range st.StatusPages {
		f.StatusPages = append(f.StatusPages, exportPage(p, st))
	}
	for _, w := range st.Workflows {
		if w.Alert != nil {
			f.Alerts = append(f.Alerts, exportAlert(w, st))
		}
	}
	return f
}
