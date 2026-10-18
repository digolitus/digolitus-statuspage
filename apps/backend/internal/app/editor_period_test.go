package app

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Real incident period: the impact may start before it is opened here and
// end before it is closed here.
func TestIncidentRealPeriod(t *testing.T) {
	h := newHarness(t)
	start := epoch.Add(-26 * time.Hour)
	inc := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Falha noturna", "message": "Investigando.", "impact": "partial_outage", "started_at": start.Format(time.RFC3339),
	}), http.StatusCreated).json(t)
	id := str(inc, "id")

	// The end is only for resolved incidents.
	r := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id, map[string]any{"resolved_at": epoch.Format(time.RFC3339)}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"resolved_at"`) {
		t.Fatalf("end before resolution: %s", r.body)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/updates", map[string]any{
		"status": "monitoring", "message": "m", "resolved_at": epoch.Format(time.RFC3339),
	}, invalidRequest()), http.StatusUnprocessableEntity)
	for _, bad := range []string{start.Add(-time.Minute).Format(time.RFC3339), epoch.Add(time.Hour).Format(time.RFC3339), "ontem"} {
		mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/resolve", map[string]any{"message": "ok", "resolved_at": bad}, invalidRequest()), http.StatusUnprocessableEntity)
	}

	// Closed the next day, but the impact ended the previous night.
	end := epoch.Add(-14 * time.Hour)
	res := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/resolve", map[string]any{
		"message": "Resolvido.", "resolved_at": end.Format(time.RFC3339),
	}), http.StatusOK).json(t)
	if str(res, "started_at") != "2026-02-28T10:00:00Z" || str(res, "resolved_at") != "2026-02-28T22:00:00Z" {
		t.Fatalf("period: %v .. %v", res["started_at"], res["resolved_at"])
	}
	pub := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/incidents/"+id, nil), http.StatusOK).json(t)
	if str(pub, "resolved_at") != "2026-02-28T22:00:00Z" {
		t.Fatalf("public end: %v", pub["resolved_at"])
	}
	// The closing update keeps the time it was actually published.
	upd := list(pub, "updates")[0].(map[string]any)
	if str(upd, "created_at") != "2026-03-01T12:00:00Z" {
		t.Fatalf("update time: %v", upd["created_at"])
	}

	// Both ends can be corrected later, and must stay ordered.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id, map[string]any{"resolved_at": start.Add(-time.Hour).Format(time.RFC3339)}, invalidRequest()), http.StatusUnprocessableEntity)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id, map[string]any{"started_at": end.Add(time.Hour).Format(time.RFC3339)}, invalidRequest()), http.StatusUnprocessableEntity)
	fixed := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id, map[string]any{
		"started_at": "2026-02-28T07:30:00-03:00", "resolved_at": "2026-02-28T20:15:00-03:00",
	}), http.StatusOK).json(t)
	if str(fixed, "started_at") != "2026-02-28T10:30:00Z" || str(fixed, "resolved_at") != "2026-02-28T23:15:00Z" {
		t.Fatalf("corrected period: %v .. %v", fixed["started_at"], fixed["resolved_at"])
	}

	// Without resolved_at the end is the publication time, as before.
	other := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "Outro", "message": "m", "impact": "operational"}), http.StatusCreated).json(t)
	h.clock.Advance(time.Hour)
	done := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+str(other, "id")+"/resolve", map[string]any{"message": "ok"}), http.StatusOK).json(t)
	if str(done, "resolved_at") != "2026-03-01T13:00:00Z" {
		t.Fatalf("default end: %v", done["resolved_at"])
	}
}

func TestComponentDescriptionVisibility(t *testing.T) {
	h := newHarness(t)
	hidden := h.createComponent("Banco interno", map[string]any{"description": "Cluster primário em us-east-1"})
	shown := h.createComponent("API", map[string]any{"description": "Endpoints REST públicos", "show_description": true})
	if hidden["show_description"] != false || shown["show_description"] != true {
		t.Fatalf("defaults: %v %v", hidden["show_description"], shown["show_description"])
	}
	descs := func() map[string]string {
		out := map[string]string{}
		for _, c := range list(mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/components", nil), http.StatusOK).json(t), "data") {
			m := c.(map[string]any)
			out[str(m, "name")] = str(m, "description")
		}
		return out
	}
	if d := descs(); d["Banco interno"] != "" || d["API"] != "Endpoints REST públicos" {
		t.Fatalf("public descriptions: %v", d)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(hidden, "id"), map[string]any{"show_description": true}), http.StatusOK)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(shown, "id"), map[string]any{"show_description": false}), http.StatusOK)
	if d := descs(); d["Banco interno"] != "Cluster primário em us-east-1" || d["API"] != "" {
		t.Fatalf("after toggle: %v", d)
	}
	// The summary used by the page applies the same rule.
	for _, c := range list(mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t), "components") {
		if m := c.(map[string]any); str(m, "name") == "API" && str(m, "description") != "" {
			t.Fatal("hidden description leaked in /status")
		}
	}
}

func TestTeamTags(t *testing.T) {
	h := newHarness(t)
	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{
		"name": "Squad Pagamentos", "tags": []string{" pagamentos ", "On-call  24x7", "PAGAMENTOS", ""},
	}), http.StatusCreated).json(t)
	tags := list(team, "tags")
	if len(tags) != 2 || tags[0] != "pagamentos" || tags[1] != "On-call 24x7" {
		t.Fatalf("normalized tags: %v", tags)
	}
	r := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"tags": []string{strings.Repeat("x", 31)}}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"tags"`) {
		t.Fatalf("long tag: %s", r.body)
	}
	cleared := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"tags": []string{}}), http.StatusOK).json(t)
	if len(list(cleared, "tags")) != 0 {
		t.Fatalf("cleared: %v", cleared["tags"])
	}
	plain := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Sem tags"}), http.StatusCreated).json(t)
	if tags, ok := plain["tags"].([]any); !ok || len(tags) != 0 {
		t.Fatalf("default tags: %v", plain["tags"])
	}
}

func TestAnnouncementTemplates(t *testing.T) {
	h := newHarness(t)
	seeded := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/announcement-templates?is_active=true", nil), http.StatusOK).json(t)
	if total(seeded) != 3 {
		t.Fatalf("seeded templates: %d", total(seeded))
	}
	cat := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-categories", map[string]any{"name": "Novidades", "color": "blue"}), http.StatusCreated).json(t)
	tpl := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-templates", map[string]any{
		"name": "Janela de deploy", "title": "Deploy em [data]", "body": "## Quando\n- [data]", "importance": "important",
		"category_id": str(cat, "id"), "guidance": "Avise com 2 dias.",
	}), http.StatusCreated).json(t)
	if str(tpl, "importance") != "important" || str(tpl, "category_id") != str(cat, "id") || tpl["is_active"] != true {
		t.Fatalf("template: %v", tpl)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-templates", map[string]any{"name": "janela de DEPLOY", "title": "x", "body": "x"}), http.StatusConflict)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-templates", map[string]any{"name": "Sem corpo", "title": "x"}, invalidRequest()), http.StatusUnprocessableEntity)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-templates", map[string]any{
		"name": "Categoria inexistente", "title": "x", "body": "x", "category_id": "00000000-0000-4000-8000-000000000000",
	}), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"category_id"`) {
		t.Fatalf("missing category: %s", r.body)
	}
	upd := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/announcement-templates/"+str(tpl, "id"), map[string]any{"category_id": nil, "is_active": false}), http.StatusOK).json(t)
	if upd["category_id"] != nil || upd["is_active"] != false {
		t.Fatalf("patched: %v", upd)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/announcement-templates/"+str(tpl, "id"), map[string]any{}, invalidRequest()), http.StatusUnprocessableEntity)

	// Anyone signed in can use templates; only templates.manage changes them.
	writer := h.userWith("Redação", "announcements.write")
	mustStatus(t, writer.call(http.MethodGet, "/api/v1/admin/announcement-templates", nil), http.StatusOK)
	mustStatus(t, writer.call(http.MethodPost, "/api/v1/admin/announcement-templates", map[string]any{"name": "x", "title": "x", "body": "x"}), http.StatusForbidden)
	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=announcement_template", nil), http.StatusOK).json(t)
	if total(audit) != 2 {
		t.Fatalf("audit events: %d", total(audit))
	}
}
