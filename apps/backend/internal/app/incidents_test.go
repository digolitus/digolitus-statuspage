package app

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestIncidentLifecycle(t *testing.T) {
	h := newHarness(t)
	api := h.createComponent("API", nil)
	web := h.createComponent("Web", nil)

	inc := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Lentidão", "message": "Investigando lentidão.", "impact": "major",
		"component_ids": []string{str(api, "id")}, "internal_notes": "ticket OPS-1",
	}), http.StatusCreated)
	if inc.header.Get("Location") == "" {
		t.Error("missing Location header")
	}
	created := inc.json(t)
	id := str(created, "id")
	if str(created, "status") != "investigating" || len(list(created, "updates")) != 1 {
		t.Fatalf("unexpected incident: %v", created)
	}

	h.clock.Advance(10 * time.Minute)
	upd := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/updates", map[string]any{
		"status": "identified", "message": "Causa identificada.",
		"component_ids": []string{str(api, "id"), str(web, "id")},
	}), http.StatusCreated).json(t)
	if str(upd, "status") != "identified" || len(list(upd, "components")) != 2 {
		t.Fatalf("update not applied: %v", upd)
	}
	updates := list(upd, "updates")
	if len(updates) != 2 || str(updates[0].(map[string]any), "message") != "Causa identificada." ||
		str(updates[1].(map[string]any), "message") != "Investigando lentidão." {
		t.Fatalf("timeline must keep earlier messages, newest first: %v", updates)
	}
	if updates[0].(map[string]any)["components"] == nil || updates[1].(map[string]any)["components"] == nil {
		t.Fatalf("component snapshots expected on both updates: %v", updates)
	}

	// Going back to investigating is allowed while open.
	h.clock.Advance(time.Minute)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/updates",
		map[string]any{"status": "investigating", "message": "A correção não resolveu."}), http.StatusCreated)

	active := mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents?state=active", nil), http.StatusOK).json(t)
	if total(active) != 1 {
		t.Fatalf("expected one active incident: %v", active)
	}

	h.clock.Advance(time.Hour)
	res := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/resolve", map[string]any{"message": "Resolvido."}), http.StatusOK).json(t)
	if str(res, "status") != "resolved" || str(res, "resolved_at") != epoch.Add(71*time.Minute).Format(time.RFC3339) {
		t.Fatalf("resolution not recorded: %v", res)
	}

	// Terminal state: further updates and a second resolve are conflicts.
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/resolve", map[string]any{"message": "De novo."}), http.StatusConflict)
	if r.errorCode(t) != "incident_already_resolved" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/updates", map[string]any{"status": "monitoring", "message": "x"}), http.StatusConflict)
	if r.errorCode(t) != "incident_resolved" {
		t.Fatalf("code = %s", r.errorCode(t))
	}

	active = mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents?state=active", nil), http.StatusOK).json(t)
	resolved := mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents?state=resolved", nil), http.StatusOK).json(t)
	if total(active) != 0 || total(resolved) != 1 {
		t.Fatalf("resolved incident must leave active list and stay in history: active=%v resolved=%v", active, resolved)
	}

	timeline := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incidents/"+id+"/updates?limit=2", nil), http.StatusOK).json(t)
	if total(timeline) != 4 || len(list(timeline, "data")) != 2 {
		t.Fatalf("paginated timeline: %v", timeline)
	}

	// Public view hides internal notes (also enforced by the contract) and
	// still exposes the full timeline.
	pub := mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents/"+id, nil), http.StatusOK).json(t)
	if _, ok := pub["internal_notes"]; ok || len(list(pub, "updates")) != 4 {
		t.Fatalf("public incident: %v", pub)
	}
}

func TestIncidentValidation(t *testing.T) {
	h := newHarness(t)
	c := h.createComponent("API", nil)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+str(c, "id"), map[string]any{"is_active": false}), http.StatusOK)

	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"resolved initial status", map[string]any{"title": "x", "message": "x", "impact": "minor", "status": "resolved"}, "status"},
		{"unknown impact", map[string]any{"title": "x", "message": "x", "impact": "huge"}, "impact"},
		{"future start", map[string]any{"title": "x", "message": "x", "impact": "minor", "started_at": epoch.Add(time.Hour).Format(time.RFC3339)}, "started_at"},
		{"start without offset", map[string]any{"title": "x", "message": "x", "impact": "minor", "started_at": "2026-03-01T10:00"}, "started_at"},
		{"missing message", map[string]any{"title": "x", "impact": "minor"}, "message"},
		{"unknown component", map[string]any{"title": "x", "message": "x", "impact": "minor", "component_ids": []string{"7d0e6f1c-9d1f-4a42-8f34-0a1b2c3d4e5f"}}, "component_ids"},
		{"inactive component", map[string]any{"title": "x", "message": "x", "impact": "minor", "component_ids": []string{str(c, "id")}}, "component_ids"},
		{"unknown template", map[string]any{"title": "x", "message": "x", "impact": "minor", "template_id": "7d0e6f1c-9d1f-4a42-8f34-0a1b2c3d4e5f"}, "template_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mustStatus(t, h.with(t).call(http.MethodPost, "/api/v1/admin/incidents", tc.body, invalidRequest()), http.StatusUnprocessableEntity)
			found := false
			for _, d := range list(obj(r.json(t), "error"), "details") {
				if str(d.(map[string]any), "field") == tc.field {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected error on %s: %s", tc.field, r.body)
			}
		})
	}

	// Failed creations must not leave partial rows behind.
	var n int
	if err := h.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM incidents) + (SELECT count(*) FROM incident_updates)").Scan(&n); err != nil || n != 0 {
		t.Fatalf("expected no persisted rows, got %d (%v)", n, err)
	}
}

func TestIncidentPatchDoesNotPublish(t *testing.T) {
	h := newHarness(t)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents",
		map[string]any{"title": "Erro", "message": "Investigando.", "impact": "minor"}), http.StatusCreated).json(t), "id")
	p := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id,
		map[string]any{"title": "Erro em pagamentos", "impact": "critical"}), http.StatusOK).json(t)
	if str(p, "title") != "Erro em pagamentos" || str(p, "impact") != "critical" || len(list(p, "updates")) != 1 {
		t.Fatalf("patch: %v", p)
	}
	r := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/incidents/"+id, map[string]any{"status": "resolved"}, invalidRequest()), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("status must not be patchable: %s", r.body)
	}
}

// TestConcurrentResolve checks row locking: exactly one of several
// simultaneous resolutions succeeds and a single resolution is recorded.
func TestConcurrentResolve(t *testing.T) {
	h := newHarness(t)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents",
		map[string]any{"title": "Erro", "message": "Investigando.", "impact": "minor"}), http.StatusCreated).json(t), "id")

	const n = 6
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = h.call(http.MethodPost, "/api/v1/admin/incidents/"+id+"/resolve", map[string]any{"message": "Resolvido."}).status
		}()
	}
	wg.Wait()
	ok := 0
	for _, s := range statuses {
		switch s {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected status %d", s)
		}
	}
	var resolutions int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM incident_updates WHERE incident_id = $1 AND status = 'resolved'", id).Scan(&resolutions); err != nil {
		t.Fatal(err)
	}
	if ok != 1 || resolutions != 1 {
		t.Fatalf("successes = %d, recorded resolutions = %d; want 1 and 1", ok, resolutions)
	}
}

func TestIncidentFilters(t *testing.T) {
	h := newHarness(t)
	c := h.createComponent("API", nil)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "A", "message": "m", "impact": "minor"}), http.StatusCreated)
	h.clock.Advance(time.Minute)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "B", "message": "m", "impact": "critical", "component_ids": []string{str(c, "id")}}), http.StatusCreated)

	byImpact := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incidents?impact=critical", nil), http.StatusOK).json(t)
	byComponent := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incidents?component_id="+str(c, "id"), nil), http.StatusOK).json(t)
	oldestFirst := mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents?sort=started_at", nil), http.StatusOK).json(t)
	if total(byImpact) != 1 || total(byComponent) != 1 || str(list(oldestFirst, "data")[0].(map[string]any), "title") != "A" {
		t.Fatalf("filters: impact=%v component=%v sort=%v", byImpact, byComponent, oldestFirst)
	}
	newest := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents", nil), http.StatusOK).json(t), "data")
	if str(newest[0].(map[string]any), "title") != "B" || newest[0].(map[string]any)["latest_update"] == nil {
		t.Fatalf("default order must be newest first with latest update: %v", newest)
	}
}
