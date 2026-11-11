package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
)

// history lists the admin status history of a component, oldest first.
func history(t *testing.T, h *harness, componentID string) []map[string]any {
	t.Helper()
	l := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/status-history?limit=100&component_id="+componentID, nil), http.StatusOK).json(t), "data")
	out := make([]map[string]any, len(l))
	for i, e := range l {
		out[len(l)-1-i] = e.(map[string]any)
	}
	return out
}

func statusOf(e map[string]any) string {
	if e["status"] == nil {
		return "<nil>"
	}
	return e["status"].(string)
}

func TestStatusHistoryRecordsEffectiveChanges(t *testing.T) {
	h := newHarness(t)
	c := h.createComponent("API", nil)
	id := str(c, "id")

	// Declared change.
	h.clock.Advance(time.Hour)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+id, map[string]any{"status": "degraded_performance"}), http.StatusOK)
	// An incident worse than the declared status raises the effective status.
	h.clock.Advance(time.Hour)
	inc := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "API fora", "message": "Investigando.", "impact": "major_outage", "component_ids": []string{id}}), http.StatusCreated).json(t)
	// A message without status change records nothing.
	h.clock.Advance(time.Minute)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+str(inc, "id")+"/updates", map[string]any{
		"status": "identified", "message": "Causa encontrada."}), http.StatusCreated)
	// Resolving restores the declared status.
	h.clock.Advance(time.Hour)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+str(inc, "id")+"/resolve", map[string]any{"message": "Ok."}), http.StatusOK)
	// Maintenance never hides a worse declared status, but shows over operational.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+id, map[string]any{"status": "operational"}), http.StatusOK)
	mt := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances", map[string]any{
		"title": "Janela", "timezone": "UTC", "scheduled_start": "2026-03-02T00:00:00Z", "scheduled_end": "2026-03-02T01:00:00Z",
		"component_ids": []string{id}}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+str(mt, "id")+"/start", nil), http.StatusOK)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+str(mt, "id")+"/complete", nil), http.StatusOK)
	// Deactivation stops tracking (status null).
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+id, map[string]any{"is_active": false}), http.StatusOK)

	got := history(t, h, id)
	want := []struct{ status, cause string }{
		{"operational", "component"},
		{"degraded_performance", "component"},
		{"major_outage", "incident"},
		{"degraded_performance", "incident"},
		{"operational", "component"},
		{"maintenance", "maintenance"},
		{"operational", "maintenance"},
		{"<nil>", "component"},
	}
	if len(got) != len(want) {
		t.Fatalf("history has %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if statusOf(got[i]) != w.status || str(got[i], "cause_type") != w.cause {
			t.Fatalf("entry %d = %s/%s, want %s/%s (%v)", i, statusOf(got[i]), str(got[i], "cause_type"), w.status, w.cause, got)
		}
	}
	if str(got[2], "cause_name") != "API fora" || got[2]["actor"] == nil || obj(got[2], "actor")["name"] == "" {
		t.Fatalf("incident entry: %v", got[2])
	}
	if got[0]["previous_status"] != nil || str(got[1], "previous_status") != "operational" {
		t.Fatalf("previous statuses: %v", got)
	}

	// Filters are validated.
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/status-history?cause_type=nope", nil, invalidRequest()), http.StatusUnprocessableEntity)
	l := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/status-history?cause_type=incident", nil), http.StatusOK).json(t)
	if total(l) != 2 {
		t.Fatalf("incident entries = %d", total(l))
	}
}

func TestPublicUptime(t *testing.T) {
	// Sessions must survive the clock jumping days ahead.
	h := newHarnessWith(t, func(o *Options) { o.Auth.SessionTTL, o.Auth.IdleTimeout = 30*24*time.Hour, 30*24*time.Hour })
	api := str(h.createComponent("API", nil), "id")
	str(h.createComponent("Interno", map[string]any{"is_public": false}), "id")

	// Two days later: a 1 h major outage caused by an incident.
	h.clock.Advance(48 * time.Hour)
	inc := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "API fora", "message": "Investigando.", "impact": "major_outage", "component_ids": []string{api}}), http.StatusCreated).json(t)
	h.clock.Advance(time.Hour)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+str(inc, "id")+"/resolve", map[string]any{"message": "Ok."}), http.StatusOK)
	h.clock.Advance(time.Hour)

	pub := h.anonymous()
	u := mustStatus(t, pub.call(http.MethodGet, "/api/v1/status/uptime?days=30&timezone=America/Sao_Paulo", nil), http.StatusOK).json(t)
	comps := list(u, "components")
	if len(comps) != 1 || str(comps[0].(map[string]any), "id") != api {
		t.Fatalf("only public components: %v", comps)
	}
	c := comps[0].(map[string]any)
	days := list(c, "days")
	if len(days) != 30 || u["days"].(float64) != 30 || str(u, "timezone") != "America/Sao_Paulo" {
		t.Fatalf("range: %v", u)
	}
	// Before the component existed: no data.
	if str(days[0].(map[string]any), "status") != "no_data" {
		t.Fatalf("first day: %v", days[0])
	}
	var outageDay map[string]any
	for _, d := range days {
		if m := d.(map[string]any); str(m, "status") == "major_outage" {
			outageDay = m
		}
	}
	if outageDay == nil || obj(outageDay, "durations")["major_outage"].(float64) != 3600 {
		t.Fatalf("outage day missing: %v", days)
	}
	if inc := list(outageDay, "incidents"); len(inc) != 1 || str(inc[0].(map[string]any), "title") != "API fora" {
		t.Fatalf("incidents of the day: %v", outageDay)
	}
	// 50 h tracked, 1 h down.
	if p := c["uptime_percent"].(float64); p != 98 {
		t.Fatalf("uptime = %v", p)
	}

	for _, q := range []string{"days=7", "days=abc", "timezone=Local", "timezone=Mars/Base"} {
		r := mustStatus(t, pub.call(http.MethodGet, "/api/v1/status/uptime?"+q, nil, invalidRequest()), http.StatusUnprocessableEntity)
		if r.errorCode(t) != "validation_failed" {
			t.Fatalf("%s: %s", q, r.body)
		}
	}
}

func TestExternalHistory(t *testing.T) {
	h := newHarness(t)
	snap := githubSnapshot("operational")
	snap.Events[0].ID = "inc-1"
	h.fetcher.set(githubBase, snap, nil)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": githubBase}), http.StatusCreated).json(t), "id")
	comps := externalComponents(t, h, id)
	apiID := str(comps["API Requests"], "id")
	mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/external-sources/"+id+"/selection", map[string]any{"component_ids": []string{apiID}}), http.StatusOK)

	// The provider reports an outage, then recovers and closes the incident.
	h.clock.Advance(time.Hour)
	snap = githubSnapshot("major_outage")
	snap.Events[0].ID = "inc-1"
	h.fetcher.set(githubBase, snap, nil)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources/"+id+"/sync", nil), http.StatusOK)
	h.clock.Advance(30 * time.Minute)
	snap = githubSnapshot("operational")
	snap.Events = []external.Event{snap.Events[1]}
	h.fetcher.set(githubBase, snap, nil)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources/"+id+"/sync", nil), http.StatusOK)

	outs := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/outages?only_selected=true", nil), http.StatusOK).json(t), "data")
	if len(outs) != 1 {
		t.Fatalf("outages: %v", outs)
	}
	if o := outs[0].(map[string]any); str(o, "worst_status") != "major_outage" || o["duration_seconds"].(float64) != 1800 || str(o, "component_name") != "API Requests" {
		t.Fatalf("outage: %v", o)
	}
	// Codespaces has been in major_outage since registration: ongoing, not listed.
	all := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/outages", nil), http.StatusOK).json(t)
	if total(all) != 1 {
		t.Fatalf("all outages: %v", all)
	}

	// The history lists resolved events only; ongoing ones are listed apart.
	resolved := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/events", nil), http.StatusOK).json(t), "data")
	if len(resolved) != 1 || str(resolved[0].(map[string]any), "name") != "API degraded" || resolved[0].(map[string]any)["closed_at"] == nil ||
		list(resolved[0].(map[string]any), "component_names")[0] != "API Requests" {
		t.Fatalf("resolved events: %v", resolved)
	}
	open := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/events?state=open", nil), http.StatusOK).json(t), "data")
	if len(open) != 1 || str(open[0].(map[string]any), "name") != "Codespaces down" || open[0].(map[string]any)["closed_at"] != nil {
		t.Fatalf("open events: %v", open)
	}

	u := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/uptime?days=30", nil), http.StatusOK).json(t)
	uc := list(u, "components")
	if len(uc) != 1 || str(uc[0].(map[string]any), "name") != "API Requests" {
		t.Fatalf("uptime components: %v", uc)
	}
	last := list(uc[0].(map[string]any), "days")[29].(map[string]any)
	if str(last, "status") != "major_outage" || obj(last, "durations")["major_outage"].(float64) != 1800 {
		t.Fatalf("today: %v", last)
	}

	// Deleting the source keeps nothing reachable through it, but the rows stay.
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/external-sources/"+id, nil), http.StatusNoContent)
	var kept int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM external_component_status_changes WHERE source_id IS NULL").Scan(&kept); err != nil || kept != 5 {
		t.Fatalf("kept = %d, %v", kept, err)
	}
}
