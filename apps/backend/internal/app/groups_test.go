package app

import (
	"net/http"
	"testing"
)

func TestComponentGroups(t *testing.T) {
	h := newHarness(t)

	g := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups",
		map[string]any{"name": "Pagamentos", "description": "Cobrança", "display_order": 1}), http.StatusCreated).json(t)
	gid := str(g, "id")
	if g["component_count"].(float64) != 0 {
		t.Fatalf("new group must be empty: %v", g)
	}

	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "pagamentos"}), http.StatusConflict)
	if r.errorCode(t) != "component_group_name_conflict" {
		t.Fatalf("names are unique ignoring case: %s", r.body)
	}

	api := h.createComponent("API de Pagamentos", map[string]any{"group_id": gid})
	if str(api, "group_id") != gid {
		t.Fatalf("component not grouped: %v", api)
	}
	checkout := h.createComponent("Checkout", nil)
	moved := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(checkout, "id"), map[string]any{"group_id": gid}), http.StatusOK).json(t)
	if str(moved, "group_id") != gid {
		t.Fatalf("group not assigned on update: %v", moved)
	}
	h.createComponent("Portal", nil)

	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components",
		map[string]any{"name": "X", "group_id": "7d0e6f1c-9d1f-4a42-8f34-0a1b2c3d4e5f"}), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("unknown group must be rejected: %s", r.body)
	}

	inGroup := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?group_id="+gid, nil), http.StatusOK).json(t)
	if total(inGroup) != 2 {
		t.Fatalf("group filter: %v", inGroup)
	}

	// Public summary: group status is derived from its components.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Falha no checkout", "message": "Investigando.", "impact": "major", "component_ids": []string{str(checkout, "id")},
	}), http.StatusCreated)
	sum := mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	groups := list(sum, "groups")
	if len(groups) != 1 {
		t.Fatalf("expected one public group: %v", groups)
	}
	pg := groups[0].(map[string]any)
	if str(pg, "status") != "partial_outage" || pg["component_count"].(float64) != 2 || pg["affected_components"].(float64) != 1 {
		t.Fatalf("derived group status: %v", pg)
	}
	grouped := 0
	for _, c := range list(sum, "components") {
		if str(c.(map[string]any), "group_id") == gid {
			grouped++
		}
	}
	if grouped != 2 || len(list(sum, "components")) != 3 {
		t.Fatalf("components keep group_id in the flat list: %v", list(sum, "components"))
	}

	// A non-empty group cannot be deleted.
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/component-groups/"+gid, nil), http.StatusConflict)
	if r.errorCode(t) != "component_group_not_empty" {
		t.Fatalf("code = %s", r.errorCode(t))
	}

	// null removes components from the group; then the group can be deleted.
	for _, c := range []map[string]any{api, checkout} {
		u := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(c, "id"), map[string]any{"group_id": nil}), http.StatusOK).json(t)
		if u["group_id"] != nil {
			t.Fatalf("group_id must be cleared: %v", u)
		}
	}
	sum = mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	if len(list(sum, "groups")) != 0 {
		t.Fatalf("groups without public components are not published: %v", list(sum, "groups"))
	}
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/component-groups/"+gid, nil), http.StatusNoContent)
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/component-groups/"+gid, nil), http.StatusNotFound)

	events := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=component_group&sort=occurred_at", nil), http.StatusOK).json(t)
	var actions []string
	for _, e := range list(events, "data") {
		actions = append(actions, str(e.(map[string]any), "action"))
	}
	if len(actions) != 2 || actions[0] != "created" || actions[1] != "deleted" {
		t.Fatalf("group audit actions = %v", actions)
	}
}

func TestComponentGroupValidation(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "", "display_order": -1}, invalidRequest()), http.StatusUnprocessableEntity)
	if len(list(obj(r.json(t), "error"), "details")) != 2 {
		t.Fatalf("expected name and display_order errors: %s", r.body)
	}
	g := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Infra"}), http.StatusCreated).json(t)
	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/component-groups/"+str(g, "id"), map[string]any{}, invalidRequest()), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "empty_update" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	upd := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/component-groups/"+str(g, "id"), map[string]any{"name": "Infraestrutura"}), http.StatusOK).json(t)
	if str(upd, "name") != "Infraestrutura" {
		t.Fatalf("rename: %v", upd)
	}
}
