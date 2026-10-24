package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth/social"
)

// fakeProvider authenticates whoever presents a known code.
type fakeProvider struct {
	id    string
	codes map[string]social.Identity
	got   social.Params
}

func (f *fakeProvider) ID() string   { return f.id }
func (f *fakeProvider) Name() string { return "Fake " + f.id }
func (f *fakeProvider) AuthURL(p social.Params) string {
	return "https://idp.example/auth?" + url.Values{"state": {p.State}, "redirect_uri": {p.RedirectURI}}.Encode()
}

func (f *fakeProvider) Exchange(_ context.Context, code string, p social.Params) (social.Identity, error) {
	f.got = p
	id, ok := f.codes[code]
	if !ok {
		return social.Identity{}, &social.Error{Message: "unknown code"}
	}
	return id, nil
}

func newOAuthHarness(t *testing.T) (*harness, *fakeProvider, *fakeProvider) {
	t.Helper()
	google := &fakeProvider{id: "google", codes: map[string]social.Identity{}}
	apple := &fakeProvider{id: "apple", codes: map[string]social.Identity{}}
	h := newHarnessWith(t, func(o *Options) {
		o.Auth.Providers = []social.Provider{google, apple}
		o.Auth.BaseURL = publicBaseURL
	})
	return h, google, apple
}

// oauthStart begins a sign-in and returns the flow cookie and the state.
func oauthStart(t *testing.T, h *harness, provider string) (cookie, state string) {
	t.Helper()
	r := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/auth/oauth/"+provider, nil), http.StatusFound)
	loc, err := url.Parse(r.header.Get("Location"))
	if err != nil || loc.Host != "idp.example" {
		t.Fatalf("Location = %q", r.header.Get("Location"))
	}
	if got := loc.Query().Get("redirect_uri"); got != publicBaseURL+"/api/v1/auth/oauth/"+provider+"/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == "dsp_oauth" {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteNoneMode || c.Path != "/api/v1/auth/oauth/" || c.MaxAge != 600 {
				t.Fatalf("flow cookie attributes: %+v", c)
			}
			return c.Value, loc.Query().Get("state")
		}
	}
	t.Fatal("no dsp_oauth cookie")
	return "", ""
}

// callback returns the redirect target of a GET callback.
func callback(t *testing.T, h *harness, provider, cookie, query string) (string, response) {
	t.Helper()
	var opts []callOpt
	if cookie != "" {
		opts = append(opts, withHeader("Cookie", "dsp_oauth="+cookie))
	}
	r := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/auth/oauth/"+provider+"/callback?"+query, nil, opts...), http.StatusFound)
	return r.header.Get("Location"), r
}

func TestSocialLoginProviders(t *testing.T) {
	h, _, _ := newOAuthHarness(t)
	got := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/auth/providers", nil), http.StatusOK).json(t)
	items := list(got, "data")
	if len(items) != 2 || str(items[0].(map[string]any), "id") != "google" || str(items[1].(map[string]any), "id") != "apple" {
		t.Fatalf("providers: %v", got)
	}
	// Not configured here.
	mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/auth/oauth/facebook", nil), http.StatusNotFound)

	plain := newBareHarness(t)
	if l := list(mustStatus(t, plain.call(http.MethodGet, "/api/v1/auth/providers", nil), http.StatusOK).json(t), "data"); len(l) != 0 {
		t.Fatalf("no provider should be listed: %v", l)
	}
}

func TestSocialLoginOnlyForExistingUsers(t *testing.T) {
	h, google, _ := newOAuthHarness(t)
	user := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "ana@example.com", "name": "Ana", "password": "a long enough password",
	}), http.StatusCreated).json(t)
	google.codes["ana"] = social.Identity{Subject: "g-ana", Email: "ANA@example.com", EmailVerified: true}
	google.codes["ana-new-email"] = social.Identity{Subject: "g-ana", Email: "ana@personal.example", EmailVerified: true}
	google.codes["stranger"] = social.Identity{Subject: "g-x", Email: "x@example.com", EmailVerified: true}
	google.codes["unverified"] = social.Identity{Subject: "g-y", Email: "ana@example.com", EmailVerified: false}
	cookie, state := oauthStart(t, h, "google")
	q := func(code string) string { return url.Values{"code": {code}, "state": {state}}.Encode() }

	for name, tc := range map[string]struct{ cookie, query, want string }{
		"missing cookie":   {"", q("ana"), "/admin?login_error=failed"},
		"wrong state":      {cookie, url.Values{"code": {"ana"}, "state": {"forged"}}.Encode(), "/admin?login_error=failed"},
		"other provider":   {"apple" + cookie[strings.Index(cookie, "."):], q("ana"), "/admin?login_error=failed"},
		"exchange fails":   {cookie, q("bogus"), "/admin?login_error=failed"},
		"cancelled":        {cookie, "error=access_denied&state=" + state, "/admin?login_error=cancelled"},
		"unknown e-mail":   {cookie, q("stranger"), "/admin?login_error=not_allowed"},
		"unverified email": {cookie, q("unverified"), "/admin?login_error=not_allowed"},
		// Not linked yet: the subject alone is not enough.
		"different e-mail": {cookie, q("ana-new-email"), "/admin?login_error=not_allowed"},
	} {
		if got, _ := callback(t, h, "google", tc.cookie, tc.query); got != tc.want {
			t.Errorf("%s: redirected to %q, want %q", name, got, tc.want)
		}
	}

	// Verified e-mail of an existing user: linked and signed in.
	loc, r := callback(t, h, "google", cookie, q("ana"))
	if loc != "/admin" {
		t.Fatalf("redirected to %q", loc)
	}
	parts := strings.Split(cookie, ".")
	if google.got.Nonce != parts[2] || google.got.Verifier != parts[3] || google.got.RedirectURI != publicBaseURL+"/api/v1/auth/oauth/google/callback" {
		t.Fatalf("exchange params: %+v", google.got)
	}
	ana := *h
	ana.session = sessionCookie(t, r)
	me := mustStatus(t, ana.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t)
	if str(obj(me, "user"), "email") != "ana@example.com" || obj(me, "user")["last_login_at"] == nil {
		t.Fatalf("me: %v", me)
	}

	// Linked: recognised by subject even with another e-mail at Google.
	if loc, _ := callback(t, h, "google", cookie, q("ana-new-email")); loc != "/admin" {
		t.Fatalf("linked identity: redirected to %q", loc)
	}

	// Changing the user's e-mail drops the link.
	id := str(user, "id")
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+id, map[string]any{"email": "ana@company.example"}), http.StatusOK)
	if loc, _ := callback(t, h, "google", cookie, q("ana-new-email")); loc != "/admin?login_error=not_allowed" {
		t.Fatalf("after e-mail change: redirected to %q", loc)
	}

	// Inactive users cannot sign in.
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/users/"+id, map[string]any{"email": "ana@example.com", "is_active": false}), http.StatusOK)
	if loc, _ := callback(t, h, "google", cookie, q("ana")); loc != "/admin?login_error=not_allowed" {
		t.Fatalf("inactive: redirected to %q", loc)
	}
}

func TestSocialLoginFormPostCallback(t *testing.T) {
	h, _, apple := newOAuthHarness(t)
	apple.codes["admin"] = social.Identity{Subject: "a-1", Email: adminEmail, EmailVerified: true}
	cookie, state := oauthStart(t, h, "apple")
	form := url.Values{"code": {"admin"}, "state": {state}, "user": {`{"name":{"firstName":"A"}}`}}.Encode()
	r := mustStatus(t, h.anonymous().call(http.MethodPost, "/api/v1/auth/oauth/apple/callback", nil,
		rawBody([]byte(form), "application/x-www-form-urlencoded"), withHeader("Cookie", "dsp_oauth="+cookie),
		withHeader("Sec-Fetch-Site", "cross-site")), http.StatusFound)
	if loc := r.header.Get("Location"); loc != "/admin" {
		t.Fatalf("redirected to %q", loc)
	}
	sessionCookie(t, r)
	// The flow cookie is single use.
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == "dsp_oauth" && c.MaxAge >= 0 {
			t.Fatalf("flow cookie not cleared: %+v", c)
		}
	}
}
