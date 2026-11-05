package workspace

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Alerts are template alerts in workflows. Workflows made or edited in the
// editor aren't in the file; the plan names them so nobody wonders.

func alertEvents(a Alert) []string {
	if len(a.Events) == 0 {
		return []string{"opened", "resolved"}
	}
	out := append([]string{}, a.Events...)
	sort.Strings(out)
	return out
}

// pickIntegration finds the integration an alert posts to.
func pickIntegration(st *State, want string) (integration, error) {
	var byProvider []integration
	for _, i := range st.Integrations {
		if i.UUID == want || strings.EqualFold(i.Label(), want) || strings.EqualFold(i.Name, want) {
			if _, ok := alertProviders[i.Provider]; !ok {
				return i, fmt.Errorf("%s can't receive alerts; use a Slack, Discord, Teams, Telegram or PagerDuty integration", i.Label())
			}
			return i, nil
		}
		if strings.EqualFold(i.Provider, want) || strings.EqualFold(alertProviders[i.Provider], want) {
			byProvider = append(byProvider, i)
		}
	}
	switch len(byProvider) {
	case 1:
		return byProvider[0], nil
	case 0:
		return integration{}, fmt.Errorf("no integration %q is connected; connect it in the Uptimy app (Settings → Integrations)", want)
	default:
		labels := make([]string, 0, len(byProvider))
		for _, i := range byProvider {
			labels = append(labels, i.Label())
		}
		return integration{}, fmt.Errorf("more than one %s integration is connected; name one: %s", want, strings.Join(labels, ", "))
	}
}

// alertBody is what workflows' alert routes take. Monitors and tags may only
// exist once the earlier steps of apply have run, so this runs then.
func alertBody(a Alert, st *State) (map[string]any, error) {
	dest := map[string]any{}
	switch {
	case a.To.Integration != "":
		i, err := pickIntegration(st, a.To.Integration)
		if err != nil {
			return nil, err
		}
		dest = map[string]any{"type": "integration", "integrationUuid": i.UUID}
	case len(a.To.Emails) > 0:
		dest = map[string]any{"type": "email", "recipients": a.To.Emails}
	default:
		dest = map[string]any{"type": "webhook", "url": a.To.Webhook}
	}
	tags, err := tagIDsFor(st, a.Tags)
	if err != nil {
		return nil, err
	}
	sources := []string{}
	for _, name := range a.Monitors {
		_, uuid, ok := st.subject(name)
		if !ok {
			return nil, fmt.Errorf("no monitor or heartbeat named %q", name)
		}
		sources = append(sources, uuid)
	}
	severities := a.Severities
	if severities == nil {
		severities = []string{}
	}
	return map[string]any{"name": a.Name, "destination": dest, "events": alertEvents(a), "tagIds": tags,
		"severities": severities, "sourceUuids": sources, "enabled": !a.Paused}, nil
}

// sendsTo describes a live alert's destination the way the file writes it.
func sendsTo(la *liveAlert, st *State) AlertTo {
	switch la.Destination.Type {
	case "email":
		return AlertTo{Emails: la.Destination.Recipients}
	case "webhook":
		return AlertTo{Webhook: la.Destination.URL}
	default:
		for _, i := range st.Integrations {
			if i.UUID == la.Destination.IntegrationUUID {
				return AlertTo{Integration: i.Label()}
			}
		}
		return AlertTo{Integration: la.Destination.IntegrationUUID}
	}
}

func exportAlert(w liveWorkflow, st *State) Alert {
	la := w.Alert
	out := Alert{Name: w.Name, To: sendsTo(la, st), Tags: st.tagNames(la.TagIDs), Severities: la.Severities, Paused: !w.Enabled}
	events := append([]string{}, la.Events...)
	sort.Strings(events)
	if strings.Join(events, ",") != "opened,resolved" {
		out.Events = events
	}
	for _, u := range la.SourceUUIDs {
		if name := st.subjectName(u); name != "" {
			out.Monitors = append(out.Monitors, name)
		}
	}
	return out
}

func alertChanges(a Alert, w liveWorkflow, st *State) []string {
	var d []string
	la := w.Alert
	have, want := sendsTo(la, st), a.To
	switch {
	case want.Integration != "":
		i, err := pickIntegration(st, want.Integration)
		if err != nil || la.Destination.IntegrationUUID != i.UUID {
			d = append(d, fmt.Sprintf("to %s", want.Integration))
		}
	case len(want.Emails) > 0:
		if setDiff("", have.Emails, want.Emails) != "" || len(have.Emails) != len(want.Emails) {
			d = append(d, "to "+strings.Join(want.Emails, ", "))
		}
	default:
		if have.Webhook != want.Webhook {
			d = append(d, "to "+want.Webhook)
		}
	}
	liveEvents := append([]string{}, la.Events...)
	sort.Strings(liveEvents)
	if strings.Join(liveEvents, ",") != strings.Join(alertEvents(a), ",") {
		d = append(d, "events "+strings.Join(alertEvents(a), ", "))
	}
	if diff := setDiff("tags", st.tagNames(la.TagIDs), a.Tags); diff != "" {
		d = append(d, diff)
	}
	if diff := setDiff("severities", la.Severities, a.Severities); diff != "" {
		d = append(d, diff)
	}
	liveMonitors := []string{}
	for _, u := range la.SourceUUIDs {
		liveMonitors = append(liveMonitors, st.subjectName(u))
	}
	if diff := setDiff("monitors", liveMonitors, a.Monitors); diff != "" {
		d = append(d, diff)
	}
	if w.Enabled == a.Paused {
		if a.Paused {
			d = append(d, "pause")
		} else {
			d = append(d, "resume")
		}
	}
	return d
}

func createAlert(s Services, st *State, a Alert) error {
	b, err := alertBody(a, st)
	if err != nil {
		return err
	}
	_, err = s.Workflows.Post("/v1/api/alerts/", b)
	return err
}

func updateAlert(s Services, st *State, a Alert, w liveWorkflow) error {
	b, err := alertBody(a, st)
	if err != nil {
		return err
	}
	_, err = s.Workflows.Put("/v1/api/alerts/"+url.PathEscape(w.UUID), b)
	return err
}
