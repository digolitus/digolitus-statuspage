package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func presenceSnapshot(t *testing.T, h *harness) map[string]any {
	t.Helper()
	return mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/presence?timezone=America/Sao_Paulo", nil), http.StatusOK).json(t)
}

func areaCounts(m map[string]any) map[string]float64 {
	out := map[string]float64{}
	for _, a := range list(m["public"].(map[string]any), "areas") {
		e := a.(map[string]any)
		out[e["area"].(string)] = e["count"].(float64)
	}
	return out
}

func TestPresenceCountsPublicVisitorsAndAdminUsers(t *testing.T) {
	h := newHarness(t)
	anon := h.anonymous()
	beat := func(tab, area string) {
		mustStatus(t, anon.call(http.MethodPost, "/api/v1/presence", map[string]any{"tab_id": tab, "area": area}), http.StatusNoContent)
	}
	beat("tab-aaaaaaaaaaaaaaaa", "status")
	beat("tab-bbbbbbbbbbbbbbbb", "status")
	beat("tab-cccccccccccccccc", "incident")
	beat("tab-cccccccccccccccc", "incident") // same tab again: still one visitor

	marina := h.userWith("Marina Alves")
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/presence", map[string]any{"tab_id": "adm-aaaaaaaaaaaaaaaa", "area": "Visão geral"}), http.StatusNoContent)
	mustStatus(t, marina.call(http.MethodPost, "/api/v1/admin/presence", map[string]any{
		"tab_id": "adm-bbbbbbbbbbbbbbbb", "area": "  Incidentes \n› Lentidão  ", "hidden": true}), http.StatusNoContent)
	// A tab id already used by another user does not move to this one.
	mustStatus(t, marina.call(http.MethodPost, "/api/v1/admin/presence", map[string]any{"tab_id": "adm-aaaaaaaaaaaaaaaa", "area": "Usurpado"}), http.StatusNoContent)

	// Reading the panel requires presence.read.
	mustStatus(t, marina.call(http.MethodGet, "/api/v1/admin/presence", nil), http.StatusForbidden)
	mustStatus(t, anon.call(http.MethodGet, "/api/v1/admin/presence", nil), http.StatusUnauthorized)

	s := presenceSnapshot(t, h)
	pub := s["public"].(map[string]any)
	if pub["total"].(float64) != 3 {
		t.Fatalf("public total = %v, want 3", pub["total"])
	}
	if c := areaCounts(s); c["status"] != 2 || c["incident"] != 1 || c["announcements"] != 0 || len(c) != 6 {
		t.Fatalf("areas = %v", c)
	}
	users := list(s["admin"].(map[string]any), "users")
	if len(users) != 2 {
		t.Fatalf("admin users = %v", users)
	}
	first, second := users[0].(map[string]any), users[1].(map[string]any)
	if first["name"] != "Admin" || first["area"] != "Visão geral" || first["hidden"] != false {
		t.Fatalf("first user = %v (visible users come first, tab not taken over)", first)
	}
	if second["name"] != "Marina Alves" || second["area"] != "Incidentes › Lentidão" || second["hidden"] != true {
		t.Fatalf("second user = %v", second)
	}

	// The sampler records the minute; after the window everyone is gone but
	// the history and the peak of the day remain.
	if err := h.app.Presence.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(3 * time.Minute)
	if err := h.app.Presence.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	s = presenceSnapshot(t, h)
	if s["public"].(map[string]any)["total"].(float64) != 0 || s["admin"].(map[string]any)["total"].(float64) != 0 {
		t.Fatalf("expired tabs still counted: %v", s)
	}
	hist := s["history"].([]any)
	if len(hist) != 2 || hist[0].(map[string]any)["public"].(float64) != 3 || hist[0].(map[string]any)["admin"].(float64) != 2 {
		t.Fatalf("history = %v", hist)
	}
	if p, ok := s["peak_today"].(map[string]any); !ok || p["count"].(float64) != 3 {
		t.Fatalf("peak_today = %v", s["peak_today"])
	}

	// Rows are pruned ten minutes after the last heartbeat.
	h.clock.Advance(10 * time.Minute)
	if err := h.app.Presence.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.pool.QueryRow(context.Background(), "SELECT count(*) FROM presence_heartbeats").Scan(&n); err != nil || n != 0 {
		t.Fatalf("heartbeats left = %d (%v)", n, err)
	}
}

func TestPresenceValidation(t *testing.T) {
	h := newHarness(t)
	anon := h.anonymous()
	for _, body := range []map[string]any{
		{"tab_id": "short", "area": "status"},
		{"tab_id": "tab-aaaaaaaaaaaaaaaa", "area": "admin"},
	} {
		mustStatus(t, anon.call(http.MethodPost, "/api/v1/presence", body, invalidRequest()), http.StatusUnprocessableEntity)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/presence", map[string]any{"tab_id": "adm-aaaaaaaaaaaaaaaa", "area": "   "}, invalidRequest()), http.StatusUnprocessableEntity)
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/presence?timezone=Local", nil), http.StatusUnprocessableEntity)
}

func TestPresenceDisabled(t *testing.T) {
	h := newHarnessWith(t, func(o *Options) { o.PresenceEnabled = false })
	if h.app.Presence != nil {
		t.Fatal("presence service built while disabled")
	}
	for _, r := range h.app.Router.Routes() {
		if strings.Contains(r.Path, "presence") {
			t.Fatalf("route %s %s registered while disabled", r.Method, r.Path)
		}
	}
}
