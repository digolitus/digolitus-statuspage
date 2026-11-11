package app

import (
	"net/http"
	"testing"
)

func num(m map[string]any, key string) int { return int(m[key].(float64)) }

// TestDepartments covers the optional hierarchy above teams (ADR 0017):
// up to four levels, teams at any level, people directly or via teams.
func TestDepartments(t *testing.T) {
	h := newHarness(t)
	const path = "/api/v1/admin/departments"
	create := func(name string, parent any) map[string]any {
		return mustStatus(t, h.call(http.MethodPost, path, map[string]any{"name": name, "parent_id": parent}), http.StatusCreated).json(t)
	}

	levels := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/department-levels", map[string]any{"names": []string{" Diretoria ", "Gerência", "Coordenação"}}), http.StatusOK).json(t)
	if names := list(levels, "names"); len(names) != 4 || names[0] != "Diretoria" || names[3] != "" {
		t.Fatalf("levels: %v", levels)
	}

	tech := create("Tecnologia", nil)
	plat := create("Plataforma", str(tech, "id"))
	infra := create("Infraestrutura", str(plat, "id"))
	sre := create("SRE", str(infra, "id"))
	if num(sre, "level") != 4 || len(list(sre, "path")) != 4 {
		t.Fatalf("level 4: %v", sre)
	}
	// A fifth level is refused, and so is a move that would create one.
	r := mustStatus(t, h.call(http.MethodPost, path, map[string]any{"name": "Fundo", "parent_id": str(sre, "id")}), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("depth: %s", r.body)
	}
	other := create("Negócios", nil)
	mustStatus(t, h.call(http.MethodPatch, path+"/"+str(plat, "id"), map[string]any{"parent_id": str(other, "id")}), http.StatusOK)
	other2 := create("Pagamentos", str(other, "id"))
	mustStatus(t, h.call(http.MethodPatch, path+"/"+str(plat, "id"), map[string]any{"parent_id": str(other2, "id")}), http.StatusUnprocessableEntity)
	// Cycles are refused.
	mustStatus(t, h.call(http.MethodPatch, path+"/"+str(plat, "id"), map[string]any{"parent_id": str(infra, "id")}), http.StatusUnprocessableEntity)
	// Sibling names are unique, case-insensitively.
	r = mustStatus(t, h.call(http.MethodPost, path, map[string]any{"name": "pagamentos", "parent_id": str(other, "id")}), http.StatusConflict)
	if r.errorCode(t) != "department_name_conflict" {
		t.Fatalf("name: %s", r.body)
	}

	// Teams at any level; people via teams and directly, counted once.
	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Squad SRE", "department_id": str(sre, "id")}), http.StatusCreated).json(t)
	if str(team, "department_id") != str(sre, "id") {
		t.Fatalf("team department: %v", team)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"department_id": "00000000-0000-0000-0000-000000000001"}), http.StatusUnprocessableEntity)
	user := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "ana@example.com", "name": "Ana", "password": "a long enough password",
		"team_ids": []string{str(team, "id")}, "job_title": "Gerente", "department_ids": []string{str(plat, "id")},
	}), http.StatusCreated).json(t)
	if str(user, "job_title") != "Gerente" || len(list(user, "departments")) != 1 {
		t.Fatalf("user: %v", user)
	}
	d := mustStatus(t, h.call(http.MethodGet, path+"/"+str(plat, "id"), nil), http.StatusOK).json(t)
	if num(d, "total_team_count") != 1 || num(d, "total_people_count") != 1 || len(list(d, "members")) != 1 || num(d, "child_count") != 1 {
		t.Fatalf("counts: %v", d)
	}

	// The tree comes in order: parents before children.
	all := list(mustStatus(t, h.call(http.MethodGet, path, nil), http.StatusOK).json(t), "data")
	if len(all) != 6 || all[0].(map[string]any)["name"] != "Negócios" {
		t.Fatalf("list: %v", all)
	}

	// Only empty departments are deleted; direct people are just unlinked.
	r = mustStatus(t, h.call(http.MethodDelete, path+"/"+str(sre, "id"), nil), http.StatusConflict)
	if r.errorCode(t) != "department_in_use" {
		t.Fatalf("delete: %s", r.body)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/teams/"+str(team, "id"), map[string]any{"department_id": nil}), http.StatusOK)
	mustStatus(t, h.call(http.MethodDelete, path+"/"+str(sre, "id"), nil), http.StatusNoContent)
	mustStatus(t, h.call(http.MethodPatch, path+"/"+str(plat, "id"), map[string]any{"member_ids": []string{}}), http.StatusOK)
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/users/"+str(user, "id"), nil), http.StatusOK).json(t)
	if len(list(got, "departments")) != 0 {
		t.Fatalf("unlinked: %v", got)
	}
	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=department&resource_id="+str(plat, "id"), nil), http.StatusOK).json(t)
	if total(audit) < 2 {
		t.Fatalf("audit: %v", audit)
	}
}
