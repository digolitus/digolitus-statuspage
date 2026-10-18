package app

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
)

func TestEveryAdminRouteHasPolicy(t *testing.T) {
	application := New(nil, Options{AdminAPIEnabled: true})
	var missing []string
	seen := map[string]bool{}
	for _, r := range application.Router.Routes() {
		key := r.Method + " " + r.Path
		if strings.HasPrefix(r.Path, "/api/v1/admin/") {
			seen[key] = true
			if _, ok := adminPolicy[key]; !ok {
				missing = append(missing, key)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("admin routes without an authorization policy (they would always be denied):\n  %s", strings.Join(missing, "\n  "))
	}
	for key, perms := range adminPolicy {
		if !seen[key] {
			t.Errorf("policy for unknown route %s", key)
		}
		for _, p := range perms {
			if !auth.Known(p) {
				t.Errorf("%s: unknown permission %s", key, p)
			}
		}
	}
}

func TestInitialSetup(t *testing.T) {
	h := newBareHarness(t)
	anon := h.anonymous()
	st := mustStatus(t, anon.call(http.MethodGet, "/api/v1/auth/setup", nil), http.StatusOK).json(t)
	if st["setup_required"] != true {
		t.Fatalf("setup should be required: %v", st)
	}
	// Admin routes are closed even before any user exists.
	mustStatus(t, anon.call(http.MethodGet, "/api/v1/admin/components", nil), http.StatusUnauthorized)

	token, err := h.app.Auth.EnsureSetupToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"setup_token": "wrong", "name": "Ana", "email": "ana@example.com", "password": "correct horse battery staple"}
	r := mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/setup", body), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"setup_token"`) {
		t.Fatalf("wrong token: %s", r.body)
	}
	body["password"] = "short"
	body["setup_token"] = token
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/setup", body, invalidRequest()), http.StatusUnprocessableEntity)

	body["password"] = "correct horse battery staple"
	me := mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/setup", body), http.StatusCreated).json(t)
	if me["is_admin"] != true || len(list(me, "permissions")) != len(auth.Catalogue) {
		t.Fatalf("first user must be a full administrator: %v", me)
	}
	// Setup can only happen once, and no new token is issued afterwards.
	r = mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/setup", body), http.StatusConflict)
	if r.errorCode(t) != "setup_already_done" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	if again, _ := h.app.Auth.EnsureSetupToken(context.Background()); again != "" {
		t.Fatal("no setup token may be issued once a user exists")
	}
	if st := mustStatus(t, anon.call(http.MethodGet, "/api/v1/auth/setup", nil), http.StatusOK).json(t); st["setup_required"] != false {
		t.Fatalf("setup done: %v", st)
	}
}

func TestLoginLogoutAndLockout(t *testing.T) {
	h := newHarness(t)
	anon := h.anonymous()
	for _, body := range []map[string]any{
		{"email": adminEmail, "password": "wrong password!!"},
		{"email": "nobody@example.com", "password": "whatever password"},
	} {
		r := mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/login", body), http.StatusUnauthorized)
		if r.errorCode(t) != "invalid_credentials" {
			t.Fatalf("code = %s", r.errorCode(t))
		}
	}
	// E-mail is case-insensitive.
	s := h.login(strings.ToUpper(adminEmail), adminPassword)
	me := mustStatus(t, s.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t)
	if str(obj(me, "user"), "email") != adminEmail {
		t.Fatalf("me: %v", me)
	}
	mustStatus(t, s.call(http.MethodPost, "/api/v1/auth/logout", nil), http.StatusNoContent)
	mustStatus(t, s.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized)

	// Five failures in a row lock the account, even for the right password.
	for range 5 {
		mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": adminEmail, "password": "wrong password!!"}), http.StatusUnauthorized)
	}
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": adminEmail, "password": adminPassword}), http.StatusUnauthorized)
	users := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/users", nil), http.StatusOK).json(t)
	if list(users, "data")[0].(map[string]any)["locked_until"] == nil {
		t.Fatalf("account should be locked: %v", users)
	}
	h.clock.Advance(16 * time.Minute)
	h.login(adminEmail, adminPassword)
}

func TestSessionExpiresWhenIdle(t *testing.T) {
	h := newHarness(t)
	mustStatus(t, h.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK)
	h.clock.Advance(25 * time.Hour) // harness idle timeout: 24h
	mustStatus(t, h.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized)
}

func TestCrossSiteRequestsAreRejected(t *testing.T) {
	h := newHarness(t)
	body := map[string]any{"name": "X"}
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", body, withHeader("Sec-Fetch-Site", "cross-site")), http.StatusForbidden)
	if r.errorCode(t) != "cross_site_request" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", body, withHeader("Origin", "https://evil.example")), http.StatusForbidden)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", body, withHeader("Sec-Fetch-Site", "same-origin")), http.StatusCreated)
}

// TestRolesRestrictActions covers the two profiles described in the
// requirements: an incident operator and a catalogue/announcements manager.
func TestRolesRestrictActions(t *testing.T) {
	h := newHarness(t)
	comp := h.createComponent("API", nil)
	group := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Core"}), http.StatusCreated).json(t)

	a := h.userWith("Usuario A", auth.PermIncidentsManage, auth.PermMaintenancesManage, auth.PermComponentsStatus)
	b := h.userWith("Usuario B", auth.PermComponentsManage, auth.PermComponentsStatus, auth.PermAnnouncementsWrite,
		auth.PermAnnouncementsPublish, auth.PermAnnouncementsSetting, auth.PermThirdPartyManage, auth.PermTemplatesManage)

	// A: reports outages...
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Lentidão", "message": "Investigando.", "impact": "degraded_performance", "component_ids": []string{str(comp, "id")},
	}), http.StatusCreated)
	mustStatus(t, a.call(http.MethodPatch, "/api/v1/admin/components/"+str(comp, "id"), map[string]any{"status": "partial_outage"}), http.StatusOK)
	mustStatus(t, a.call(http.MethodGet, "/api/v1/admin/components", nil), http.StatusOK)
	// ...but cannot change the catalogue, announcements or users.
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Nova"}},
		{http.MethodPatch, "/api/v1/admin/components/" + str(comp, "id"), map[string]any{"name": "Renomeada"}},
		{http.MethodPatch, "/api/v1/admin/components/" + str(comp, "id"), map[string]any{"status": "operational", "is_active": false}},
		{http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "G"}},
		{http.MethodDelete, "/api/v1/admin/component-groups/" + str(group, "id"), nil},
		{http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "x", "body": "y"}},
		{http.MethodGet, "/api/v1/admin/users", nil},
		{http.MethodGet, "/api/v1/admin/audit-events", nil},
	} {
		r := mustStatus(t, a.call(c.method, c.path, c.body), http.StatusForbidden)
		if r.errorCode(t) != "forbidden" {
			t.Errorf("%s %s: code %s", c.method, c.path, r.errorCode(t))
		}
	}

	// B: full catalogue and announcements management...
	mustStatus(t, b.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Checkout"}), http.StatusCreated)
	mustStatus(t, b.call(http.MethodPatch, "/api/v1/admin/component-groups/"+str(group, "id"), map[string]any{"name": "Núcleo"}), http.StatusOK)
	ann := mustStatus(t, b.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Novidade", "body": "Texto"}), http.StatusCreated).json(t)
	mustStatus(t, b.call(http.MethodPost, "/api/v1/admin/announcements/"+str(ann, "id")+"/publish", map[string]any{}), http.StatusOK)
	// ...without user or role management, nor incidents.
	mustStatus(t, b.call(http.MethodGet, "/api/v1/admin/users", nil), http.StatusForbidden)
	mustStatus(t, b.call(http.MethodGet, "/api/v1/admin/roles", nil), http.StatusForbidden)
	mustStatus(t, b.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "x", "message": "y", "impact": "operational"}), http.StatusForbidden)

	// Changes are attributed to the person who made them.
	ev := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=announcement&action=published", nil), http.StatusOK).json(t)
	actor := obj(list(ev, "data")[0].(map[string]any), "actor")
	if !strings.Contains(str(actor, "name"), "usuario.b@example.com") {
		t.Fatalf("audit actor: %v", actor)
	}

	// Role changes apply immediately to existing sessions.
	roles := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/roles", nil), http.StatusOK).json(t)
	for _, r := range list(roles, "data") {
		if str(r.(map[string]any), "name") == "Papel Usuario A" {
			mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/roles/"+str(r.(map[string]any), "id"),
				map[string]any{"permissions": []string{auth.PermComponentsManage}}), http.StatusOK)
		}
	}
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Agora pode"}), http.StatusCreated)
	mustStatus(t, a.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "x", "message": "y", "impact": "operational"}), http.StatusForbidden)
}

func TestNoPrivilegeEscalation(t *testing.T) {
	h := newHarness(t)
	m := h.userWith("Gestor de Acesso", auth.PermUsersManage, auth.PermRolesManage)
	roles := mustStatus(t, m.call(http.MethodGet, "/api/v1/admin/roles", nil), http.StatusOK).json(t)
	var adminRole, operatorRole string
	for _, r := range list(roles, "data") {
		rm := r.(map[string]any)
		switch {
		case rm["is_system"] == true:
			adminRole = str(rm, "id")
		case str(rm, "name") == "Operador de incidentes":
			operatorRole = str(rm, "id")
		}
	}
	// Cannot create roles with permissions they lack, nor grant the
	// administrator role or roles with foreign permissions.
	mustStatus(t, m.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{"name": "Tudo", "permissions": []string{auth.PermIncidentsManage}}), http.StatusForbidden)
	for _, role := range []string{adminRole, operatorRole} {
		mustStatus(t, m.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
			"email": "x" + role[:4] + "@example.com", "name": "X", "password": "a long enough password", "role_ids": []string{role},
		}), http.StatusForbidden)
	}
	// Cannot take over the administrator account.
	users := mustStatus(t, m.call(http.MethodGet, "/api/v1/admin/users", nil), http.StatusOK).json(t)
	var adminID string
	for _, u := range list(users, "data") {
		if str(u.(map[string]any), "email") == adminEmail {
			adminID = str(u.(map[string]any), "id")
		}
	}
	mustStatus(t, m.call(http.MethodPost, "/api/v1/admin/users/"+adminID+"/password", map[string]any{"password": "attacker password!"}), http.StatusForbidden)
	mustStatus(t, m.call(http.MethodPatch, "/api/v1/admin/users/"+adminID, map[string]any{"is_active": false}), http.StatusForbidden)
	// Within their own permissions they can work normally.
	sub := mustStatus(t, m.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{"name": "Só usuários", "permissions": []string{auth.PermUsersManage}}), http.StatusCreated).json(t)
	mustStatus(t, m.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "sub@example.com", "name": "Sub", "password": "a long enough password", "role_ids": []string{str(sub, "id")},
	}), http.StatusCreated)
}

func TestUserAdministration(t *testing.T) {
	h := newHarness(t)
	me := mustStatus(t, h.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t)
	adminID := str(obj(me, "user"), "id")

	// Self-protection and last-admin protection.
	r := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+adminID, map[string]any{"is_active": false}), http.StatusConflict)
	if r.errorCode(t) != "cannot_deactivate_self" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+adminID, map[string]any{"role_ids": []string{}}), http.StatusConflict)
	if r.errorCode(t) != "last_admin" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	roles := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/roles", nil), http.StatusOK).json(t)
	system := list(roles, "data")[0].(map[string]any)
	if system["is_system"] != true || len(list(system, "permissions")) != len(auth.Catalogue) {
		t.Fatalf("system role first, with every permission: %v", system)
	}
	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/roles/"+str(system, "id"), map[string]any{"permissions": []string{}}), http.StatusConflict)
	if r.errorCode(t) != "system_role_immutable" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/roles/"+str(system, "id"), nil), http.StatusConflict)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{"name": "x", "permissions": []string{"everything"}}, invalidRequest()), http.StatusUnprocessableEntity)

	// Create, duplicate e-mail, deactivate (ends sessions), reset password.
	u := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "Joana@Example.com", "name": "Joana", "password": "a long enough password",
	}), http.StatusCreated).json(t)
	if str(u, "email") != "joana@example.com" || len(list(u, "roles")) != 0 {
		t.Fatalf("user: %v", u)
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "joana@example.com", "name": "J", "password": "a long enough password"}), http.StatusConflict)
	if r.errorCode(t) != "user_email_conflict" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	j := h.login("joana@example.com", "a long enough password")
	// The administrator chose this password: nothing else works until Joana
	// picks her own.
	me2 := mustStatus(t, j.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t)
	if obj(me2, "user")["must_change_password"] != true {
		t.Fatalf("must_change_password: %v", me2)
	}
	r = mustStatus(t, j.call(http.MethodGet, "/api/v1/admin/incidents", nil), http.StatusForbidden)
	if r.errorCode(t) != "password_change_required" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	j.choosePassword("a long enough password", "joana own password 1")
	// No role: may read operational data, nothing else.
	mustStatus(t, j.call(http.MethodGet, "/api/v1/admin/incidents", nil), http.StatusOK)
	mustStatus(t, j.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "x"}), http.StatusForbidden)

	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+str(u, "id"), map[string]any{"is_active": false}), http.StatusOK)
	mustStatus(t, j.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized)
	mustStatus(t, h.anonymous().call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "joana@example.com", "password": "joana own password 1"}), http.StatusUnauthorized)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+str(u, "id"), map[string]any{"is_active": true}), http.StatusOK)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users/"+str(u, "id")+"/password", map[string]any{"password": "brand new password 2"}), http.StatusNoContent)
	j = h.login("joana@example.com", "brand new password 2")
	mustStatus(t, j.call(http.MethodGet, "/api/v1/admin/incidents", nil), http.StatusForbidden)

	// Own password change: wrong current password, then success; other
	// sessions end, the current one stays.
	other := h.login("joana@example.com", "brand new password 2")
	r = mustStatus(t, j.call(http.MethodPost, "/api/v1/auth/password", map[string]any{"current_password": "nope", "new_password": "another good password"}), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), "current_password") {
		t.Fatalf("expected current_password error: %s", r.body)
	}
	mustStatus(t, j.call(http.MethodPost, "/api/v1/auth/password", map[string]any{"current_password": "brand new password 2", "new_password": "another good password"}), http.StatusNoContent)
	mustStatus(t, j.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK)
	mustStatus(t, other.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized)

	// A role in use cannot be deleted.
	role := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{"name": "Leitura de auditoria", "permissions": []string{auth.PermAuditRead}}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+str(u, "id"), map[string]any{"role_ids": []string{str(role, "id")}}), http.StatusOK)
	r = mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/roles/"+str(role, "id"), nil), http.StatusConflict)
	if r.errorCode(t) != "role_in_use" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	mustStatus(t, j.call(http.MethodGet, "/api/v1/admin/audit-events", nil), http.StatusOK)

	ev := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=user&resource_id="+str(u, "id")+"&sort=occurred_at", nil), http.StatusOK).json(t)
	var actions []string
	for _, e := range list(ev, "data") {
		actions = append(actions, str(e.(map[string]any), "action"))
	}
	if strings.Join(actions, ",") != "created,password_changed,deactivated,reactivated,password_reset,password_changed,roles_changed" {
		t.Fatalf("audit actions = %v", actions)
	}
	if mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/permissions", nil), http.StatusOK).json(t)["data"] == nil {
		t.Fatal("permissions catalogue")
	}
}

// TestRecoverAccess covers the server-side recovery command (api
// reset-password): the only administrator forgot the password and is locked.
func TestRecoverAccess(t *testing.T) {
	h := newHarness(t)
	for range 5 {
		mustStatus(t, h.anonymous().call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": adminEmail, "password": "wrong password!!"}), http.StatusUnauthorized)
	}
	temp, err := h.app.Auth.RecoverAccess(context.Background(), " ADMIN@example.com ")
	if err != nil || len(temp) < 20 {
		t.Fatalf("recover: %q %v", temp, err)
	}
	if _, err := h.app.Auth.RecoverAccess(context.Background(), "nobody@example.com"); err == nil {
		t.Fatal("unknown e-mail must fail")
	}
	mustStatus(t, h.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized)
	a := h.login(adminEmail, temp)
	mustStatus(t, a.call(http.MethodGet, "/api/v1/admin/components", nil), http.StatusForbidden)
	a.choosePassword(temp, "a fresh admin password")
	ev := mustStatus(t, a.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=user&action=access_recovered", nil), http.StatusOK).json(t)
	if total(ev) != 1 || str(list(ev, "data")[0].(map[string]any), "source") != "cli" {
		t.Fatalf("audit: %v", ev)
	}
}
