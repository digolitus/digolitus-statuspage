package app

import (
	"net/http"
	"testing"
)

// TestComponentSubgroups covers the optional Group -> Subgroup -> Component
// hierarchy: admin rules, derived public status and inherited ownership.
func TestComponentSubgroups(t *testing.T) {
	h := newHarness(t)
	groups := "/api/v1/admin/component-groups"
	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Squad Cartões"}), http.StatusCreated).json(t)
	pay := mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "Pagamentos"}), http.StatusCreated).json(t)
	plat := mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "Plataforma"}), http.StatusCreated).json(t)
	cards := mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "Cartões", "parent_id": str(pay, "id"), "team_id": str(team, "id")}), http.StatusCreated).json(t)
	if str(cards, "parent_id") != str(pay, "id") {
		t.Fatalf("subgroup parent: %v", cards)
	}
	// Names are unique per parent: the same name works under another group.
	mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "API", "parent_id": str(pay, "id")}), http.StatusCreated)
	mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "API", "parent_id": str(plat, "id")}), http.StatusCreated)
	r := mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "api", "parent_id": str(pay, "id")}), http.StatusConflict)
	if r.errorCode(t) != "component_group_name_conflict" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	// Only two levels.
	mustStatus(t, h.call(http.MethodPost, groups, map[string]any{"name": "Débito", "parent_id": str(cards, "id")}, invalidRequest()), http.StatusUnprocessableEntity)

	// A component in a subgroup gets the subgroup's group and team.
	api := h.createComponent("API de Cartões", map[string]any{"subgroup_id": str(cards, "id")})
	if str(api, "group_id") != str(pay, "id") || str(api, "subgroup_id") != str(cards, "id") || str(api, "owner_team_id") != str(team, "id") {
		t.Fatalf("component in subgroup: %v", api)
	}
	checkout := h.createComponent("Checkout", map[string]any{"group_id": str(pay, "id")})
	if checkout["subgroup_id"] != nil {
		t.Fatalf("subgroup is optional: %v", checkout)
	}
	// A subgroup from another group, or a subgroup as group_id, is rejected.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components",
		map[string]any{"name": "X", "group_id": str(plat, "id"), "subgroup_id": str(cards, "id")}), http.StatusUnprocessableEntity)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components",
		map[string]any{"name": "Y", "group_id": str(cards, "id")}), http.StatusUnprocessableEntity)

	// Filters: group includes its subgroups; subgroup is exact.
	if n := total(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?group_id="+str(pay, "id"), nil), http.StatusOK).json(t)); n != 2 {
		t.Fatalf("group filter = %d", n)
	}
	if n := total(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?subgroup_id="+str(cards, "id"), nil), http.StatusOK).json(t)); n != 1 {
		t.Fatalf("subgroup filter = %d", n)
	}
	g := mustStatus(t, h.call(http.MethodGet, groups+"/"+str(pay, "id"), nil), http.StatusOK).json(t)
	if g["component_count"].(float64) != 2 || g["subgroup_count"].(float64) != 2 {
		t.Fatalf("group counts: %v", g)
	}

	// Public summary: subgroup and group statuses are derived.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Lentidão", "message": "Investigando.", "impact": "degraded_performance", "component_ids": []string{str(api, "id")},
	}), http.StatusCreated)
	sum := mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	byID := map[string]map[string]any{}
	for _, x := range list(sum, "groups") {
		byID[str(x.(map[string]any), "id")] = x.(map[string]any)
	}
	if len(byID) != 2 || str(byID[str(pay, "id")], "status") != "degraded_performance" || byID[str(pay, "id")]["component_count"].(float64) != 2 ||
		str(byID[str(cards, "id")], "status") != "degraded_performance" || str(byID[str(cards, "id")], "parent_id") != str(pay, "id") {
		t.Fatalf("public groups: %v", list(sum, "groups"))
	}

	// Changing group without subgroup_id leaves the subgroup; subgroup null keeps the group.
	moved := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(api, "id"), map[string]any{"group_id": str(plat, "id")}), http.StatusOK).json(t)
	if moved["subgroup_id"] != nil || str(moved, "group_id") != str(plat, "id") {
		t.Fatalf("group change: %v", moved)
	}
	back := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(api, "id"), map[string]any{"subgroup_id": str(cards, "id")}), http.StatusOK).json(t)
	if str(back, "group_id") != str(pay, "id") {
		t.Fatalf("subgroup moves the component to its group: %v", back)
	}

	// Level changes need an empty group; moving a subgroup carries its components.
	r = mustStatus(t, h.call(http.MethodPatch, groups+"/"+str(cards, "id"), map[string]any{"parent_id": nil}), http.StatusConflict)
	if r.errorCode(t) != "component_group_level_locked" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	mustStatus(t, h.call(http.MethodPatch, groups+"/"+str(pay, "id"), map[string]any{"parent_id": str(plat, "id")}), http.StatusConflict)
	mustStatus(t, h.call(http.MethodPatch, groups+"/"+str(cards, "id"), map[string]any{"parent_id": str(plat, "id")}), http.StatusOK)
	c := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components/"+str(api, "id"), nil), http.StatusOK).json(t)
	if str(c, "group_id") != str(plat, "id") || str(c, "subgroup_id") != str(cards, "id") {
		t.Fatalf("cascade on subgroup move: %v", c)
	}

	// Deletion: subgroups block their group.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(checkout, "id"), map[string]any{"group_id": nil}), http.StatusOK)
	r = mustStatus(t, h.call(http.MethodDelete, groups+"/"+str(pay, "id"), nil), http.StatusConflict)
	if r.errorCode(t) != "component_group_has_subgroups" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
}
