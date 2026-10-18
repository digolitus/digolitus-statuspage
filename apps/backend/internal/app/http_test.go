package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestHTTPErrorHandling(t *testing.T) {
	h := newHarness(t)

	mustStatus(t, h.call(http.MethodGet, "/healthz", nil), http.StatusOK)
	mustStatus(t, h.call(http.MethodGet, "/readyz", nil), http.StatusOK)

	r := mustStatus(t, h.call(http.MethodGet, "/api/v1/incidents/not-a-uuid", nil, invalidRequest()), http.StatusNotFound)
	if r.errorCode(t) != "incident_not_found" {
		t.Fatalf("malformed IDs must be 404: %s", r.body)
	}

	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", nil, rawBody([]byte(`{"name":`), "application/json")), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "invalid_json" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", nil, rawBody([]byte(`{"name":"a"}{"name":"b"}`), "application/json")), http.StatusUnprocessableEntity)
	if r.errorCode(t) != "invalid_json" {
		t.Fatalf("trailing data must be rejected: %s", r.body)
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", nil, rawBody([]byte(`name=a`), "application/x-www-form-urlencoded")), http.StatusUnsupportedMediaType)
	if r.errorCode(t) != "unsupported_media_type" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	big := `{"name":"` + strings.Repeat("a", 70<<10) + `"}`
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", nil, rawBody([]byte(big), "application/json")), http.StatusRequestEntityTooLarge)
	if r.errorCode(t) != "payload_too_large" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
	r = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "a", "owner": "x"}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"owner"`) {
		t.Fatalf("unknown fields must be reported: %s", r.body)
	}

	// Request IDs supplied by clients are propagated to responses and audit.
	c := h.call(http.MethodPost, "/api/v1/admin/components", map[string]any{"name": "Rastreado"}, withHeader("X-Request-ID", "req-abc.123"))
	if c.header.Get("X-Request-ID") != "req-abc.123" {
		t.Fatalf("request id not echoed: %v", c.header)
	}
	events := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?action=created", nil), http.StatusOK).json(t)
	e := list(events, "data")[0].(map[string]any)
	if str(e, "request_id") != "req-abc.123" || str(e, "source") != "admin_api" {
		t.Fatalf("audit event: %v", e)
	}
	// Invalid request IDs are replaced, never reflected.
	c = h.call(http.MethodGet, "/healthz", nil, withHeader("X-Request-ID", "<script>"))
	if c.header.Get("X-Request-ID") == "<script>" {
		t.Fatal("unsafe request id reflected")
	}
	if c.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing: %v", c.header)
	}
}

func TestRouterJSONErrors(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.server.URL + "/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("unknown route: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	req, _ := http.NewRequest(http.MethodDelete, h.server.URL+"/api/v1/admin/components", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") == "" ||
		!strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("wrong method: %d allow=%q ct=%q", resp.StatusCode, resp.Header.Get("Allow"), resp.Header.Get("Content-Type"))
	}
}

func TestSwaggerUI(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/swagger/", "/swagger/init.js", "/swagger/swagger-ui-bundle.js"} {
		resp, err := http.Get(h.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatalf("%s: status %d csp %q", path, resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
		}
	}
	spec := mustStatus(t, h.call(http.MethodGet, "/api/openapi.yaml", nil), http.StatusOK)
	if !strings.HasPrefix(string(spec.body), "openapi: 3.0.3") {
		t.Fatalf("spec not served: %.40s", spec.body)
	}
}
