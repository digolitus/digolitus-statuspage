package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// MaxResponseBytes bounds a provider response.
const MaxResponseBytes = 2 << 20

// Snapshot is the normalized content of a provider status page.
type Snapshot struct {
	PageName             string
	Indicator            string
	IndicatorDescription string
	Components           []SnapshotComponent
	Events               []Event
}

// SnapshotComponent is one leaf component reported by the provider.
type SnapshotComponent struct {
	ExternalID  string
	Name        string
	Description string
	GroupName   *string
	Status      string
	Position    int
	UpdatedAt   *time.Time
}

// Event is an unresolved incident or in-progress maintenance reported by the
// provider. ComponentIDs are provider component IDs (may be empty).
type Event struct {
	Kind         string     `json:"kind"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	Impact       string     `json:"impact"`
	URL          string     `json:"url"`
	UpdatedAt    *time.Time `json:"updated_at"`
	ComponentIDs []string   `json:"component_ids"`
}

// Fetcher reads provider snapshots.
type Fetcher interface {
	Fetch(ctx context.Context, baseURL string) (Snapshot, error)
}

// FetchError is a provider failure safe to show to operators.
type FetchError struct{ Message string }

func (e *FetchError) Error() string { return e.Message }

func fetchErr(format string, args ...any) error {
	return &FetchError{Message: fmt.Sprintf(format, args...)}
}

// NormalizeBaseURL turns operator input ("githubstatus.com",
// "https://www.githubstatus.com/history") into "https://host[:port]".
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("informe a URL da página de status")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("URL inválida")
	}
	if u.Scheme != "https" {
		return "", errors.New("use uma URL https://")
	}
	if u.User != nil {
		return "", errors.New("a URL não pode conter usuário ou senha")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", errors.New("endereços locais ou internos não são permitidos")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "", errors.New("informe um nome de domínio, não um endereço IP")
	}
	base := "https://" + host
	if p := u.Port(); p != "" && p != "443" {
		base += ":" + p
	}
	return base, nil
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// publicAddress reports whether ip may be contacted: no loopback, private,
// link-local, multicast, unspecified or special-purpose ranges.
func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// HTTPFetcher reads Statuspage (Atlassian) public APIs.
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher builds a fetcher. Unless allowPrivate is set (tests only),
// connections to non-public addresses are refused at dial time, after DNS
// resolution, which also covers redirects and DNS rebinding. Proxy
// environment variables are ignored so that the check applies to the
// provider itself.
func NewHTTPFetcher(timeout time.Duration, allowPrivate bool) *HTTPFetcher {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !publicAddress(ap.Addr()) {
				return errors.New("destination address is not public")
			}
			return nil
		}
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &HTTPFetcher{client: &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("redirect to non-https URL")
			}
			return nil
		},
	}}
}

// Statuspage v2 summary payload (subset).
type spSummary struct {
	Page *struct {
		Name string `json:"name"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
	Components []struct {
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Description *string    `json:"description"`
		Status      string     `json:"status"`
		Position    int        `json:"position"`
		Group       bool       `json:"group"`
		GroupID     *string    `json:"group_id"`
		UpdatedAt   *time.Time `json:"updated_at"`
	} `json:"components"`
	Incidents             []spEvent `json:"incidents"`
	ScheduledMaintenances []spEvent `json:"scheduled_maintenances"`
}

type spEvent struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Impact     string     `json:"impact"`
	Shortlink  string     `json:"shortlink"`
	UpdatedAt  *time.Time `json:"updated_at"`
	Components []struct {
		ID string `json:"id"`
	} `json:"components"`
}

var componentStatus = map[string]string{
	"operational":          "operational",
	"degraded_performance": "degraded_performance",
	"partial_outage":       "partial_outage",
	"major_outage":         "major_outage",
	"under_maintenance":    "maintenance",
}

var indicators = map[string]bool{"none": true, "minor": true, "major": true, "critical": true, "maintenance": true}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// MaxEvents bounds stored provider events per source.
const MaxEvents = 10

// Fetch reads <base>/api/v2/summary.json.
func (f *HTTPFetcher) Fetch(ctx context.Context, baseURL string) (Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v2/summary.json", nil)
	if err != nil {
		return Snapshot{}, fetchErr("URL inválida")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "DigolitusStatusPage/1.0 (+https://github.com/digolitus/digolitus-statuspage)")
	resp, err := f.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			return Snapshot{}, fetchErr("o provedor não respondeu a tempo")
		}
		if strings.Contains(err.Error(), "not public") {
			return Snapshot{}, fetchErr("o endereço do provedor não é público")
		}
		return Snapshot{}, fetchErr("não foi possível conectar ao provedor")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fetchErr("o provedor respondeu HTTP %d em /api/v2/summary.json; a página usa Atlassian Statuspage?", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return Snapshot{}, fetchErr("falha ao ler a resposta do provedor")
	}
	if len(body) > MaxResponseBytes {
		return Snapshot{}, fetchErr("resposta do provedor excede o limite de tamanho")
	}
	return parseSummary(body)
}

func parseSummary(body []byte) (Snapshot, error) {
	var s spSummary
	if err := json.Unmarshal(body, &s); err != nil || s.Page == nil || s.Components == nil {
		return Snapshot{}, fetchErr("a resposta não é uma API Statuspage válida")
	}
	snap := Snapshot{PageName: clip(s.Page.Name, 255), IndicatorDescription: clip(s.Status.Description, 255)}
	if indicators[s.Status.Indicator] {
		snap.Indicator = s.Status.Indicator
	}
	groups := map[string]string{}
	for _, c := range s.Components {
		if c.Group {
			groups[c.ID] = clip(c.Name, 255)
		}
	}
	for _, c := range s.Components {
		if c.Group || c.ID == "" || strings.TrimSpace(c.Name) == "" {
			continue
		}
		sc := SnapshotComponent{ExternalID: clip(c.ID, 64), Name: clip(c.Name, 255), Position: c.Position, UpdatedAt: c.UpdatedAt, Status: "unknown"}
		if st, ok := componentStatus[c.Status]; ok {
			sc.Status = st
		}
		if c.Description != nil {
			sc.Description = clip(*c.Description, 2000)
		}
		if c.GroupID != nil {
			if g, ok := groups[*c.GroupID]; ok {
				sc.GroupName = &g
			}
		}
		snap.Components = append(snap.Components, sc)
		if len(snap.Components) >= 500 {
			break
		}
	}
	add := func(kind string, events []spEvent, keep func(spEvent) bool) {
		for _, e := range events {
			if len(snap.Events) >= MaxEvents || !keep(e) {
				continue
			}
			ev := Event{Kind: kind, Name: clip(e.Name, 255), Status: clip(e.Status, 32), Impact: clip(e.Impact, 32), UpdatedAt: e.UpdatedAt, ComponentIDs: []string{}}
			if strings.HasPrefix(e.Shortlink, "https://") {
				ev.URL = clip(e.Shortlink, 500)
			}
			for _, c := range e.Components {
				ev.ComponentIDs = append(ev.ComponentIDs, clip(c.ID, 64))
			}
			snap.Events = append(snap.Events, ev)
		}
	}
	add("incident", s.Incidents, func(e spEvent) bool { return e.Status != "resolved" && e.Status != "postmortem" })
	add("maintenance", s.ScheduledMaintenances, func(e spEvent) bool { return e.Status == "in_progress" || e.Status == "verifying" })
	return snap, nil
}
