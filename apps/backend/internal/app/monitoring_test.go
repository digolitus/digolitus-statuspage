package app

import (
	"net/http"
	"net/url"
	"testing"
)

func hook(token string) string { return "/api/v1/hooks/monitoring?token=" + url.QueryEscape(token) }

func publicComponent(t *testing.T, h *harness, id string) map[string]any {
	t.Helper()
	sum := mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	for _, c := range list(sum, "components") {
		if m := c.(map[string]any); str(m, "id") == id {
			return m
		}
	}
	t.Fatalf("component %s not public", id)
	return nil
}

func statusOverall(t *testing.T, h *harness) map[string]any {
	t.Helper()
	return obj(mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t), "overall")
}

func TestMonitoringUptimeRobotFlow(t *testing.T) {
	h := newHarness(t)
	portal := h.createComponent("Portal", nil)
	other := h.createComponent("Relatórios", nil)
	pid := str(portal, "id")

	if o := statusOverall(t, h); str(o, "basis") != "declared" || o["monitored_components"].(float64) != 0 {
		t.Fatalf("overall before monitoring: %v", o)
	}

	created := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations",
		map[string]any{"name": "UptimeRobot produção", "provider": "uptimerobot"}), http.StatusCreated).json(t)
	iid, token := str(created, "id"), str(created, "webhook_token")
	if len(token) < 40 || str(created, "token_hint") != token[len(token)-4:] {
		t.Fatalf("token: %v", created)
	}
	// The token is never returned again.
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/monitoring-integrations/"+iid, nil), http.StatusOK).json(t)
	if _, ok := got["webhook_token"]; ok {
		t.Fatal("token must only be shown once")
	}

	anon := h.anonymous()
	// Unknown or missing tokens are not found.
	mustStatus(t, anon.call(http.MethodPost, hook("wrong"), map[string]any{"monitorID": "1", "alertType": "1"}), http.StatusNotFound)
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/hooks/monitoring", map[string]any{"monitorID": "1", "alertType": "1"}), http.StatusNotFound)

	// First signal from an unlinked monitor creates it without effect.
	mustStatus(t, anon.call(http.MethodPost, hook(token),
		map[string]any{"monitorID": "798122451", "monitorFriendlyName": "Site público", "alertType": "1", "alertDetails": "Timeout"}), http.StatusAccepted)
	mons := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/monitors?integration_id="+iid, nil), http.StatusOK).json(t), "data")
	if len(mons) != 1 {
		t.Fatalf("monitors: %v", mons)
	}
	mon := mons[0].(map[string]any)
	if str(mon, "name") != "Site público" || str(mon, "state") != "down" || mon["component"] != nil {
		t.Fatalf("monitor: %v", mon)
	}
	if str(publicComponent(t, h, pid), "status") != "operational" {
		t.Fatal("an unlinked monitor must not change any status")
	}

	// Linking a down monitor applies its effect immediately.
	mid := str(mon, "id")
	linked := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/monitors/"+mid,
		map[string]any{"component_id": pid, "down_status": "partial_outage"}), http.StatusOK).json(t)
	if str(obj(linked, "component"), "name") != "Portal" {
		t.Fatalf("linked: %v", linked)
	}
	pc := publicComponent(t, h, pid)
	if str(pc, "status") != "partial_outage" || pc["monitored"] != true {
		t.Fatalf("public component while down: %v", pc)
	}
	if o := statusOverall(t, h); str(o, "status") != "partial_outage" || str(o, "basis") != "mixed" || o["monitored_components"].(float64) != 1 {
		t.Fatalf("overall while down: %v", o)
	}
	if publicComponent(t, h, str(other, "id"))["monitored"] != false {
		t.Fatal("other components stay declared")
	}

	// The change is in the status history, attributed to the monitor.
	hist := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/status-history?cause_type=monitoring", nil), http.StatusOK).json(t), "data")
	if len(hist) != 1 || str(hist[0].(map[string]any), "cause_id") != mid || str(hist[0].(map[string]any), "status") != "partial_outage" ||
		str(hist[0].(map[string]any), "cause_name") != "Site público" {
		t.Fatalf("status history: %v", hist)
	}

	// Up restores the declared status; form bodies are accepted too.
	mustStatus(t, anon.call(http.MethodPost, hook(token), nil,
		rawBody([]byte("monitorID=798122451&alertType=2"), "application/x-www-form-urlencoded")), http.StatusAccepted)
	if st := str(publicComponent(t, h, pid), "status"); st != "operational" {
		t.Fatalf("after up: %s", st)
	}

	// A declared outage is never hidden by a monitor that is up.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+pid, map[string]any{"status": "major_outage"}), http.StatusOK)
	if st := str(publicComponent(t, h, pid), "status"); st != "major_outage" {
		t.Fatalf("declared outage: %s", st)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/components/"+pid, map[string]any{"status": "operational"}), http.StatusOK)

	// Unreadable alerts are rejected but recorded.
	r := mustStatus(t, anon.call(http.MethodPost, hook(token), map[string]any{"monitorID": "*monitorID*", "alertType": "1"}), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "invalid_signal" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	events := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/monitoring-integrations/"+iid+"/events", nil), http.StatusOK).json(t), "data")
	if len(events) != 3 {
		t.Fatalf("events: %v", events)
	}
	if latest := events[0].(map[string]any); str(latest, "kind") != "invalid" || latest["applied"] != false {
		t.Fatalf("latest event: %v", latest)
	}
	if up := events[1].(map[string]any); str(up, "kind") != "up" || up["applied"] != true {
		t.Fatalf("up event: %v", up)
	}

	// Down again, then deactivating the integration removes its effect.
	mustStatus(t, anon.call(http.MethodPost, hook(token), map[string]any{"monitorID": "798122451", "alertType": "1"}), http.StatusAccepted)
	if st := str(publicComponent(t, h, pid), "status"); st != "partial_outage" {
		t.Fatalf("down again: %s", st)
	}
	integ := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/monitoring-integrations/"+iid, nil), http.StatusOK).json(t)
	if integ["down_count"].(float64) != 1 || integ["linked_count"].(float64) != 1 || integ["last_received_at"] == nil {
		t.Fatalf("integration counters: %v", integ)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/monitoring-integrations/"+iid, map[string]any{"is_active": false}), http.StatusOK)
	pc = publicComponent(t, h, pid)
	if str(pc, "status") != "operational" || pc["monitored"] != false {
		t.Fatalf("inactive integration: %v", pc)
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/monitoring-integrations/"+iid, map[string]any{"is_active": true}), http.StatusOK)

	// Rotating the token invalidates the old URL.
	rot := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations/"+iid+"/token", nil), http.StatusOK).json(t)
	mustStatus(t, anon.call(http.MethodPost, hook(token), map[string]any{"monitorID": "798122451", "alertType": "2"}), http.StatusNotFound)
	mustStatus(t, anon.call(http.MethodPost, hook(str(rot, "webhook_token")), map[string]any{"monitorID": "798122451", "alertType": "2"}), http.StatusAccepted)
	if st := str(publicComponent(t, h, pid), "status"); st != "operational" {
		t.Fatalf("after rotated up: %s", st)
	}

	// Filter by component (used by the component screen).
	byComp := list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/monitors?component_id="+pid, nil), http.StatusOK).json(t), "data")
	if len(byComp) != 1 {
		t.Fatalf("monitors by component: %v", byComp)
	}

	// Changes are audited.
	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=monitoring_integration&resource_id="+iid, nil), http.StatusOK).json(t)
	if total(audit) < 5 {
		t.Fatalf("audit: %v", audit)
	}

	// Deleting the integration removes monitors and their effect.
	mustStatus(t, anon.call(http.MethodPost, hook(str(rot, "webhook_token")), map[string]any{"monitorID": "798122451", "alertType": "1"}), http.StatusAccepted)
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/monitoring-integrations/"+iid, nil), http.StatusNoContent)
	if st := str(publicComponent(t, h, pid), "status"); st != "operational" {
		t.Fatalf("after delete: %s", st)
	}
}

func TestMonitoringHealthchecksAndManualMonitor(t *testing.T) {
	h := newHarness(t)
	backup := h.createComponent("Backup diário", nil)
	bid := str(backup, "id")
	created := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations",
		map[string]any{"name": "Jobs noturnos", "provider": "healthchecks"}), http.StatusCreated).json(t)
	iid, token := str(created, "id"), str(created, "webhook_token")

	// Linked before the first signal: not "monitored" until a signal arrives.
	mon := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations/"+iid+"/monitors",
		map[string]any{"external_id": "6f3c1d2e", "name": "Backup", "component_id": bid}), http.StatusCreated).json(t)
	if str(mon, "down_status") != "major_outage" || str(mon, "state") != "unknown" {
		t.Fatalf("manual monitor: %v", mon)
	}
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations/"+iid+"/monitors",
		map[string]any{"external_id": "6f3c1d2e"}), http.StatusConflict)
	if r.errorCode(t) != "monitor_conflict" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	if publicComponent(t, h, bid)["monitored"] != false {
		t.Fatal("a monitor without signals proves nothing")
	}

	// Bearer token and Healthchecks fields.
	mustStatus(t, h.anonymous().call(http.MethodPost, "/api/v1/hooks/monitoring",
		map[string]any{"check_id": "6f3c1d2e", "name": "Backup", "status": "down"}, withHeader("Authorization", "Bearer "+token)), http.StatusAccepted)
	pc := publicComponent(t, h, bid)
	if str(pc, "status") != "major_outage" || pc["monitored"] != true {
		t.Fatalf("down: %v", pc)
	}
	if o := statusOverall(t, h); str(o, "basis") != "monitored" {
		t.Fatalf("all public components monitored: %v", o)
	}

	// Unlinking removes the effect; deleting the monitor works.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/monitors/"+str(mon, "id"), map[string]any{"component_id": nil}), http.StatusOK)
	if st := str(publicComponent(t, h, bid), "status"); st != "operational" {
		t.Fatalf("after unlink: %s", st)
	}
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/monitors/"+str(mon, "id"), nil), http.StatusNoContent)
	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/monitors/"+str(mon, "id"), nil), http.StatusNotFound)
}

func TestMonitoringValidationAndPermissions(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations",
		map[string]any{"name": "X", "provider": "pingdom"}, invalidRequest()), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	created := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations",
		map[string]any{"name": "UR", "provider": "uptimerobot"}), http.StatusCreated).json(t)
	iid := str(created, "id")
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations/"+iid+"/monitors",
		map[string]any{"external_id": "1", "component_id": "00000000-0000-0000-0000-000000000000"}), http.StatusUnprocessableEntity)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/monitoring-integrations/"+iid+"/monitors",
		map[string]any{"external_id": "1", "down_status": "maintenance"}, invalidRequest()), http.StatusUnprocessableEntity)

	reader := h.userWith("Leitor")
	mustStatus(t, reader.call(http.MethodGet, "/api/v1/admin/monitoring-integrations", nil), http.StatusOK)
	mustStatus(t, reader.call(http.MethodPost, "/api/v1/admin/monitoring-integrations",
		map[string]any{"name": "UR2", "provider": "uptimerobot"}), http.StatusForbidden)
	manager := h.userWith("Gestor", "monitoring.manage")
	mustStatus(t, manager.call(http.MethodPatch, "/api/v1/admin/monitoring-integrations/"+iid, map[string]any{"name": "UR novo"}), http.StatusOK)
}
