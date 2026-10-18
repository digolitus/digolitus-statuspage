package app

import (
	"net/http"
	"testing"
	"time"
)

func TestMaintenanceLifecycle(t *testing.T) {
	h := newHarness(t)
	c := h.createComponent("Banco", nil)

	// Local time interpreted in the given IANA zone: 02:00 in São Paulo
	// (UTC-3, no DST) is 05:00 UTC.
	m := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances", map[string]any{
		"title": "Atualização", "timezone": "America/Sao_Paulo",
		"scheduled_start": "2026-03-05T02:00", "scheduled_end": "2026-03-05T04:00",
		"expected_impact": "degraded_performance", "component_ids": []string{str(c, "id")},
		"message": "Janela programada.",
	}), http.StatusCreated).json(t)
	id := str(m, "id")
	if str(m, "scheduled_start") != "2026-03-05T05:00:00Z" || str(m, "scheduled_end") != "2026-03-05T07:00:00Z" {
		t.Fatalf("times not interpreted in timezone: %v", m)
	}
	if str(m, "status") != "scheduled" || m["start_overdue"] != false {
		t.Fatalf("unexpected state: %v", m)
	}

	upcoming := mustStatus(t, h.call(http.MethodGet, "/api/v1/maintenances?state=upcoming", nil), http.StatusOK).json(t)
	if total(upcoming) != 1 {
		t.Fatalf("expected upcoming maintenance: %v", upcoming)
	}

	// Reschedule while scheduled (RFC 3339 with offset is accepted too).
	re := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenances/"+id,
		map[string]any{"scheduled_start": "2026-03-06T05:00:00Z", "scheduled_end": "2026-03-06T08:00:00Z"}), http.StatusOK).json(t)
	if str(re, "scheduled_start") != "2026-03-06T05:00:00Z" {
		t.Fatalf("reschedule: %v", re)
	}

	// No automatic transition: past the planned start it stays scheduled,
	// flagged as overdue.
	h.clock.now = time.Date(2026, 3, 6, 6, 0, 0, 0, time.UTC)
	// Days later: the earlier session has expired; sign in again.
	h = h.login(adminEmail, adminPassword)
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/maintenances/"+id, nil), http.StatusOK).json(t)
	if str(got, "status") != "scheduled" || got["start_overdue"] != true {
		t.Fatalf("expected overdue scheduled maintenance: %v", got)
	}

	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/complete", nil), http.StatusConflict)
	if r.errorCode(t) != "maintenance_invalid_transition" {
		t.Fatalf("code = %s", r.errorCode(t))
	}

	started := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/start", map[string]any{"message": "Iniciada."}), http.StatusOK).json(t)
	if str(started, "status") != "in_progress" || str(started, "started_at") != "2026-03-06T06:00:00Z" {
		t.Fatalf("start: %v", started)
	}

	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenances/"+id, map[string]any{"scheduled_start": "2026-03-06T05:30:00Z"}), http.StatusConflict)
	if r.errorCode(t) != "maintenance_already_started" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	// Extending the end of a running window is allowed.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenances/"+id, map[string]any{"scheduled_end": "2026-03-06T09:00:00Z"}), http.StatusOK)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/updates", map[string]any{"message": "Progresso: 50%."}), http.StatusCreated)

	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/cancel", nil), http.StatusConflict)
	if r.errorCode(t) != "maintenance_invalid_transition" {
		t.Fatalf("in-progress maintenance must not be cancellable: %s", r.body)
	}

	h.clock.Advance(4 * time.Hour)
	running := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/maintenances/"+id, nil), http.StatusOK).json(t)
	if running["end_overdue"] != true || str(running, "status") != "in_progress" {
		t.Fatalf("expected overdue running maintenance: %v", running)
	}

	done := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/complete", nil), http.StatusOK).json(t)
	kinds := []string{}
	for _, u := range list(done, "updates") {
		kinds = append(kinds, str(u.(map[string]any), "kind"))
	}
	if str(done, "status") != "completed" || len(kinds) != 4 || kinds[0] != "completed" || kinds[3] != "created" {
		t.Fatalf("completion/timeline: status=%s kinds=%v", str(done, "status"), kinds)
	}
	if list(done, "updates")[0].(map[string]any)["message"] != nil {
		t.Fatalf("transition without message must publish null message: %v", list(done, "updates")[0])
	}

	r = mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/maintenances/"+id, map[string]any{"title": "x"}), http.StatusConflict)
	if r.errorCode(t) != "maintenance_closed" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	past := mustStatus(t, h.call(http.MethodGet, "/api/v1/maintenances?state=past", nil), http.StatusOK).json(t)
	if total(past) != 1 {
		t.Fatalf("completed maintenance must be in history: %v", past)
	}
}

func TestMaintenanceCancel(t *testing.T) {
	h := newHarness(t)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances", map[string]any{
		"title": "Migração", "timezone": "UTC", "scheduled_start": "2026-04-01T00:00:00Z", "scheduled_end": "2026-04-01T01:00:00Z",
	}), http.StatusCreated).json(t), "id")
	c := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/cancel", map[string]any{"message": "Adiada."}), http.StatusOK).json(t)
	if str(c, "status") != "cancelled" || c["cancelled_at"] == nil {
		t.Fatalf("cancel: %v", c)
	}
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/updates", map[string]any{"message": "x"}), http.StatusConflict)
	if r.errorCode(t) != "maintenance_closed" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+id+"/start", nil), http.StatusConflict)
	if r.errorCode(t) != "maintenance_invalid_transition" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
}

func TestMaintenanceValidation(t *testing.T) {
	h := newHarness(t)
	base := func(over map[string]any) map[string]any {
		b := map[string]any{"title": "Janela", "timezone": "UTC", "scheduled_start": "2026-04-01T00:00:00Z", "scheduled_end": "2026-04-01T02:00:00Z"}
		for k, v := range over {
			b[k] = v
		}
		return b
	}
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"end before start", base(map[string]any{"scheduled_end": "2026-03-31T23:00:00Z"}), "scheduled_end"},
		{"end equals start", base(map[string]any{"scheduled_end": "2026-04-01T00:00:00Z"}), "scheduled_end"},
		{"end in the past", base(map[string]any{"scheduled_start": "2026-02-01T00:00:00Z", "scheduled_end": "2026-02-01T01:00:00Z"}), "scheduled_end"},
		{"window too long", base(map[string]any{"scheduled_end": "2026-06-01T00:00:00Z"}), "scheduled_end"},
		{"invalid timezone", base(map[string]any{"timezone": "Mars/Olympus"}), "timezone"},
		{"machine-local timezone", base(map[string]any{"timezone": "Local"}), "timezone"},
		{"malformed instant", base(map[string]any{"scheduled_start": "amanhã"}), "scheduled_start"},
		{"unknown impact", base(map[string]any{"expected_impact": "some"}), "expected_impact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mustStatus(t, h.with(t).call(http.MethodPost, "/api/v1/admin/maintenances", tc.body, invalidRequest()), http.StatusUnprocessableEntity)
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
}
