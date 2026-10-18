package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
)

const githubBase = "https://www.githubstatus.com"

func ptr(s string) *string { return &s }

func githubSnapshot(apiStatus string) external.Snapshot {
	return external.Snapshot{
		PageName:  "GitHub",
		Indicator: "minor",
		Components: []external.SnapshotComponent{
			{ExternalID: "c1", Name: "Git Operations", GroupName: ptr("Core"), Status: "operational", Position: 1},
			{ExternalID: "c2", Name: "API Requests", GroupName: ptr("Core"), Status: apiStatus, Position: 2},
			{ExternalID: "c3", Name: "Codespaces", Status: "major_outage", Position: 3},
		},
		Events: []external.Event{
			{Kind: "incident", Name: "API degraded", Status: "investigating", Impact: "minor", URL: "https://stspg.io/a", ComponentIDs: []string{"c2"}},
			{Kind: "incident", Name: "Codespaces down", Status: "identified", Impact: "major", ComponentIDs: []string{"c3"}},
		},
	}
}

func externalComponents(t *testing.T, h *harness, id string) map[string]map[string]any {
	t.Helper()
	list := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id+"/components", nil), http.StatusOK).json(t)
	out := map[string]map[string]any{}
	for _, c := range list["data"].([]any) {
		m := c.(map[string]any)
		out[str(m, "name")] = m
	}
	return out
}

func thirdParty(t *testing.T, h *harness) []any {
	t.Helper()
	return list(mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t), "third_party")
}

func TestExternalSourceLifecycle(t *testing.T) {
	h := newHarness(t)
	h.fetcher.set(githubBase, githubSnapshot("partial_outage"), nil)

	// URL is normalized; the name defaults to the provider's page name.
	src := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources",
		map[string]any{"url": "www.githubstatus.com/history"}), http.StatusCreated).json(t)
	id := str(src, "id")
	if str(src, "base_url") != githubBase || str(src, "name") != "GitHub" || src["component_count"].(float64) != 3 || src["selected_count"].(float64) != 0 {
		t.Fatalf("created source: %v", src)
	}
	if src["stale"] != false || src["last_error"] != nil {
		t.Fatalf("fresh source must not be stale: %v", src)
	}

	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": "https://www.githubstatus.com"}), http.StatusConflict)
	if r.errorCode(t) != "external_source_conflict" {
		t.Fatalf("duplicate URL: %s", r.body)
	}

	// Nothing selected: not published.
	if len(thirdParty(t, h)) != 0 {
		t.Fatal("sources without selected components must not be public")
	}

	comps := externalComponents(t, h, id)
	if str(comps["API Requests"], "group_name") != "Core" {
		t.Fatalf("group names: %v", comps)
	}
	sel := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/external-sources/"+id+"/selection",
		map[string]any{"component_ids": []string{str(comps["Git Operations"], "id"), str(comps["API Requests"], "id")}}), http.StatusOK).json(t)
	selected := 0
	for _, c := range sel["data"].([]any) {
		if c.(map[string]any)["is_selected"] == true {
			selected++
		}
	}
	if selected != 2 {
		t.Fatalf("selection: %v", sel)
	}

	tp := thirdParty(t, h)
	if len(tp) != 1 {
		t.Fatalf("third_party: %v", tp)
	}
	gh := tp[0].(map[string]any)
	if str(gh, "status") != "partial_outage" || len(list(gh, "components")) != 2 || str(gh, "page_url") != githubBase {
		t.Fatalf("public source: %v", gh)
	}
	events := list(gh, "events")
	if len(events) != 1 || str(events[0].(map[string]any), "name") != "API degraded" {
		t.Fatalf("only events affecting selected components: %v", events)
	}

	// Third-party status never changes the platform's overall status.
	sum := mustStatus(t, h.call(http.MethodGet, "/api/v1/status", nil), http.StatusOK).json(t)
	if str(obj(sum, "overall"), "status") != "unknown" {
		t.Fatalf("overall must ignore third parties: %v", obj(sum, "overall"))
	}

	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/external-sources/"+id, map[string]any{"is_active": false}), http.StatusOK)
	if len(thirdParty(t, h)) != 0 {
		t.Fatal("inactive sources are not published")
	}
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/external-sources/"+id, map[string]any{"is_active": true, "name": "GitHub.com"}), http.StatusOK)

	mustStatus(t, h.call(http.MethodDelete, "/api/v1/admin/external-sources/"+id, nil), http.StatusNoContent)
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id, nil), http.StatusNotFound)

	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=external_source&sort=occurred_at", nil), http.StatusOK).json(t)
	var actions []string
	for _, e := range list(audit, "data") {
		actions = append(actions, str(e.(map[string]any), "action"))
	}
	if strings.Join(actions, ",") != "created,selection_changed,updated,updated,deleted" {
		t.Fatalf("audit actions = %v", actions)
	}
}

func TestExternalSourceValidation(t *testing.T) {
	h := newHarness(t)
	for _, url := range []string{"http://www.githubstatus.com", "https://10.0.0.5", "https://localhost", ""} {
		r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": url}), http.StatusUnprocessableEntity)
		if !strings.Contains(string(r.body), `"field":"url"`) {
			t.Fatalf("%q: expected url error: %s", url, r.body)
		}
	}
	// Unreachable or non-Statuspage pages are rejected at registration.
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": "https://status.example.com"}), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), "HTTP 404") {
		t.Fatalf("fetch failure must be explained: %s", r.body)
	}
	h.fetcher.set(githubBase, githubSnapshot("operational"), nil)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": githubBase}), http.StatusCreated).json(t), "id")
	r = mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/external-sources/"+id+"/selection",
		map[string]any{"component_ids": []string{"7d0e6f1c-9d1f-4a42-8f34-0a1b2c3d4e5f"}}), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "validation_failed" {
		t.Fatalf("foreign component ids must be rejected: %s", r.body)
	}
}

func TestExternalSyncStaleAndPoller(t *testing.T) {
	h := newHarness(t)
	h.fetcher.set(githubBase, githubSnapshot("operational"), nil)
	id := str(mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources", map[string]any{"url": githubBase}), http.StatusCreated).json(t), "id")
	comps := externalComponents(t, h, id)
	mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/external-sources/"+id+"/selection",
		map[string]any{"component_ids": []string{str(comps["API Requests"], "id")}}), http.StatusOK)

	// A failed manual sync keeps the previous snapshot and records the error.
	h.fetcher.set(githubBase, external.Snapshot{}, &external.FetchError{Message: "o provedor não respondeu a tempo"})
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/external-sources/"+id+"/sync", nil), http.StatusBadGateway)
	if r.errorCode(t) != "external_fetch_failed" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	src := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/external-sources/"+id, nil), http.StatusOK).json(t)
	if str(src, "last_error") != "o provedor não respondeu a tempo" || src["stale"] != false {
		t.Fatalf("error recorded, data still fresh: %v", src)
	}

	// Old data is never shown as current: statuses become unknown.
	h.clock.Advance(11 * time.Minute)
	gh := thirdParty(t, h)[0].(map[string]any)
	if gh["stale"] != true || str(gh, "status") != "unknown" || str(list(gh, "components")[0].(map[string]any), "status") != "unknown" {
		t.Fatalf("stale source: %v", gh)
	}

	// The poller refreshes due sources; a component removed upstream disappears.
	snap := githubSnapshot("major_outage")
	snap.Components = snap.Components[1:2] // only API Requests remains
	h.fetcher.set(githubBase, snap, nil)
	external.NewPoller(h.app.External, slog.New(slog.NewTextHandler(io.Discard, nil))).RunOnce(context.Background())
	gh = thirdParty(t, h)[0].(map[string]any)
	if gh["stale"] != false || str(gh, "status") != "major_outage" {
		t.Fatalf("after poll: %v", gh)
	}
	comps = externalComponents(t, h, id)
	if comps["Git Operations"]["is_present"] != false || comps["API Requests"]["is_present"] != true {
		t.Fatalf("presence flags: %v", comps)
	}

	// Not due yet: the poller does not call the provider again.
	calls := h.fetcher.calls
	external.NewPoller(h.app.External, slog.New(slog.NewTextHandler(io.Discard, nil))).RunOnce(context.Background())
	if h.fetcher.calls != calls {
		t.Fatal("poller must respect the interval")
	}
}
