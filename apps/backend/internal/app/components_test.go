package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestComponentsCRUD(t *testing.T) {
	h := newHarness(t)

	c := h.createComponent("API Pública", map[string]any{"description": "Gateway REST"})
	if str(c, "slug") != "api-publica" || str(c, "status") != "operational" || c["is_public"] != true || c["is_active"] != true {
		t.Fatalf("unexpected defaults: %v", c)
	}
	id := str(c, "id")

	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components/"+id, nil), http.StatusOK).json(t)
	if str(got, "name") != "API Pública" {
		t.Fatalf("get returned %v", got)
	}

	upd := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+id,
		map[string]any{"status": "degraded_performance", "display_order": 5}), http.StatusOK).json(t)
	if str(upd, "status") != "degraded_performance" || upd["display_order"].(float64) != 5 {
		t.Fatalf("patch returned %v", upd)
	}

	// Slug must be unique.
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Outra", "slug": "api-publica"}), http.StatusConflict)
	if r.errorCode(t) != "component_slug_conflict" {
		t.Fatalf("code = %s", r.errorCode(t))
	}

	// Deactivation keeps the component but hides it from the public page.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+id, map[string]any{"is_active": false}), http.StatusOK)
	pub := mustStatus(t, h.call(http.MethodGet, "/api/v1/components", nil), http.StatusOK).json(t)
	if total(pub) != 0 {
		t.Fatalf("inactive component must not be public: %v", pub)
	}
	inactive := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?is_active=false", nil), http.StatusOK).json(t)
	if total(inactive) != 1 {
		t.Fatalf("expected one inactive component, got %v", inactive)
	}

	// Audit trail: created, status_changed, deactivated.
	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=component&resource_id="+id+"&sort=occurred_at", nil), http.StatusOK).json(t)
	var actions []string
	for _, e := range list(audit, "data") {
		actions = append(actions, str(e.(map[string]any), "action"))
	}
	if strings.Join(actions, ",") != "created,status_changed,deactivated" {
		t.Fatalf("audit actions = %v", actions)
	}
}

func TestComponentValidation(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components",
		map[string]any{"name": "", "status": "broken", "display_order": -1}, invalidRequest()), http.StatusUnprocessableEntity)
	details := list(obj(r.json(t), "error"), "details")
	fields := map[string]bool{}
	for _, d := range details {
		fields[str(d.(map[string]any), "field")] = true
	}
	for _, f := range []string{"name", "status", "display_order"} {
		if !fields[f] {
			t.Errorf("missing validation detail for %s: %s", f, r.body)
		}
	}

	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(h.createComponent("X", nil), "id"), map[string]any{}, invalidRequest()), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "empty_update" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
}

func TestComponentListPaginationAndSort(t *testing.T) {
	h := newHarness(t)
	for i := range 5 {
		h.createComponent(fmt.Sprintf("Serviço %d", i), map[string]any{"display_order": 5 - i})
	}
	page := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?limit=2&offset=1", nil), http.StatusOK).json(t)
	data := list(page, "data")
	if total(page) != 5 || len(data) != 2 {
		t.Fatalf("pagination: %v", page)
	}
	// Default order is display_order ascending: Serviço 4 (1), Serviço 3 (2), Serviço 2 (3)...
	if str(data[0].(map[string]any), "name") != "Serviço 3" || str(data[1].(map[string]any), "name") != "Serviço 2" {
		t.Fatalf("unexpected order: %v", data)
	}
	byName := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?sort=-name&limit=1", nil), http.StatusOK).json(t)
	if str(list(byName, "data")[0].(map[string]any), "name") != "Serviço 4" {
		t.Fatalf("sort=-name: %v", byName)
	}
	r := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?sort=password&limit=500", nil, invalidRequest()), http.StatusUnprocessableEntity)
	if len(list(obj(r.json(t), "error"), "details")) != 2 {
		t.Fatalf("expected errors for sort and limit: %s", r.body)
	}
}
