package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/testsupport"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/subscription"
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
	t       *testing.T
	pool    *pgxpool.Pool
	server  *httptest.Server
	router  routers.Router
	clock   *fakeClock
	fetcher *fakeFetcher
	app     *App
	mailer  *fakeMailer
	poster  *fakePoster
	// session is the cookie value sent with every call ("" = anonymous).
	session string
}

// fakeFetcher serves third-party snapshots from memory, keyed by base URL.
type fakeFetcher struct {
	mu    sync.Mutex
	snaps map[string]external.Snapshot
	errs  map[string]error
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context, base string) (external.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err, ok := f.errs[base]; ok {
		return external.Snapshot{}, err
	}
	snap, ok := f.snaps[base]
	if !ok {
		return external.Snapshot{}, &external.FetchError{Message: "o provedor respondeu HTTP 404 em /api/v2/summary.json"}
	}
	return snap, nil
}

func (f *fakeFetcher) set(base string, snap external.Snapshot, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.errs[base] = err
		return
	}
	delete(f.errs, base)
	f.snaps[base] = snap
}

// fakeMailer records e-mails instead of sending them.
type fakeMailer struct {
	mu   sync.Mutex
	sent []subscription.Mail
}

func (m *fakeMailer) Send(_ context.Context, mail subscription.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mail)
	return nil
}

// take returns and clears the recorded e-mails.
func (m *fakeMailer) take() []subscription.Mail {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.sent
	m.sent = nil
	return out
}

// fakePoster records webhook/Slack posts; err makes them fail.
type fakePoster struct {
	mu    sync.Mutex
	posts []fakePost
	err   error
}

type fakePost struct {
	URL     string
	Body    []byte
	Headers map[string]string
}

func (p *fakePoster) Post(_ context.Context, url string, body []byte, headers map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.posts = append(p.posts, fakePost{URL: url, Body: body, Headers: headers})
	return nil
}

func (p *fakePoster) take() []fakePost {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.posts
	p.posts = nil
	return out
}

// notify runs one cycle of the notification worker.
func (h *harness) notify() {
	subscription.NewWorker(h.app.Subscriptions, slog.New(slog.DiscardHandler)).RunOnce(context.Background())
}

const publicBaseURL = "https://status.example.com"

var epoch = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// newBareHarness starts with an empty database and no user (setup pending).
func newBareHarness(t *testing.T) *harness {
	t.Helper()
	return newBareHarnessWith(t, nil)
}

// newBareHarnessWith is newBareHarness with adjusted options.
func newBareHarnessWith(t *testing.T, adjust func(*Options)) *harness {
	t.Helper()
	pool := testsupport.NewPool(t)
	fc := &fakeClock{now: epoch}
	ff := &fakeFetcher{snaps: map[string]external.Snapshot{}, errs: map[string]error{}}
	fm, fp := &fakeMailer{}, &fakePoster{}
	opts := Options{AdminAPIEnabled: true, Clock: func() time.Time { return clock.Normalize(fc.Now()) },
		ExternalFetcher: ff, ExternalInterval: 2 * time.Minute, Mailer: fm, Poster: fp, PublicBaseURL: publicBaseURL, SiteName: "Status Teste", Auth: auth.Config{CookieSecure: true, SessionTTL: 72 * time.Hour, IdleTimeout: 24 * time.Hour}}
	if adjust != nil {
		adjust(&opts)
	}
	application := New(pool, opts)
	srv := httptest.NewServer(application.Handler)
	t.Cleanup(srv.Close)
	doc := loadSpec(t)
	doc.Servers = openapi3.Servers{{URL: srv.URL}}
	router, err := legacyrouter.NewRouter(doc)
	if err != nil {
		t.Fatalf("build OpenAPI router: %v", err)
	}
	return &harness{t: t, pool: pool, server: srv, router: router, clock: fc, fetcher: ff, app: application, mailer: fm, poster: fp}
}

// newHarness starts signed in as the first administrator, created through
// the real setup flow.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, nil)
}

// newHarnessWith is newHarness with adjusted options.
func newHarnessWith(t *testing.T, adjust func(*Options)) *harness {
	t.Helper()
	h := newBareHarnessWith(t, adjust)
	token, err := h.app.Auth.EnsureSetupToken(context.Background())
	if err != nil || token == "" {
		t.Fatalf("setup token: %q %v", token, err)
	}
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/auth/setup", map[string]any{
		"setup_token": token, "name": "Admin", "email": adminEmail, "password": adminPassword,
	}), http.StatusCreated)
	h.session = sessionCookie(t, r)
	return h
}

const (
	adminEmail    = "admin@example.com"
	adminPassword = "correct horse battery staple"
)

// sessionCookie extracts the session token set by a response.
func sessionCookie(t *testing.T, r response) string {
	t.Helper()
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == auth.CookieName {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || !c.Secure {
				t.Fatalf("session cookie must be HttpOnly, SameSite=Strict and Secure: %+v", c)
			}
			return c.Value
		}
	}
	t.Fatalf("no session cookie in response: %v", r.header)
	return ""
}

// anonymous returns a copy of the harness without a session.
func (h *harness) anonymous() *harness {
	c := *h
	c.session = ""
	return &c
}

// login returns a copy of the harness signed in as email.
func (h *harness) login(email, password string) *harness {
	h.t.Helper()
	r := mustStatus(h.t, h.anonymous().call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": password}), http.StatusOK)
	c := *h
	c.session = sessionCookie(h.t, r)
	return &c
}

// userWith creates a user holding a new role with perms and signs in as it.
func (h *harness) userWith(name string, perms ...string) *harness {
	h.t.Helper()
	if perms == nil {
		perms = []string{}
	}
	role := mustStatus(h.t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{"name": "Papel " + name, "permissions": perms}), http.StatusCreated).json(h.t)
	email := strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@example.com"
	mustStatus(h.t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": email, "name": name, "password": "a long enough password", "role_ids": []string{str(role, "id")},
	}), http.StatusCreated)
	u := h.login(email, "a long enough password")
	u.choosePassword("a long enough password", "my own long password")
	return u
}

// choosePassword replaces a password set by an administrator.
func (h *harness) choosePassword(current, next string) {
	h.t.Helper()
	mustStatus(h.t, h.call(http.MethodPost, "/api/v1/auth/password", map[string]any{"current_password": current, "new_password": next}), http.StatusNoContent)
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
		if h.session != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: h.session})
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
		Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
	if !cfg.skipRequestValidation {
		if err := openapi3filter.ValidateRequest(ctx, input); err != nil {
			h.t.Fatalf("request %s %s violates the contract: %v", method, path, err)
		}
	}

	resp, err := noRedirects.Do(newReq())
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

// noRedirects returns redirects as responses, so tests can check them.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

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
