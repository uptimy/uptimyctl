package workspace

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Reports are report definitions in the reports service, set as the app's
// report drawer sets them. The default report (every monitor) can be
// changed but never removed. A logo is uploaded in the app and kept.

const defaultSLA = 99.9

var weekdayNames = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

type liveReport struct {
	UUID               string   `json:"uuid"`
	Name               string   `json:"name"`
	IsDefault          bool     `json:"isDefault"`
	ScopeType          string   `json:"scopeType"`
	MonitorUUIDs       []string `json:"monitorUuids"`
	TagIDs             []int    `json:"tagIds"`
	SLATarget          float64  `json:"slaTarget"`
	LatencyThresholdMs *int     `json:"latencyThresholdMs"`
	Cadence            string   `json:"cadence"`
	SendDay            int      `json:"sendDay"`
	SendHour           int      `json:"sendHour"`
	SendMinute         int      `json:"sendMinute"`
	SendTimezone       string   `json:"sendTimezone"`
	EmailEnabled       bool     `json:"emailEnabled"`
	BrandName          *string  `json:"brandName"`
	BrandLogoURL       *string  `json:"brandLogoUrl"`
	BrandColor         *string  `json:"brandColor"`
	Recipients         []string `json:"recipients"`
}

// defaultTimezone is the default report's: the workspace's own.
func (st *State) defaultTimezone() string {
	for _, r := range st.Reports {
		if r.IsDefault && r.SendTimezone != "" {
			return r.SendTimezone
		}
	}
	return "UTC"
}

func weekdayIndex(name string) (int, bool) {
	for i, d := range weekdayNames {
		if strings.EqualFold(name, d) {
			return i, true
		}
	}
	return 0, false
}

// validateReport checks what can be checked offline.
func validateReport(r Report) []string {
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf("report %q: "+format, append([]any{r.Name}, args...)...))
	}
	if len(r.Monitors) > 0 && len(r.Tags) > 0 {
		add("give monitors or tags, not both")
	}
	if r.SLA != 0 && (r.SLA < 50 || r.SLA > 100) {
		add("sla must be 50-100")
	}
	if r.SlowAboveMs < 0 || r.SlowAboveMs > 600000 {
		add("slowAboveMs must be 1-600000")
	}
	switch r.Schedule {
	case "", "none", "weekly", "monthly", "quarterly":
	default:
		add("schedule %q: use weekly, monthly, quarterly or none", r.Schedule)
	}
	if r.Weekday != "" {
		if _, ok := weekdayIndex(r.Weekday); !ok {
			add("weekday %q: use monday … sunday", r.Weekday)
		}
		if r.Schedule != "weekly" {
			add("weekday is for a weekly schedule; monthly and quarterly ones take dayOfMonth")
		}
	}
	if r.DayOfMonth != 0 {
		if r.DayOfMonth < 1 || r.DayOfMonth > 28 {
			add("dayOfMonth must be 1-28")
		}
		if r.Schedule == "weekly" {
			add("dayOfMonth is for a monthly or quarterly schedule; weekly ones take weekday")
		}
	}
	if r.At != "" {
		if _, err := time.Parse("15:04", r.At); err != nil {
			add("at %q: use HH:MM, e.g. 09:00", r.At)
		}
	}
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			add("timezone %q: use an IANA timezone, e.g. Europe/Berlin", r.Timezone)
		}
	}
	if r.Brand != nil && r.Brand.Color != "" && !colorPattern.MatchString(r.Brand.Color) {
		add("brand.color %q: use #rrggbb", r.Brand.Color)
	}
	return errs
}

// reportWant is a file report with its defaults filled in.
type reportWant struct {
	Report
	sla      float64
	cadence  string
	sendDay  int
	hour     int
	minute   int
	timezone string
}

func resolveReport(r Report, st *State) reportWant {
	w := reportWant{Report: r, sla: r.SLA, cadence: r.Schedule, sendDay: 1, hour: 9, timezone: r.Timezone}
	if w.sla == 0 {
		w.sla = defaultSLA
	}
	// As the app: emailed monthly; nobody to email, read on demand.
	if w.cadence == "" {
		w.cadence = "monthly"
		if len(r.Emails) == 0 {
			w.cadence = "none"
		}
	}
	if i, ok := weekdayIndex(r.Weekday); ok {
		w.sendDay = i
	}
	if r.DayOfMonth != 0 {
		w.sendDay = r.DayOfMonth
	}
	if at, err := time.Parse("15:04", r.At); err == nil {
		w.hour, w.minute = at.Hour(), at.Minute()
	}
	if w.timezone == "" {
		w.timezone = st.defaultTimezone()
	}
	return w
}

func (w reportWant) scope() string {
	switch {
	case len(w.Monitors) > 0:
		return "monitors"
	case len(w.Tags) > 0:
		return "tags"
	}
	return "workspace"
}

func scheduleText(cadence string, day, hour, minute int, tz string) string {
	switch cadence {
	case "none":
		return "not emailed"
	case "weekly":
		name := strconv.Itoa(day)
		if day >= 0 && day < 7 {
			name = weekdayNames[day]
		}
		return fmt.Sprintf("weekly on %s at %02d:%02d (%s)", name, hour, minute, tz)
	}
	return fmt.Sprintf("%s on day %d at %02d:%02d (%s)", cadence, day, hour, minute, tz)
}

func (w reportWant) schedule() string {
	return scheduleText(w.cadence, w.sendDay, w.hour, w.minute, w.timezone)
}

func (r liveReport) schedule() string {
	return scheduleText(r.Cadence, r.SendDay, r.SendHour, r.SendMinute, r.SendTimezone)
}

func (w reportWant) describe() string {
	covers := "every monitor"
	switch w.scope() {
	case "monitors":
		covers = strings.Join(w.Monitors, ", ")
	case "tags":
		covers = "tags " + strings.Join(w.Tags, ", ")
	}
	return fmt.Sprintf("%s, SLA %g%%, %s", covers, w.sla, w.schedule())
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (w reportWant) brand() (name, color string) {
	if w.Brand == nil {
		return "", ""
	}
	return w.Brand.Name, strings.ToLower(w.Brand.Color)
}

func reportChanges(w reportWant, live liveReport, st *State) []string {
	var d []string
	switch {
	case live.ScopeType != w.scope():
		d = append(d, fmt.Sprintf("covers %s → %s", live.ScopeType, w.scope()))
	case w.scope() == "monitors":
		if diff := setDiff("monitors", sourceNames(live.MonitorUUIDs, st), w.Monitors); diff != "" {
			d = append(d, diff)
		}
	case w.scope() == "tags":
		if diff := setDiff("tags", st.tagNames(live.TagIDs), w.Tags); diff != "" {
			d = append(d, diff)
		}
	}
	if math.Abs(live.SLATarget-w.sla) > 0.0005 {
		d = append(d, fmt.Sprintf("SLA %g%% → %g%%", live.SLATarget, w.sla))
	}
	if have := live.LatencyThresholdMs; (have == nil) != (w.SlowAboveMs == 0) || have != nil && *have != w.SlowAboveMs {
		d = append(d, fmt.Sprintf("slow above %s → %s", slowText(have), slowText(&w.SlowAboveMs)))
	}
	if s := live.schedule(); s != w.schedule() {
		d = append(d, fmt.Sprintf("schedule %s → %s", s, w.schedule()))
	}
	if diff := setDiff("emails", live.Recipients, w.Emails); diff != "" {
		d = append(d, diff)
	}
	if live.EmailEnabled == w.Paused {
		if w.Paused {
			d = append(d, "stop emails")
		} else {
			d = append(d, "send emails")
		}
	}
	name, color := w.brand()
	if deref(live.BrandName) != name || strings.ToLower(deref(live.BrandColor)) != color {
		d = append(d, fmt.Sprintf("brand %q %s → %q %s", deref(live.BrandName), deref(live.BrandColor), name, color))
	}
	return d
}

func slowText(ms *int) string {
	if ms == nil || *ms == 0 {
		return "off"
	}
	return fmt.Sprintf("%dms", *ms)
}

// reportBody is the whole definition the reports service takes on create
// and update. Monitors may only exist once apply's earlier steps have run.
func reportBody(w reportWant, live *liveReport, st *State) (map[string]any, error) {
	b := map[string]any{"name": w.Name, "scopeType": w.scope(), "monitorUuids": nil, "tagIds": nil,
		"slaTarget": w.sla, "latencyThresholdMs": nil, "cadence": w.cadence, "sendDay": w.sendDay,
		"sendHour": w.hour, "sendMinute": w.minute, "sendTimezone": w.timezone, "emailEnabled": !w.Paused,
		"recipients": w.Emails, "brandName": nil, "brandLogoUrl": nil, "brandColor": nil}
	if w.Emails == nil {
		b["recipients"] = []string{}
	}
	if w.SlowAboveMs > 0 {
		b["latencyThresholdMs"] = w.SlowAboveMs
	}
	if name, color := w.brand(); name != "" || color != "" {
		if name != "" {
			b["brandName"] = name
		}
		if color != "" {
			b["brandColor"] = color
		}
	}
	if live != nil && live.BrandLogoURL != nil {
		b["brandLogoUrl"] = *live.BrandLogoURL
	}
	switch w.scope() {
	case "monitors":
		uuids := []string{}
		for _, name := range w.Monitors {
			_, uuid, ok := st.subject(name)
			switch {
			case ok:
				uuids = append(uuids, uuid)
			case isUUID(name):
				// A deleted monitor's UUID stays, as in alerts.
				uuids = append(uuids, name)
			default:
				return nil, fmt.Errorf("no monitor or heartbeat named %q", name)
			}
		}
		b["monitorUuids"] = uuids
	case "tags":
		tags, err := tagIDsFor(st, w.Tags)
		if err != nil {
			return nil, err
		}
		b["tagIds"] = tags
	}
	return b, nil
}

func createReport(s Services, st *State, w reportWant) error {
	b, err := reportBody(w, nil, st)
	if err != nil {
		return err
	}
	_, err = s.Reports.Post("/v1/api/reports/definitions", b)
	return err
}

func updateReport(s Services, st *State, w reportWant, live liveReport) error {
	b, err := reportBody(w, &live, st)
	if err != nil {
		return err
	}
	_, err = s.Reports.Put("/v1/api/reports/definitions/"+url.PathEscape(live.UUID), b)
	return err
}

// exportReport writes a live report as the file has it, defaults left out.
func exportReport(r liveReport, st *State) Report {
	out := Report{Name: r.Name, Emails: r.Recipients, Paused: !r.EmailEnabled}
	switch r.ScopeType {
	case "monitors":
		out.Monitors = sourceNames(r.MonitorUUIDs, st)
	case "tags":
		out.Tags = st.tagNames(r.TagIDs)
	}
	if math.Abs(r.SLATarget-defaultSLA) > 0.0005 {
		out.SLA = r.SLATarget
	}
	if r.LatencyThresholdMs != nil {
		out.SlowAboveMs = *r.LatencyThresholdMs
	}
	defaultSchedule := "monthly"
	if len(r.Recipients) == 0 {
		defaultSchedule = "none"
	}
	if r.Cadence != defaultSchedule {
		out.Schedule = r.Cadence
	}
	switch {
	case r.Cadence == "weekly" && r.SendDay != 1 && r.SendDay >= 0 && r.SendDay < 7:
		out.Weekday = weekdayNames[r.SendDay]
	case r.Cadence != "weekly" && r.Cadence != "none" && r.SendDay != 1:
		out.DayOfMonth = r.SendDay
	}
	if r.SendHour != 9 || r.SendMinute != 0 {
		out.At = fmt.Sprintf("%02d:%02d", r.SendHour, r.SendMinute)
	}
	if r.SendTimezone != st.defaultTimezone() {
		out.Timezone = r.SendTimezone
	}
	if deref(r.BrandName) != "" || deref(r.BrandColor) != "" {
		out.Brand = &ReportBrand{Name: deref(r.BrandName), Color: deref(r.BrandColor)}
	}
	return out
}
