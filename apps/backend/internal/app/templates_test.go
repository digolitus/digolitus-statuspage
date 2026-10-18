package app

import (
	"net/http"
	"testing"
)

func TestSeededTemplates(t *testing.T) {
	h := newHarness(t)
	it := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incident-templates?limit=100", nil), http.StatusOK).json(t)
	mt := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/maintenance-templates?limit=100", nil), http.StatusOK).json(t)
	if total(it) != 7 || total(mt) != 6 {
		t.Fatalf("seed templates: incident=%d maintenance=%d", total(it), total(mt))
	}
}

func TestTemplatesDoNotRewriteHistory(t *testing.T) {
	h := newHarness(t)
	tpl := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incident-templates", map[string]any{
		"name": "Banco lento", "title": "Lentidão no banco", "message": "Investigando o banco.", "impact": "minor",
	}), http.StatusCreated).json(t)
	tid := str(tpl, "id")

	// The client fills the form from the template, then creates the incident.
	inc := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": str(tpl, "title"), "message": str(tpl, "message"), "impact": str(tpl, "impact"), "template_id": tid,
	}), http.StatusCreated).json(t)

	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incident-templates/"+tid,
		map[string]any{"title": "Outro título", "message": "Outra mensagem", "is_active": false}), http.StatusOK)

	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incidents/"+str(inc, "id"), nil), http.StatusOK).json(t)
	if str(got, "title") != "Lentidão no banco" || str(got, "template_id") != tid ||
		str(list(got, "updates")[0].(map[string]any), "message") != "Investigando o banco." {
		t.Fatalf("incident changed after template edit: %v", got)
	}

	active := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incident-templates?is_active=false", nil), http.StatusOK).json(t)
	if total(active) != 1 {
		t.Fatalf("deactivated template filter: %v", active)
	}
}

func TestTemplateValidationAndConflicts(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incident-templates",
		map[string]any{"name": "degradação de desempenho", "title": "t", "message": "m", "impact": "minor"}), http.StatusConflict)
	if r.errorCode(t) != "template_name_conflict" {
		t.Fatalf("names are unique ignoring case: %s", r.body)
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incident-templates", map[string]any{"name": "x"}, invalidRequest()), http.StatusUnprocessableEntity)
	if len(list(obj(r.json(t), "error"), "details")) < 3 {
		t.Fatalf("expected title, message and impact errors: %s", r.body)
	}

	mt := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenance-templates", map[string]any{
		"name": "Reinício", "title": "Reinício programado", "message": "Reiniciaremos o serviço.",
	}), http.StatusCreated).json(t)
	if str(mt, "expected_impact") != "none" {
		t.Fatalf("default expected impact: %v", mt)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenance-templates/"+str(mt, "id"),
		map[string]any{"expected_impact": "partial_outage"}), http.StatusOK)
	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenance-templates/"+str(mt, "id"),
		map[string]any{"impact": "minor"}, invalidRequest()), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("incident-only field must be rejected on maintenance templates: %s", r.body)
	}
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/maintenance-templates/00000000-0000-4000-8000-000000000000", nil), http.StatusNotFound)
}
