package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

const sampleSummary = `{
  "page": {"id": "p1", "name": "GitHub", "url": "https://www.githubstatus.com"},
  "status": {"indicator": "minor", "description": "Minor Service Outage"},
  "components": [
    {"id": "g1", "name": "Core", "status": "partial_outage", "position": 1, "group": true, "group_id": null},
    {"id": "c1", "name": "Git Operations", "status": "operational", "position": 1, "group": false, "group_id": "g1", "description": "Clone and push"},
    {"id": "c2", "name": "API Requests", "status": "partial_outage", "position": 2, "group": false, "group_id": "g1", "description": null},
    {"id": "c3", "name": "Pages", "status": "under_maintenance", "position": 3, "group": false, "group_id": null},
    {"id": "c4", "name": "Weird", "status": "something_new", "position": 4, "group": false, "group_id": null}
  ],
  "incidents": [
    {"name": "API degraded", "status": "investigating", "impact": "minor", "shortlink": "https://stspg.io/abc", "components": [{"id": "c2"}]},
    {"name": "Old", "status": "resolved", "impact": "none", "shortlink": "https://stspg.io/old", "components": []}
  ],
  "scheduled_maintenances": [
    {"name": "Pages upgrade", "status": "in_progress", "impact": "maintenance", "shortlink": "javascript:alert(1)", "components": [{"id": "c3"}]},
    {"name": "Later", "status": "scheduled", "impact": "maintenance", "shortlink": "https://stspg.io/later", "components": []}
  ]
}`

func TestParseSummary(t *testing.T) {
	snap, err := parseSummary([]byte(sampleSummary))
	if err != nil {
		t.Fatal(err)
	}
	if snap.PageName != "GitHub" || snap.Indicator != "minor" {
		t.Fatalf("page: %+v", snap)
	}
	if len(snap.Components) != 4 {
		t.Fatalf("groups must be excluded, leaves kept: %+v", snap.Components)
	}
	byID := map[string]SnapshotComponent{}
	for _, c := range snap.Components {
		byID[c.ExternalID] = c
	}
	if byID["c1"].GroupName == nil || *byID["c1"].GroupName != "Core" || byID["c1"].Description != "Clone and push" {
		t.Errorf("group name/description: %+v", byID["c1"])
	}
	if byID["c3"].Status != "maintenance" || byID["c4"].Status != "unknown" || byID["c3"].GroupName != nil {
		t.Errorf("status mapping: c3=%s c4=%s", byID["c3"].Status, byID["c4"].Status)
	}
	if len(snap.Events) != 2 {
		t.Fatalf("only unresolved incidents and in-progress maintenance: %+v", snap.Events)
	}
	if snap.Events[0].URL != "https://stspg.io/abc" || snap.Events[1].URL != "" {
		t.Errorf("only https links are kept: %+v", snap.Events)
	}
}

func TestParseSummaryRejectsNonStatuspage(t *testing.T) {
	for _, body := range []string{`<html>`, `{}`, `{"page": {"name": "x"}}`} {
		if _, err := parseSummary([]byte(body)); err == nil {
			t.Errorf("%q should be rejected", body)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	ok := map[string]string{
		"githubstatus.com":                         "https://githubstatus.com",
		"https://www.githubstatus.com/":            "https://www.githubstatus.com",
		"https://Status.Veracode.com/history?x=1":  "https://status.veracode.com",
		"https://status.example.com:443/":          "https://status.example.com",
		"https://status.example.com:8443/whatever": "https://status.example.com:8443",
	}
	for in, want := range ok {
		got, err := NormalizeBaseURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "http://githubstatus.com", "https://127.0.0.1", "https://[::1]", "https://localhost",
		"https://db.internal", "https://user:pw@status.example.com", "ftp://x.com"} {
		if _, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("NormalizeBaseURL(%q) should fail", bad)
		}
	}
}

// testFetcher returns a fetcher that trusts srv's certificate.
func testFetcher(srv *httptest.Server, allowPrivate bool) *HTTPFetcher {
	f := NewHTTPFetcher(2*time.Second, allowPrivate)
	f.client.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	return f
}

func TestHTTPFetcher(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/summary.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(sampleSummary)) })
	mux.HandleFunc("/big/api/v2/summary.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", MaxResponseBytes+10)))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	ctx := context.Background()

	snap, err := testFetcher(srv, true).Fetch(ctx, srv.URL)
	if err != nil || snap.PageName != "GitHub" {
		t.Fatalf("fetch: %+v %v", snap, err)
	}
	if _, err := testFetcher(srv, true).Fetch(ctx, srv.URL+"/big"); err == nil || !strings.Contains(err.Error(), "limite") {
		t.Fatalf("oversized response must fail: %v", err)
	}
	if _, err := testFetcher(srv, true).Fetch(ctx, srv.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("404 must fail: %v", err)
	}
	// SSRF guard: the test server listens on loopback, which must be refused.
	if _, err := testFetcher(srv, false).Fetch(ctx, srv.URL); err == nil || !strings.Contains(err.Error(), "não é público") {
		t.Fatalf("loopback must be refused: %v", err)
	}
}

func TestHTTPFetcherRefusesRedirectToHTTP(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/api/v2/summary.json", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := testFetcher(srv, true).Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("redirect to http must fail")
	}
}

func TestPublicAddress(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fd00::1", "fe80::1", "::ffff:10.0.0.1"} {
		if publicAddress(mustAddr(t, s)) {
			t.Errorf("%s must not be public", s)
		}
	}
	for _, s := range []string{"140.82.112.3", "2606:50c0:8000::153"} {
		if !publicAddress(mustAddr(t, s)) {
			t.Errorf("%s must be public", s)
		}
	}
}

func TestWorst(t *testing.T) {
	cases := map[string][]string{
		"operational":    {"operational", "operational"},
		"unknown":        {"operational", "unknown"},
		"partial_outage": {"unknown", "partial_outage", "maintenance"},
		"maintenance":    {"operational", "maintenance"},
	}
	for want, in := range cases {
		if got := worst(in); got != want {
			t.Errorf("worst(%v) = %s, want %s", in, got, want)
		}
	}
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
