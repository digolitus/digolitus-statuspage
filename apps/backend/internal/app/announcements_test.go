package app

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAnnouncementTaxonomy(t *testing.T) {
	h := newHarness(t)
	cat := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-categories",
		map[string]any{"name": "Novidades", "color": "blue"}), http.StatusCreated).json(t)
	if str(cat, "color") != "blue" || cat["is_active"] != true {
		t.Fatalf("category: %v", cat)
	}
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-categories", map[string]any{"name": "novidades"}), http.StatusConflict)
	if r.errorCode(t) != "announcement_category_name_conflict" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-categories", map[string]any{"name": "X", "color": "pink"}, invalidRequest()), http.StatusUnprocessableEntity)

	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Plataforma"}), http.StatusCreated).json(t)
	a := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{
		"title": "Nova região", "body": "Texto.", "category_id": str(cat, "id"), "team_id": str(team, "id"),
	}), http.StatusCreated).json(t)
	if str(obj(a, "category"), "name") != "Novidades" || str(obj(a, "team"), "name") != "Plataforma" {
		t.Fatalf("refs: %v", a)
	}

	// In use: delete is refused, deactivation works and blocks new associations.
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/announcement-categories/"+str(cat, "id"), nil), http.StatusConflict)
	if r.errorCode(t) != "announcement_category_in_use" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/teams/"+str(team, "id"), nil), http.StatusConflict)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"is_active": false}), http.StatusOK)
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements",
		map[string]any{"title": "x", "body": "y", "team_id": str(team, "id")}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"team_id"`) {
		t.Fatalf("inactive team accepted: %s", r.body)
	}
	// Keeping an already-associated inactive team is allowed.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/announcements/"+str(a, "id"), map[string]any{"title": "Nova região (sa-east-1)"}), http.StatusOK)

	teams := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/teams?is_active=true", nil), http.StatusOK).json(t)
	if len(list(teams, "data")) != 0 {
		t.Fatalf("active teams: %v", teams)
	}
	empty := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcement-categories", map[string]any{"name": "Vazia"}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/announcement-categories/"+str(empty, "id"), nil), http.StatusNoContent)
	pub := mustStatus(t, h.call(http.MethodGet, "/api/v1/announcement-categories", nil), http.StatusOK).json(t)
	if len(list(pub, "data")) != 1 {
		t.Fatalf("public categories: %v", pub)
	}
}

func TestAnnouncementLifecycle(t *testing.T) {
	h := newHarness(t)
	api := h.createComponent("API", nil)
	hidden := h.createComponent("Interno", map[string]any{"is_public": false})

	a := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{
		"title": "Mudança no limite de requisições", "body": "A partir de **dezembro**:\n\n- 100 req/s\n- [Docs](https://docs.example.com)",
		"importance": "important", "is_pinned": true, "component_ids": []string{str(api, "id"), str(hidden, "id")},
	}), http.StatusCreated).json(t)
	id := str(a, "id")
	if str(a, "state") != "draft" || a["published_at"] != nil || len(list(a, "components")) != 2 {
		t.Fatalf("draft: %v", a)
	}
	// Drafts are not public.
	mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements/"+id, nil), http.StatusNotFound)
	if total(mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements", nil), http.StatusOK).json(t)) != 0 {
		t.Fatal("draft listed publicly")
	}

	// Scheduling: in the past is refused; in the future it is "scheduled".
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/publish",
		map[string]any{"publish_at": epoch.Add(-time.Hour).Format(time.RFC3339)}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), "publish_at") {
		t.Fatalf("past schedule: %s", r.body)
	}
	at := epoch.Add(2 * time.Hour)
	s := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/publish",
		map[string]any{"publish_at": at.Format(time.RFC3339)}), http.StatusOK).json(t)
	if str(s, "state") != "scheduled" {
		t.Fatalf("schedule: %v", s)
	}
	mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements/"+id, nil), http.StatusNotFound)
	sched := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/announcements?state=scheduled", nil), http.StatusOK).json(t)
	if total(sched) != 1 {
		t.Fatalf("state filter: %v", sched)
	}
	// Publishing twice is a conflict.
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/publish", map[string]any{}), http.StatusConflict)
	if r.errorCode(t) != "invalid_announcement_transition" {
		t.Fatalf("code = %s", r.errorCode(t))
	}

	h.clock.Advance(3 * time.Hour)
	p := mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements/"+id, nil), http.StatusOK).json(t)
	if len(list(p, "components")) != 1 || str(p, "importance") != "important" || p["is_pinned"] != true {
		t.Fatalf("public announcement (hidden component must be filtered): %v", p)
	}
	byComp := mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements?component_id="+str(api, "id")+"&pinned=true", nil), http.StatusOK).json(t)
	if total(byComp) != 1 {
		t.Fatalf("filters: %v", byComp)
	}
	if total(mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements?pinned=false", nil), http.StatusOK).json(t)) != 0 {
		t.Fatal("pinned=false must exclude pinned announcements")
	}

	// Published announcements cannot be deleted; archive hides, restore keeps the date.
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/announcements/"+id, nil), http.StatusConflict)
	if r.errorCode(t) != "announcement_not_draft" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	arch := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/archive", nil), http.StatusOK).json(t)
	if str(arch, "state") != "archived" || arch["archived_at"] == nil {
		t.Fatalf("archive: %v", arch)
	}
	mustStatus(t, h.call(http.MethodGet, "/api/v1/announcements/"+id, nil), http.StatusNotFound)
	rest := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/restore", nil), http.StatusOK).json(t)
	if str(rest, "state") != "published" || str(rest, "published_at") != str(s, "published_at") {
		t.Fatalf("restore: %v (scheduled %v)", rest, s["published_at"])
	}

	// Unpublish returns to draft, which can then be deleted.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+id+"/unpublish", nil), http.StatusOK)
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/announcements/"+id, nil), http.StatusNoContent)

	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=announcement&resource_id="+id+"&sort=occurred_at", nil), http.StatusOK).json(t)
	var actions []string
	for _, e := range list(audit, "data") {
		actions = append(actions, str(e.(map[string]any), "action"))
	}
	if strings.Join(actions, ",") != "created,published,archived,restored,unpublished,deleted" {
		t.Fatalf("audit actions = %v", actions)
	}
}

func TestAnnouncementValidationAndSearch(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements",
		map[string]any{"title": "", "body": "", "importance": "urgent"}, invalidRequest()), http.StatusUnprocessableEntity)
	for _, f := range []string{"title", "body", "importance"} {
		if !strings.Contains(string(r.body), `"field":"`+f+`"`) {
			t.Errorf("missing %s: %s", f, r.body)
		}
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Desconto_100%", "body": "x"}), http.StatusCreated)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Outro", "body": "x"}), http.StatusCreated)
	// Wildcards in the search term are literal.
	res := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/announcements?q=_100%25", nil), http.StatusOK).json(t)
	if total(res) != 1 {
		t.Fatalf("search: %v", res)
	}
	if total(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/announcements?q=%25", nil), http.StatusOK).json(t)) != 1 {
		t.Fatal("'%' must match literally")
	}
}
