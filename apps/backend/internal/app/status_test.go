package app

import (
	"net/http"
	"testing"
)

func overall(t *testing.T, h *harness) map[string]any {
	t.Helper()
	return obj(mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t), "overall")
}

func TestStatusSummary(t *testing.T) {
	h := newHarness(t)

	if o := overall(t, h); str(o, "status") != "unknown" || str(o, "basis") != "declared" {
		t.Fatalf("empty platform must be unknown/declared: %v", o)
	}

	api := h.createComponent("API", nil)
	web := h.createComponent("Web", nil)
	hidden := h.createComponent("Interno", map[string]any{"is_public": false})
	if o := overall(t, h); str(o, "status") != "operational" {
		t.Fatalf("all operational: %v", o)
	}

	// Maintenance on one component: overall "maintenance", not an outage.
	mid := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances", map[string]any{
		"title": "Upgrade", "timezone": "UTC", "scheduled_start": "2026-03-01T12:30:00Z", "scheduled_end": "2026-03-01T13:00:00Z",
		"component_ids": []string{str(web, "id")},
	}), http.StatusCreated).json(t), "id")
	sum := mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	if len(list(obj(sum, "maintenances"), "upcoming")) != 1 || str(obj(sum, "overall"), "status") != "operational" {
		t.Fatalf("scheduled maintenance must not change status: %v", sum)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances/"+mid+"/start", nil), http.StatusOK)
	o := overall(t, h)
	if str(o, "status") != "maintenance" || o["components_under_maintenance"].(float64) != 1 || o["affected_components"].(float64) != 0 {
		t.Fatalf("in-progress maintenance: %v", o)
	}

	// Incident on a hidden component: its impact counts, the component stays hidden.
	inc := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Falha interna", "message": "Investigando.", "impact": "major", "component_ids": []string{str(hidden, "id")},
	}), http.StatusCreated).json(t), "id")
	sum = mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	if str(obj(sum, "overall"), "status") != "partial_outage" {
		t.Fatalf("incident impact must reach overall: %v", obj(sum, "overall"))
	}
	if len(list(sum, "components")) != 2 {
		t.Fatalf("hidden component must not be listed: %v", list(sum, "components"))
	}
	incidents := list(sum, "active_incidents")
	if len(incidents) != 1 || len(list(incidents[0].(map[string]any), "components")) != 0 {
		t.Fatalf("hidden components must not appear in public incidents: %v", incidents)
	}

	// Critical incident on a public component wins over everything.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "API fora", "message": "Indisponível.", "impact": "critical", "component_ids": []string{str(api, "id")},
	}), http.StatusCreated)
	sum = mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	statuses := map[string]string{}
	for _, c := range list(sum, "components") {
		statuses[str(c.(map[string]any), "name")] = str(c.(map[string]any), "status")
	}
	if str(obj(sum, "overall"), "status") != "major_outage" || statuses["API"] != "major_outage" || statuses["Web"] != "maintenance" {
		t.Fatalf("effective statuses: overall=%v components=%v", obj(sum, "overall"), statuses)
	}

	// Resolving restores the declared status.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+inc+"/resolve", map[string]any{"message": "ok"}), http.StatusOK)
	pub := mustStatus(t, h.call(http.MethodGet, "/api/v1/components", nil), http.StatusOK).json(t)
	if total(pub) != 2 {
		t.Fatalf("public components: %v", pub)
	}
}
