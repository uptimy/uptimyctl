package workspace

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fake is one service: canned responses by "METHOD path", and every write.
type fake struct {
	mu        sync.Mutex
	responses map[string]string
	writes    []string
	bodies    map[string]any
	onWrite   func(f *fake, key string)
}

func (f *fake) respond(key string) ([]byte, error) {
	data, ok := f.responses[key]
	if !ok {
		return nil, fmt.Errorf("fake: no response for %s", key)
	}
	return []byte(`{"data":` + data + `}`), nil
}

func (f *fake) write(method, path string, body any) ([]byte, error) {
	f.mu.Lock()
	key := method + " " + path
	f.writes = append(f.writes, key)
	if f.bodies == nil {
		f.bodies = map[string]any{}
	}
	f.bodies[key] = normalized(body)
	f.mu.Unlock()
	if f.onWrite != nil {
		f.onWrite(f, key)
	}
	if _, ok := f.responses[key]; !ok {
		return []byte(`{"data":{}}`), nil
	}
	return f.respond(key)
}

func (f *fake) Get(path string, _ url.Values) ([]byte, error) { return f.respond("GET " + path) }
func (f *fake) Post(path string, body interface{}) ([]byte, error) {
	return f.write("POST", path, body)
}
func (f *fake) Put(path string, body interface{}) ([]byte, error) { return f.write("PUT", path, body) }
func (f *fake) Patch(path string, body interface{}) ([]byte, error) {
	return f.write("PATCH", path, body)
}
func (f *fake) Delete(path string) ([]byte, error) { return f.write("DELETE", path, nil) }

const httpConfig = `{"method":"GET","uri":"https://acme.test","expectedStatusCode":200,"body":{},"headers":{},"expectedResponseBody":{},"proxyUrl":""}`

func services() (Services, *fake, *fake, *fake) {
	api := &fake{responses: map[string]string{
		"GET /v1/api/api-key":     `{"workspace":{"name":"Acme"}}`,
		"GET /v1/api/quota":       `{"healthcheckFrequency":30}`,
		"GET /v1/api/schedulers/": `{"results":[{"uuid":"s-fra","name":"🇩🇪 Frankfurt","premium":false},{"uuid":"s-sin","name":"🇸🇬 Singapore","premium":false}]}`,
		"GET /v1/api/tags/":       `{"results":[{"id":7,"name":"prod"}]}`,
		"GET /v1/api/healthchecks/": `{"results":[
			{"uuid":"hc-web","applicationUuid":"app-web","healthcheckTypeId":1,"config":` + httpConfig + `,"intervalSeconds":60,"timeoutSeconds":30,"active":true,"managedByLocationUuid":null,
			 "application":{"uuid":"app-web","name":"Website"},"schedulers":[{"uuid":"s-fra","name":"🇩🇪 Frankfurt"},{"uuid":"s-sin","name":"🇸🇬 Singapore"}],"tags":[{"id":7,"name":"prod"}]},
			{"uuid":"hc-ssl","applicationUuid":"app-ssl","healthcheckTypeId":6,"config":{"host":"acme.test","port":443,"expiryWarningDays":30},"intervalSeconds":3600,"timeoutSeconds":60,"active":true,
			 "application":{"uuid":"app-ssl","name":"acme.test SSL"},"schedulers":[{"uuid":"s-fra","name":"🇩🇪 Frankfurt"},{"uuid":"s-bom","name":"🇮🇳 Mumbai"}],"tags":[]},
			{"uuid":"hc-api","applicationUuid":"app-api","healthcheckTypeId":1,"config":{"method":"GET","uri":"https://api.acme.test","expectedStatusCode":401,"body":{},"headers":{"Authorization":"Bearer x"},"expectedResponseBody":{},"proxyUrl":""},
			 "intervalSeconds":60,"timeoutSeconds":30,"active":false,"application":{"uuid":"app-api","name":"API"},"schedulers":[{"uuid":"s-fra","name":"🇩🇪 Frankfurt"},{"uuid":"s-sin","name":"🇸🇬 Singapore"}],"tags":[]},
			{"uuid":"hc-agent","applicationUuid":"app-agent","healthcheckTypeId":1,"config":{"uri":"http://db:5432"},"intervalSeconds":60,"timeoutSeconds":10,"active":true,"managedByLocationUuid":"loc-1",
			 "application":{"uuid":"app-agent","name":"Postgres"},"schedulers":[],"tags":[]}]}`,
		"GET /v1/api/status-pages/":     `{"results":[{"uuid":"sp-1","name":"Acme Status","public":true}]}`,
		"GET /v1/api/status-pages/sp-1": `{"uuid":"sp-1","name":"Acme Status","slug":"acme","public":true,"whiteLabel":false,"theme":{},"branding":{"logoUrl":""},"componentsVisibility":{"showServices":true},"enableLiveUpdates":false}`,
		"GET /v1/api/status-pages/sp-1/groups/": `{"results":[{"uuid":"g-1","label":"Services","hidden":false,"order":0,"entries":[
			{"kind":"service","applicationUuid":"app-web","heartbeatMonitorUuid":null,"order":0,"showDailies":true,"isVisible":true,"customLabel":"Website"},
			{"kind":"heartbeat","applicationUuid":null,"heartbeatMonitorUuid":"hb-backup","order":1,"showDailies":true,"isVisible":true,"customLabel":null},
			{"kind":"service","applicationUuid":"app-agent","heartbeatMonitorUuid":null,"order":2,"showDailies":false,"isVisible":true,"customLabel":null}]}]}`,
		"POST /v1/api/tags/":             `{"id":8,"name":"staging"}`,
		"POST /v1/api/healthchecks/bulk": `{"results":[{"uuid":"app-docs","name":"Docs"}]}`,
	}}
	heartbeats := &fake{responses: map[string]string{
		"GET /v1/api/heartbeat-monitors/": `{"results":[
			{"uuid":"hb-backup","name":"Nightly backup","intervalSeconds":0,"cron":"0 2 * * *","timezone":"Europe/Sarajevo","noSchedule":false,"maxRunSeconds":0,"graceSeconds":600,"failureThreshold":1,"recoveryThreshold":1,"enforceOrder":true,"steps":[],"tagIds":[7],"source":"cloud"},
			{"uuid":"hb-worker","name":"Worker","intervalSeconds":300,"noSchedule":false,"maxRunSeconds":0,"graceSeconds":300,"failureThreshold":1,"recoveryThreshold":1,"enforceOrder":true,"pausedAt":"2026-11-01T00:00:00Z",
			 "steps":[{"uuid":"st-1","key":"start","name":"Start","position":0,"required":true}],"tagIds":[],"source":"cloud"},
			{"uuid":"hb-manual","name":"Manual run","intervalSeconds":0,"noSchedule":true,"maxRunSeconds":0,"graceSeconds":0,"failureThreshold":1,"recoveryThreshold":1,"enforceOrder":true,"steps":[],"tagIds":[],"source":"cloud"},
			{"uuid":"hb-liveness","name":"Uptimy Agent · abc","intervalSeconds":60,"graceSeconds":120,"failureThreshold":1,"recoveryThreshold":1,"enforceOrder":true,"tagIds":[],"source":"agent_liveness"}]}`,
	}}
	workflows := &fake{responses: map[string]string{
		"GET /v1/api/workflows/": `{"results":[
			{"uuid":"wf-default","name":"Default email alerts","enabled":true,"alert":null},
			{"uuid":"wf-ops","name":"Prod to #ops","enabled":true,"alert":{"destination":{"type":"integration","integrationUuid":"int-1","provider":"slack"},"events":["opened","resolved"],"tagIds":[7],"severities":[],"sourceUuids":["app-web"]}},
			{"uuid":"wf-old","name":"Old API alert","enabled":false,"alert":{"destination":{"type":"webhook","url":"https://hooks.acme.test/old"},"events":["opened","resolved"],"tagIds":[],"severities":[],"sourceUuids":["0a1b2c3d-0000-4000-8000-000000000001"]}}]}`,
		"GET /v1/api/integrations/": `{"results":[{"uuid":"int-1","provider":"slack","name":"Acme","config":{"channelName":"ops"}},{"uuid":"int-2","provider":"github","name":"acme/app","config":{}}]}`,
	}}
	reports := &fake{responses: map[string]string{
		"GET /v1/api/reports/definitions": `{"results":[
			{"uuid":"rd-all","name":"All monitors","isDefault":true,"scopeType":"workspace","monitorUuids":null,"tagIds":null,"slaTarget":99.9,"latencyThresholdMs":null,
			 "cadence":"weekly","sendDay":1,"sendHour":9,"sendMinute":0,"sendTimezone":"Europe/Sarajevo","emailEnabled":true,"brandName":null,"brandLogoUrl":null,"brandColor":null,"recipients":["ops@acme.test"]},
			{"uuid":"rd-globex","name":"Globex","isDefault":false,"scopeType":"monitors","monitorUuids":["app-web","hb-backup"],"tagIds":null,"slaTarget":99.5,"latencyThresholdMs":800,
			 "cadence":"monthly","sendDay":2,"sendHour":8,"sendMinute":30,"sendTimezone":"Europe/Sarajevo","emailEnabled":true,"brandName":"Globex","brandLogoUrl":"https://cdn.acme.test/globex.png","brandColor":"#0055ff","recipients":["cto@globex.test"]},
			{"uuid":"rd-prod","name":"Prod","isDefault":false,"scopeType":"tags","monitorUuids":null,"tagIds":[7],"slaTarget":99.9,"latencyThresholdMs":null,
			 "cadence":"none","sendDay":1,"sendHour":9,"sendMinute":0,"sendTimezone":"UTC","emailEnabled":true,"brandName":null,"brandLogoUrl":null,"brandColor":null,"recipients":[]}]}`,
	}}
	return Services{API: api, Workflows: workflows, Heartbeats: heartbeats, Reports: reports}, api, heartbeats, workflows
}

func reportNamed(f *File, name string) *Report {
	for i := range f.Reports {
		if f.Reports[i].Name == name {
			return &f.Reports[i]
		}
	}
	panic("no report " + name)
}

func roundTrip(t *testing.T) (*File, *State, Services, *fake, *fake, *fake) {
	t.Helper()
	s, api, hb, wf := services()
	st, err := Fetch(s)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Export(st).Marshal("Uptimy workspace Acme")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("exported file doesn't parse: %v\n%s", err, data)
	}
	return f, st, s, api, hb, wf
}

func monitorNamed(f *File, name string) *Monitor {
	for i := range f.Monitors {
		if f.Monitors[i].Name == name {
			return &f.Monitors[i]
		}
	}
	panic("no monitor " + name)
}

func heartbeatNamed(f *File, name string) *Heartbeat {
	for i := range f.Heartbeats {
		if f.Heartbeats[i].Name == name {
			return &f.Heartbeats[i]
		}
	}
	panic("no heartbeat " + name)
}

func TestExportThenPlanChangesNothing(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	p, err := MakePlan(f, st, Options{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Changes {
		t.Errorf("unexpected change: %s", c)
	}
	if len(p.EditorWorkflows) != 1 || p.EditorWorkflows[0] != "Default email alerts" {
		t.Errorf("editor workflows = %v", p.EditorWorkflows)
	}
}

func TestExportReadsLikeAHandWrittenFile(t *testing.T) {
	_, st, _, _, _, _ := roundTrip(t)
	f := Export(st)
	if len(f.Monitors) != 3 {
		t.Fatalf("monitors = %d, want 3 (the agent's isn't the file's)", len(f.Monitors))
	}
	web, ssl, api := *monitorNamed(f, "Website"), *monitorNamed(f, "acme.test SSL"), *monitorNamed(f, "API")
	if web.Name != "Website" || web.Target != "https://acme.test" || web.Config != nil || web.Interval != 0 || web.Regions != nil || web.Type != "" {
		t.Errorf("website = %+v", web)
	}
	// Mumbai isn't a region the plan offers any more, but this monitor still uses it.
	if ssl.Type != "ssl" || ssl.Target != "acme.test" || ssl.Port != 0 || ssl.Timeout != 60 || strings.Join(ssl.Regions, ",") != "🇩🇪 Frankfurt,🇮🇳 Mumbai" {
		t.Errorf("ssl = %+v", ssl)
	}
	// Headers can't be a target: the config is written as it is.
	if api.Target != "" || api.Config["headers"] == nil || !api.Paused {
		t.Errorf("api = %+v", api)
	}
	if b := heartbeatNamed(f, "Nightly backup"); len(f.Heartbeats) != 3 || b.Cron != "0 2 * * *" || b.Grace != 600 || b.Timezone != "Europe/Sarajevo" {
		t.Errorf("heartbeats = %+v", f.Heartbeats)
	}
	// No schedule, so no grace: nothing to write.
	if h := heartbeatNamed(f, "Manual run"); !h.NoSchedule || h.Grace != 0 {
		t.Errorf("no-schedule heartbeat = %+v", h)
	}
	if steps := heartbeatNamed(f, "Worker").Steps; len(steps) != 1 || steps[0]["uuid"] != nil {
		t.Errorf("steps keep no ids: %v", steps)
	}
	if g := f.StatusPages[0].Groups[0]; strings.Join(g.Monitors, ",") != "Website,Nightly backup,Postgres" {
		t.Errorf("page group = %+v", g)
	}
	if a := f.Alerts[0]; a.To.Integration != "Slack #ops (Acme)" || a.Events != nil || a.Monitors[0] != "Website" {
		t.Errorf("alert = %+v", a)
	}
	// Its only monitor was deleted: the UUID stays, or the alert would match everything.
	if a := f.Alerts[1]; len(a.Monitors) != 1 || a.Monitors[0] != "0a1b2c3d-0000-4000-8000-000000000001" {
		t.Errorf("alert scoped to a deleted monitor = %+v", a)
	}
}

func TestPlanShowsEditsAndKeepsWhatsMissing(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	monitorNamed(f, "Website").Interval = 120
	f.Monitors = append(f.Monitors, Monitor{Name: "Docs", Target: "docs.acme.test", Tags: []string{"staging"}})
	f.Heartbeats = f.Heartbeats[:2] // Worker goes
	f.StatusPages[0].Groups[0].Monitors = append(f.StatusPages[0].Groups[0].Monitors, "Docs")
	f.Alerts[0].Severities = []string{"critical"}

	p, err := MakePlan(f, st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range p.Changes {
		got = append(got, c.String())
	}
	want := []string{
		`+ tag "staging"`,
		`~ monitor "Website": interval 60s → 120s`,
		`+ monitor "Docs": http docs.acme.test, every 60s from 2 regions`,
		`~ status page "Acme Status": group "Services": Website, Nightly backup, Postgres → Website, Nightly backup, Postgres, Docs`,
		`~ alert "Prod to #ops": severities +critical`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("changes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(p.NotInFile, ",") != "heartbeat Worker" {
		t.Errorf("notInFile = %v", p.NotInFile)
	}

	pruned, err := MakePlan(f, st, Options{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if last := pruned.Changes[len(pruned.Changes)-1]; last.String() != `- heartbeat "Worker"` {
		t.Errorf("last change = %s", last)
	}
}

func TestApplyMakesTheChangesInOrder(t *testing.T) {
	f, st, s, api, hb, _ := roundTrip(t)
	monitorNamed(f, "Website").Interval = 120
	f.Monitors = append(f.Monitors, Monitor{Name: "Docs", Target: "docs.acme.test", Tags: []string{"staging"}})
	f.StatusPages[0].Groups[0].Monitors = append(f.StatusPages[0].Groups[0].Monitors, "Docs")
	heartbeatNamed(f, "Nightly backup").Grace = 900
	// After the bulk create, the workspace lists Docs too.
	api.onWrite = func(f *fake, key string) {
		if key != "POST /v1/api/healthchecks/bulk" {
			return
		}
		var list results[map[string]any]
		_ = json.Unmarshal([]byte(f.responses["GET /v1/api/healthchecks/"]), &list)
		list.Results = append(list.Results, map[string]any{"uuid": "hc-docs", "applicationUuid": "app-docs", "healthcheckTypeId": 1,
			"config": map[string]any{"uri": "https://docs.acme.test"}, "intervalSeconds": 60, "timeoutSeconds": 30, "active": true,
			"application": map[string]any{"uuid": "app-docs", "name": "Docs"}})
		b, _ := json.Marshal(list)
		f.responses["GET /v1/api/healthchecks/"] = string(b)
	}

	p, err := MakePlan(f, st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var done []string
	if err := Apply(s, f, st, p, Options{}, func(c Change) { done = append(done, c.Kind) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(done, ",") != "tag,monitor,monitor,heartbeat,status page" {
		t.Errorf("done = %v", done)
	}
	if strings.Join(api.writes, "\n") != strings.Join([]string{
		"POST /v1/api/tags/",
		"PUT /v1/api/healthchecks/hc-web",
		"POST /v1/api/healthchecks/bulk",
		"PUT /v1/api/applications/app-docs/healthchecks/tags/",
		"PUT /v1/api/status-pages/sp-1/groups/g-1",
	}, "\n") {
		t.Errorf("api writes:\n%s", strings.Join(api.writes, "\n"))
	}
	// The page keeps its rows' own settings and gets the new monitor at the end.
	entries := api.bodies["PUT /v1/api/status-pages/sp-1/groups/g-1"].(map[string]any)["entries"].([]any)
	if len(entries) != 4 || entries[0].(map[string]any)["customLabel"] != "Website" || entries[2].(map[string]any)["showDailies"] != false ||
		entries[3].(map[string]any)["applicationUuid"] != "app-docs" {
		t.Errorf("entries = %v", entries)
	}
	if tags := api.bodies["PUT /v1/api/applications/app-docs/healthchecks/tags/"].(map[string]any)["tagIds"].([]any); tags[0] != float64(8) {
		t.Errorf("docs tags = %v", tags)
	}
	// A heartbeat update sends everything, schedule included.
	b := hb.bodies["PUT /v1/api/heartbeat-monitors/hb-backup"].(map[string]any)
	if b["graceSeconds"] != float64(900) || b["cron"] != "0 2 * * *" || b["timezone"] != "Europe/Sarajevo" || b["tagIds"].([]any)[0] != float64(7) {
		t.Errorf("heartbeat body = %v", b)
	}
}

func TestPlanRefusesWhatItCantDo(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	monitorNamed(f, "Website").Regions = []string{"Mars"}
	// A region only another monitor still uses can't be given to a new one.
	f.Monitors = append(f.Monitors, Monitor{Name: "New", Target: "new.acme.test", Regions: []string{"Mumbai"}})
	monitorNamed(f, "API").Interval = 10
	f.Alerts = append(f.Alerts, Alert{Name: "default EMAIL alerts", To: AlertTo{Emails: []string{"a@acme.test"}}})
	f.Alerts = append(f.Alerts, Alert{Name: "Teams", To: AlertTo{Integration: "teams"}})
	f.StatusPages[0].Groups[0].Monitors = append(f.StatusPages[0].Groups[0].Monitors, "Nope")

	_, err := MakePlan(f, st, Options{})
	if err == nil {
		t.Fatal("want errors")
	}
	for _, want := range []string{`no region "Mars"`, `monitor "New": no region "Mumbai"`, "below the plan's minimum of 30s", "made in the workflow editor", "no integration \"teams\" is connected", "shows Nope"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q missing from:\n%v", want, err)
		}
	}
}

func TestParseChecksTheFile(t *testing.T) {
	_, err := Parse([]byte(`version: 2
monitors:
  - name: Web
    target: acme.test
  - name: web
    target: other.test
  - name: Port
    type: tcp
    target: db.acme.test
heartbeats:
  - name: Job
    every: 60
    cron: "* * * * *"
alerts:
  - name: Ops
    to: { emails: [a@acme.test], webhook: https://hooks.acme.test }
`))
	if err == nil {
		t.Fatal("want errors")
	}
	for _, want := range []string{`two monitors are named "web"`, `monitor "Port": a tcp monitor needs a port`, "exactly one of every, cron or noSchedule", "exactly one of to.integration"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q missing from:\n%v", want, err)
		}
	}
	if _, err := Parse([]byte("version: 2\nmonitors:\n  - name: x\n    targt: acme.test\n")); err == nil || !strings.Contains(err.Error(), "targt") {
		t.Errorf("a misspelt field must fail: %v", err)
	}
	if _, err := Parse([]byte("version: 1\n")); err == nil {
		t.Error("version 1 must fail")
	}
}

func TestAlertKeepsADeletedMonitorsScope(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	old := f.Alerts[1]
	old.Events = []string{"opened"} // any change makes apply send the whole alert
	b, err := alertBody(old, st)
	if err != nil {
		t.Fatal(err)
	}
	if sources := b["sourceUuids"].([]string); len(sources) != 1 || sources[0] != "0a1b2c3d-0000-4000-8000-000000000001" {
		t.Errorf("sourceUuids = %v: dropping them widens the alert", sources)
	}
	old.Monitors = []string{"Nope"}
	if _, err := alertBody(old, st); err == nil {
		t.Error("a name that isn't a monitor must still fail")
	}
}

func TestReportsExportLikeAHandWrittenFile(t *testing.T) {
	f, _, _, _, _, _ := roundTrip(t)
	if len(f.Reports) != 3 {
		t.Fatalf("reports = %d, want 3", len(f.Reports))
	}
	// Emailed, so monthly is the default: weekly is written. The timezone is the default's.
	if all := *reportNamed(f, "All monitors"); all.Schedule != "weekly" || all.Weekday != "" || all.Timezone != "" || all.SLA != 0 || all.Monitors != nil {
		t.Errorf("default = %+v", all)
	}
	g := *reportNamed(f, "Globex")
	if strings.Join(g.Monitors, ",") != "Website,Nightly backup" || g.SLA != 99.5 || g.SlowAboveMs != 800 || g.Schedule != "" || g.DayOfMonth != 2 || g.At != "08:30" {
		t.Errorf("globex = %+v", g)
	}
	if g.Brand == nil || g.Brand.Name != "Globex" || g.Brand.Color != "#0055ff" {
		t.Errorf("brand = %+v", g.Brand)
	}
	// Nobody emailed: read on demand is the default.
	if p := *reportNamed(f, "Prod"); strings.Join(p.Tags, ",") != "prod" || p.Schedule != "" || p.Timezone != "UTC" || p.Emails != nil {
		t.Errorf("prod = %+v", p)
	}
}

func TestReportsPlanAndApply(t *testing.T) {
	f, st, s, _, _, _ := roundTrip(t)
	g := reportNamed(f, "Globex")
	g.SLA = 99.95
	g.Emails = append(g.Emails, "ceo@globex.test")
	f.Reports = append(f.Reports, Report{Name: "Initech", Tags: []string{"initech"}, Emails: []string{"it@initech.test"}})
	// Neither the default nor Prod is in the file.
	f.Reports = []Report{f.Reports[1], f.Reports[3]}

	p, err := MakePlan(f, st, Options{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range p.Changes {
		got = append(got, c.String())
	}
	want := []string{
		`+ tag "initech"`,
		`~ report "Globex": SLA 99.5% → 99.95%; emails +ceo@globex.test`,
		`+ report "Initech": tags initech, SLA 99.9%, monthly on day 1 at 09:00 (Europe/Sarajevo)`,
		`- report "Prod"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("changes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(p.NotInFile, ",") != "report All monitors" {
		t.Errorf("the default report is kept: notInFile = %v", p.NotInFile)
	}

	// After the tag is created, the workspace lists it.
	s.API.(*fake).onWrite = func(f *fake, key string) {
		if key == "POST /v1/api/tags/" {
			f.responses["GET /v1/api/tags/"] = `{"results":[{"id":7,"name":"prod"},{"id":8,"name":"initech"}]}`
		}
	}
	if err := Apply(s, f, st, p, Options{Prune: true}, func(Change) {}); err != nil {
		t.Fatal(err)
	}
	rep := s.Reports.(*fake)
	if strings.Join(rep.writes, ",") != "PUT /v1/api/reports/definitions/rd-globex,POST /v1/api/reports/definitions,DELETE /v1/api/reports/definitions/rd-prod" {
		t.Errorf("report writes = %v", rep.writes)
	}
	// The whole report goes back; the logo uploaded in the app stays.
	b := rep.bodies["PUT /v1/api/reports/definitions/rd-globex"].(map[string]any)
	if b["brandLogoUrl"] != "https://cdn.acme.test/globex.png" || b["sendDay"] != float64(2) || b["sendHour"] != float64(8) || b["latencyThresholdMs"] != float64(800) ||
		strings.Join([]string{b["monitorUuids"].([]any)[0].(string), b["monitorUuids"].([]any)[1].(string)}, ",") != "app-web,hb-backup" {
		t.Errorf("update body = %v", b)
	}
	n := rep.bodies["POST /v1/api/reports/definitions"].(map[string]any)
	if n["scopeType"] != "tags" || n["tagIds"].([]any)[0] != float64(8) || n["cadence"] != "monthly" || n["sendTimezone"] != "Europe/Sarajevo" || n["brandLogoUrl"] != nil {
		t.Errorf("create body = %v", n)
	}
}

func TestReportsRefuseWhatTheyCantDo(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	reportNamed(f, "All monitors").Monitors = []string{"Website"}
	reportNamed(f, "Globex").Monitors = []string{"Website", "Nope"}
	if _, err := MakePlan(f, st, Options{}); err == nil || !strings.Contains(err.Error(), "default report covers every monitor") || !strings.Contains(err.Error(), "no monitor or heartbeat named Nope") {
		t.Errorf("err = %v", err)
	}

	_, err := Parse([]byte(`version: 2
reports:
  - name: A
    monitors: [Website]
    tags: [prod]
    schedule: monthly
    weekday: friday
    at: 9am
    brand: { color: blue }
  - name: a
    schedule: daily
`))
	for _, want := range []string{"monitors or tags, not both", "weekday is for a weekly schedule", `at "9am"`, `brand.color "blue"`, `schedule "daily"`, `two reports are named "a"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q missing from:\n%v", want, err)
		}
	}
}

func TestAFileWithoutReportsLeavesThemAlone(t *testing.T) {
	f, st, _, _, _, _ := roundTrip(t)
	f.Reports = nil
	p, err := MakePlan(f, st, Options{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 0 || len(p.NotInFile) != 0 {
		t.Errorf("changes = %v, notInFile = %v", p.Changes, p.NotInFile)
	}
}

// An MCP server monitor is its URL; with a key or required tools, its config.
func TestMCPMonitorTarget(t *testing.T) {
	cfg, err := desiredConfig(Monitor{Type: "mcp", Target: "mcp.acme.test/mcp"})
	if err != nil || cfg["uri"] != "https://mcp.acme.test/mcp" || len(cfg) != 2 {
		t.Fatalf("config = %v, %v", cfg, err)
	}
	if m := shorthand("mcp", cfg); m.Target != "https://mcp.acme.test/mcp" || m.Config != nil {
		t.Errorf("plain MCP monitor exports as %+v", m)
	}
	keyed := normalized(map[string]any{"uri": "https://mcp.acme.test/mcp", "headers": map[string]any{"Authorization": "Bearer upt_x"}, "requiredTools": []any{"get_status"}}).(map[string]any)
	if m := shorthand("mcp", keyed); m.Target != "" || m.Config["requiredTools"] == nil {
		t.Errorf("keyed MCP monitor exports as %+v", m)
	}
}
