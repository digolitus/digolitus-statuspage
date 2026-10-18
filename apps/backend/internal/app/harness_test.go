package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/testsupport"
)

// fakeClock is a controllable server clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// harness runs the full application against a fresh PostgreSQL database and
// validates every request and response against the OpenAPI contract.
type harness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	server *httptest.Server
	router routers.Router
	clock  *fakeClock
}

var epoch = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := testsupport.NewPool(t)
	fc := &fakeClock{now: epoch}
	application := New(pool, Options{AdminAPIEnabled: true, Clock: func() time.Time { return clock.Normalize(fc.Now()) }})
	srv := httptest.NewServer(application.Handler)
	t.Cleanup(srv.Close)
	doc := loadSpec(t)
	doc.Servers = openapi3.Servers{{URL: srv.URL}}
	router, err := legacyrouter.NewRouter(doc)
	if err != nil {
		t.Fatalf("build OpenAPI router: %v", err)
	}
	return &harness{t: t, pool: pool, server: srv, router: router, clock: fc}
}

// with returns a copy of the harness bound to t (for subtests).
func (h *harness) with(t *testing.T) *harness {
	c := *h
	c.t = t
	return &c
}

type callOpt func(*callConfig)

type callConfig struct {
	skipRequestValidation bool
	headers               map[string]string
	rawBody               []byte
	contentType           string
}

// invalidRequest marks requests that intentionally violate the contract
// (to test server-side validation); only the response is validated.
func invalidRequest() callOpt { return func(c *callConfig) { c.skipRequestValidation = true } }

func withHeader(k, v string) callOpt {
	return func(c *callConfig) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[k] = v
	}
}

func rawBody(b []byte, contentType string) callOpt {
	return func(c *callConfig) { c.rawBody, c.contentType = b, contentType; c.skipRequestValidation = true }
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("decode response %q: %v", r.body, err)
	}
	return m
}

func (r response) errorCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func (h *harness) call(method, path string, body any, opts ...callOpt) response {
	h.t.Helper()
	cfg := callConfig{contentType: "application/json"}
	for _, o := range opts {
		o(&cfg)
	}
	var payload []byte
	switch {
	case cfg.rawBody != nil:
		payload = cfg.rawBody
	case body != nil:
		var err error
		if payload, err = json.Marshal(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
		}
	}

	newReq := func() *http.Request {
		var r io.Reader
		if payload != nil {
			r = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, h.server.URL+path, r)
		if err != nil {
			h.t.Fatalf("new request: %v", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", cfg.contentType)
		}
		for k, v := range cfg.headers {
			req.Header.Set(k, v)
		}
		return req
	}

	ctx := context.Background()
	vreq := newReq()
	route, params, err := h.router.FindRoute(vreq)
	if err != nil {
		h.t.Fatalf("%s %s is not in the OpenAPI spec: %v", method, path, err)
	}
	input := &openapi3filter.RequestValidationInput{Request: vreq, PathParams: params, Route: route,
		Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true}}
	if !cfg.skipRequestValidation {
		if err := openapi3filter.ValidateRequest(ctx, input); err != nil {
			h.t.Fatalf("request %s %s violates the contract: %v", method, path, err)
		}
	}

	resp, err := http.DefaultClient.Do(newReq())
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("X-Request-ID") == "" {
		h.t.Errorf("%s %s: missing X-Request-ID header", method, path)
	}

	if err := openapi3filter.ValidateResponse(ctx, &openapi3filter.ResponseValidationInput{
		RequestValidationInput: input,
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
		Body:                   io.NopCloser(bytes.NewReader(data)),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}); err != nil {
		h.t.Fatalf("response %s %s (%d) violates the contract: %v\nbody: %s", method, path, resp.StatusCode, err, data)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: data}
}

// mustStatus fails unless the response has the expected status.
func mustStatus(t *testing.T, r response, want int) response {
	t.Helper()
	if r.status != want {
		t.Fatalf("status = %d, want %d; body: %s", r.status, want, r.body)
	}
	return r
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func list(m map[string]any, key string) []any {
	l, _ := m[key].([]any)
	return l
}

func obj(m map[string]any, key string) map[string]any {
	o, _ := m[key].(map[string]any)
	return o
}

func total(m map[string]any) int {
	return int(obj(m, "pagination")["total"].(float64))
}

func (h *harness) createComponent(name string, extra map[string]any) map[string]any {
	h.t.Helper()
	body := map[string]any{"name": name}
	for k, v := range extra {
		body[k] = v
	}
	return mustStatus(h.t, h.call(http.MethodPost, "/api/v1/admin/components", body), http.StatusCreated).json(h.t)
}
