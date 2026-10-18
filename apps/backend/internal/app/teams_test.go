package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
)

func TestTeamFields(t *testing.T) {
	h := newHarness(t)
	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{
		"name": "Squad Pagamentos", "kind": "Squad", "description": "Cobrança",
		"documentation_url": "https://docs.example.com/pagamentos", "support_url": "https://help.example.com/pagamentos",
	}), http.StatusCreated).json(t)
	if str(team, "kind") != "Squad" || str(team, "support_url") != "https://help.example.com/pagamentos" {
		t.Fatalf("team: %v", team)
	}
	r := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"support_url": "javascript:alert(1)"}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"support_url"`) {
		t.Fatalf("invalid url: %s", r.body)
	}
	upd := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"documentation_url": nil, "kind": "RT"}), http.StatusOK).json(t)
	if upd["documentation_url"] != nil || str(upd, "kind") != "RT" {
		t.Fatalf("patch: %v", upd)
	}

	// Ownership: group team inherited by its components unless overridden.
	group := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Pagamentos", "team_id": str(team, "id")}), http.StatusCreated).json(t)
	inherited := h.createComponent("Checkout", map[string]any{"group_id": str(group, "id")})
	if inherited["team_id"] != nil || str(inherited, "owner_team_id") != str(team, "id") {
		t.Fatalf("inherited owner: %v", inherited)
	}
	list := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components?team_id="+str(team, "id"), nil), http.StatusOK).json(t)
	if total(list) != 1 {
		t.Fatalf("team filter: %v", list)
	}
	// A team in use is deactivated, not deleted.
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/teams/"+str(team, "id"), nil), http.StatusConflict)
	if r.errorCode(t) != "team_in_use" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/teams/"+str(team, "id"), nil), http.StatusOK).json(t)
	if got["component_count"].(float64) != 0 || got["group_count"].(float64) != 1 {
		t.Fatalf("counts: %v", got)
	}
}

// TestTeamScopedRoles: each squad operates only the components its teams
// own (own team or group's team).
func TestTeamScopedRoles(t *testing.T) {
	h := newHarness(t)
	t1 := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Squad A", "kind": "Squad"}), http.StatusCreated).json(t)
	t2 := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Squad B", "kind": "Squad"}), http.StatusCreated).json(t)
	g1 := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Serviços A", "team_id": str(t1, "id")}), http.StatusCreated).json(t)
	own := h.createComponent("API A", map[string]any{"group_id": str(g1, "id")})
	other := h.createComponent("API B", map[string]any{"team_id": str(t2, "id")})
	orphan := h.createComponent("Sem dono", nil)

	// Team roles accept only scopable permissions.
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{
		"name": "Time inválido", "scope": "team", "permissions": []string{auth.PermUsersManage},
	}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"permissions"`) {
		t.Fatalf("scope validation: %s", r.body)
	}
	role := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{
		"name": "Squad", "scope": "team",
		"permissions": []string{auth.PermComponentsStatus, auth.PermComponentsManage, auth.PermIncidentsManage, auth.PermAnnouncementsWrite},
	}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "squad.a@example.com", "name": "Squad A", "password": "a long enough password",
		"role_ids": []string{str(role, "id")}, "team_ids": []string{str(t1, "id")},
	}), http.StatusCreated)
	a := h.login("squad.a@example.com", "a long enough password")
	a.choosePassword("a long enough password", "squad a own password")

	me := mustStatus(t, a.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t)
	if len(list(me, "team_permissions")) != 4 || len(list(obj(me, "user"), "teams")) != 1 {
		t.Fatalf("me: %v", me)
	}

	scoped := func(r response) {
		t.Helper()
		if r.errorCode(t) != "outside_team_scope" {
			t.Fatalf("expected outside_team_scope, got %s: %s", r.errorCode(t), r.body)
		}
	}
	// Own component (inherited from the group): allowed.
	mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(own, "id"), map[string]any{"status": "partial_outage"}), http.StatusOK)
	mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(own, "id"), map[string]any{"description": "Gateway"}), http.StatusOK)
	inc := mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Lentidão", "message": "Investigando.", "impact": "degraded_performance", "component_ids": []string{str(own, "id")},
	}), http.StatusCreated).json(t)
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents/"+str(inc, "id")+"/resolve", map[string]any{"message": "Resolvido."}), http.StatusOK)
	created := mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Worker A", "team_id": str(t1, "id")}), http.StatusCreated).json(t)

	// Other team's or unowned components: refused.
	scoped(mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(other, "id"), map[string]any{"status": "major_outage"}), http.StatusForbidden))
	scoped(mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(orphan, "id"), map[string]any{"status": "major_outage"}), http.StatusForbidden))
	scoped(mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "x", "message": "y", "impact": "operational", "component_ids": []string{str(own, "id"), str(other, "id")},
	}), http.StatusForbidden))
	scoped(mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "x", "message": "y", "impact": "operational"}), http.StatusForbidden))
	// Cannot give its component away nor create components for nobody.
	scoped(mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(created, "id"), map[string]any{"team_id": str(t2, "id")}), http.StatusForbidden))
	scoped(mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Órfão"}), http.StatusForbidden))
	scoped(mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Grupo B", "team_id": str(t2, "id")}), http.StatusForbidden))
	// Announcements: only for its team.
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Novidade A", "body": "Texto", "team_id": str(t1, "id")}), http.StatusCreated)
	scoped(mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Geral", "body": "Texto"}), http.StatusForbidden))
	// Non-scopable areas stay closed.
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Novo time"}), http.StatusForbidden)

	// The state of the other team's component is unchanged.
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/components/"+str(other, "id"), nil), http.StatusOK).json(t)
	if str(got, "status") != "operational" {
		t.Fatalf("other component changed: %v", got)
	}
	// Leaving the team removes the access immediately.
	users := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/users", nil), http.StatusOK).json(t)
	for _, u := range list(users, "data") {
		if str(u.(map[string]any), "email") == "squad.a@example.com" {
			mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+str(u.(map[string]any), "id"), map[string]any{"team_ids": []string{}}), http.StatusOK)
		}
	}
	scoped(mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(own, "id"), map[string]any{"status": "operational"}), http.StatusForbidden))
}
