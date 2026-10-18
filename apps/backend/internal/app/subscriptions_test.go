package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var tokenInLink = regexp.MustCompile(`\?token=([A-Za-z0-9_-]+)`)

func linkToken(t *testing.T, text string) string {
	t.Helper()
	m := tokenInLink.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no token link in: %s", text)
	}
	return m[1]
}

func TestEmailSubscriptionLifecycle(t *testing.T) {
	h := newHarness(t)
	anon := h.anonymous()
	api := h.createComponent("API", nil)
	web := h.createComponent("Portal", nil)

	opts := mustStatus(t, anon.call(http.MethodGet, "/api/v1/subscriptions/options", nil), http.StatusOK).json(t)
	if opts["email"] != true || opts["sms"] != false {
		t.Fatalf("options: %v", opts)
	}
	r := mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions", map[string]any{"channel": "email", "email": "x"}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), `"field":"email"`) {
		t.Fatalf("validation: %s", r.body)
	}
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions", map[string]any{"channel": "email", "email": "Pessoa@Example.com"}), http.StatusAccepted)

	// Nothing is sent before confirmation; the confirmation e-mail goes out
	// through the outbox.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "Antes", "message": "m", "impact": "operational"}), http.StatusCreated)
	h.notify()
	mails := h.mailer.take()
	if len(mails) != 1 || mails[0].To != "pessoa@example.com" || !strings.Contains(mails[0].Subject, "Confirme") || mails[0].Unsubscribe != "" {
		t.Fatalf("confirmation mail: %+v", mails)
	}
	if !strings.Contains(mails[0].Text, publicBaseURL+"/subscriptions/confirm?token=") {
		t.Fatalf("confirm link: %s", mails[0].Text)
	}
	token := linkToken(t, mails[0].Text)
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions/confirm", map[string]any{"token": "forged" + token}), http.StatusNotFound)
	view := mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions/confirm", map[string]any{"token": token}), http.StatusOK).json(t)
	if str(view, "status") != "active" || view["all_components"] != true {
		t.Fatalf("confirm: %v", view)
	}

	// An incident is e-mailed with the details, links and one-click unsubscribe.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Lentidão na API", "message": "Investigando a lentidão.", "impact": "degraded_performance", "component_ids": []string{str(api, "id")},
	}), http.StatusCreated)
	h.notify()
	mails = h.mailer.take()
	if len(mails) != 1 {
		t.Fatalf("expected one event mail, got %+v", mails)
	}
	m := mails[0]
	for _, want := range []string{"Novo incidente: Lentidão na API", "Status: Investigando", "Impacto: Performance degradada", "Componentes: API",
		"Investigando a lentidão.", publicBaseURL + "/incidents/", "/subscriptions/manage?token="} {
		if !strings.Contains(m.Subject+"\n"+m.Text, want) {
			t.Errorf("mail missing %q:\n%s\n%s", want, m.Subject, m.Text)
		}
	}
	if !strings.HasPrefix(m.Unsubscribe, publicBaseURL+"/api/v1/subscriptions/unsubscribe?token=") {
		t.Fatalf("unsubscribe header: %q", m.Unsubscribe)
	}
	manage := linkToken(t, m.Text)

	// Follow only "Portal": incidents on API are skipped, general ones are not.
	pref := mustStatus(t, anon.call(http.MethodPut, "/api/v1/subscriptions/manage", map[string]any{"token": manage, "component_ids": []string{str(web, "id")}}), http.StatusOK).json(t)
	if pref["all_components"] != false || len(list(pref, "components")) != 1 {
		t.Fatalf("preferences: %v", pref)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{
		"title": "Só API", "message": "m", "impact": "partial_outage", "component_ids": []string{str(api, "id")},
	}), http.StatusCreated)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "Geral", "message": "m", "impact": "operational"}), http.StatusCreated)
	h.notify()
	mails = h.mailer.take()
	if len(mails) != 1 || !strings.Contains(mails[0].Subject, "Geral") {
		t.Fatalf("component filter: %+v", mails)
	}

	// Subscribing again does not reveal anything; the owner gets a manage link.
	h.clock.Advance(15 * time.Minute)
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions", map[string]any{"channel": "email", "email": "pessoa@example.com"}), http.StatusAccepted)
	h.notify()
	mails = h.mailer.take()
	if len(mails) != 1 || !strings.Contains(mails[0].Subject, "Gerencie") {
		t.Fatalf("manage link mail: %+v", mails)
	}

	// One-click unsubscribe (RFC 8058): form POST to the header URL.
	u, _ := url.Parse(m.Unsubscribe)
	mustStatus(t, anon.call(http.MethodPost, u.Path+"?"+u.RawQuery, nil, rawBody([]byte("List-Unsubscribe=One-Click"), "application/x-www-form-urlencoded")), http.StatusNoContent)
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions/manage", map[string]any{"token": manage}), http.StatusNotFound)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents", map[string]any{"title": "Depois", "message": "m", "impact": "operational"}), http.StatusCreated)
	h.notify()
	if mails := h.mailer.take(); len(mails) != 0 {
		t.Fatalf("mail after unsubscribe: %+v", mails)
	}
}

func TestWebhookAndSlackIntegrations(t *testing.T) {
	h := newHarness(t)
	r := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "slack", "url": "https://example.com/hook"}, invalidRequest()), http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.body), "hooks.slack.com") {
		t.Fatalf("slack url validation: %s", r.body)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "webhook", "url": "http://plain.example.com"}, invalidRequest()), http.StatusUnprocessableEntity)
	hook := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "webhook", "url": "https://hooks.example.com/status"}), http.StatusCreated).json(t)
	secret := str(hook, "signing_secret")
	if str(hook, "status") != "active" || !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("webhook: %v", hook)
	}
	// The secret is never listed again.
	got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/subscriptions/"+str(hook, "id"), nil), http.StatusOK).json(t)
	if _, ok := got["signing_secret"]; ok {
		t.Fatal("secret must not be returned after creation")
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "webhook", "url": "https://hooks.example.com/status"}), http.StatusConflict)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "slack", "url": "https://hooks.slack.com/services/T0/B0/xyz"}), http.StatusCreated)

	// Maintenance scheduled: both integrations receive it.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/maintenances", map[string]any{
		"title": "Atualização <b>do banco</b>", "timezone": "America/Sao_Paulo", "scheduled_start": "2026-03-10T02:00", "scheduled_end": "2026-03-10T04:00",
	}), http.StatusCreated)
	h.notify()
	posts := h.poster.take()
	if len(posts) != 2 {
		t.Fatalf("posts: %+v", posts)
	}
	for _, p := range posts {
		switch p.URL {
		case "https://hooks.example.com/status":
			ts, sig := p.Headers["X-Digolitus-Timestamp"], p.Headers["X-Digolitus-Signature"]
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(ts + "."))
			mac.Write(p.Body)
			if sig != "sha256="+hex.EncodeToString(mac.Sum(nil)) || p.Headers["X-Digolitus-Event"] != "maintenance.scheduled" {
				t.Fatalf("signature/headers: %v", p.Headers)
			}
			var body map[string]any
			_ = json.Unmarshal(p.Body, &body)
			data := body["data"].(map[string]any)
			if body["event"] != "maintenance.scheduled" || data["title"] != "Atualização <b>do banco</b>" || !strings.HasPrefix(body["url"].(string), publicBaseURL+"/maintenances/") {
				t.Fatalf("webhook body: %s", p.Body)
			}
		default:
			var slack struct{ Text string }
			_ = json.Unmarshal(p.Body, &slack)
			text := slack.Text
			// Slack markup from titles is escaped; the window is in the planning zone.
			if !strings.Contains(text, "&lt;b&gt;") || !strings.Contains(text, "10/03/2026 02:00") || !strings.Contains(text, "|Ver detalhes>") {
				t.Fatalf("slack body: %s", text)
			}
		}
	}

	// Failing endpoints are retried with backoff and disabled after many failures.
	h.poster.err = errors.New("endpoint respondeu HTTP 500")
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions/"+str(hook, "id")+"/test", nil), http.StatusAccepted)
	h.notify()
	got = mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/subscriptions/"+str(hook, "id"), nil), http.StatusOK).json(t)
	if got["failure_count"].(float64) != 1 || !strings.Contains(str(got, "last_error"), "500") {
		t.Fatalf("failure recorded: %v", got)
	}
	h.notify() // not due yet (backoff): no new attempt
	if got := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/subscriptions/"+str(hook, "id"), nil), http.StatusOK).json(t); got["failure_count"].(float64) != 1 {
		t.Fatalf("retried before backoff: %v", got)
	}
	if _, err := h.pool.Exec(t.Context(), "UPDATE subscriptions SET failure_count = 14 WHERE id = $1", str(hook, "id")); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(2 * time.Minute)
	h.notify()
	got = mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/subscriptions/"+str(hook, "id"), nil), http.StatusOK).json(t)
	if str(got, "status") != "disabled" || got["disabled_reason"] == nil {
		t.Fatalf("auto-disable: %v", got)
	}
	h.poster.err = nil
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/subscriptions/"+str(hook, "id"), map[string]any{"enabled": true}), http.StatusOK)
	list := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/subscriptions?channel=webhook", nil), http.StatusOK).json(t)
	if total(list) != 1 || obj(list, "counts")["active"].(float64) != 2 {
		t.Fatalf("list: %v", list)
	}

	// Only holders of subscriptions.manage see subscribers (personal data).
	a := h.userWith("Operador", "incidents.manage")
	mustStatus(t, a.call(http.MethodGet, "/api/v1/admin/subscriptions", nil), http.StatusForbidden)
}

func TestAnnouncementNotificationsAndFeeds(t *testing.T) {
	h := newHarness(t)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/subscriptions", map[string]any{"channel": "slack", "url": "https://hooks.slack.com/services/T0/B0/feed"}), http.StatusCreated)
	a := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements", map[string]any{"title": "Nova região", "body": "Disponível **hoje**."}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/announcements/"+str(a, "id")+"/publish",
		map[string]any{"publish_at": epoch.Add(time.Hour).Format(time.RFC3339)}), http.StatusOK)
	h.notify()
	if posts := h.poster.take(); len(posts) != 0 {
		t.Fatalf("scheduled announcement notified early: %+v", posts)
	}
	h.clock.Advance(2 * time.Hour)
	h.notify()
	posts := h.poster.take()
	if len(posts) != 1 || !strings.Contains(string(posts[0].Body), "Comunicado: Nova região") || strings.Contains(string(posts[0].Body), "**") {
		t.Fatalf("announcement post: %+v", posts)
	}
	h.notify()
	if posts := h.poster.take(); len(posts) != 0 {
		t.Fatalf("announcement notified twice: %+v", posts)
	}

	anon := h.anonymous()
	atom := mustStatus(t, anon.call(http.MethodGet, "/api/v1/feed.atom", nil), http.StatusOK)
	if !strings.Contains(string(atom.body), "<feed xmlns=\"http://www.w3.org/2005/Atom\">") || !strings.Contains(string(atom.body), "Comunicado: Nova região") ||
		!strings.Contains(string(atom.body), publicBaseURL+"/announcements/"+str(a, "id")) {
		t.Fatalf("atom: %s", atom.body)
	}
	rss := mustStatus(t, anon.call(http.MethodGet, "/api/v1/feed.rss", nil), http.StatusOK)
	if !strings.Contains(string(rss.body), `<rss version="2.0">`) || !strings.Contains(string(rss.body), "<item>") {
		t.Fatalf("rss: %s", rss.body)
	}
}

func TestSubscriptionsWithoutEmail(t *testing.T) {
	h := newBareHarnessWith(t, func(o *Options) { o.Mailer = nil })
	anon := h.anonymous()
	opts := mustStatus(t, anon.call(http.MethodGet, "/api/v1/subscriptions/options", nil), http.StatusOK).json(t)
	if opts["email"] != false || opts["webhook"] != false || str(opts, "atom_url") == "" {
		t.Fatalf("options: %v", opts)
	}
	r := mustStatus(t, anon.call(http.MethodPost, "/api/v1/subscriptions", map[string]any{"channel": "email", "email": "a@example.com"}), http.StatusConflict)
	if r.errorCode(t) != "subscriptions_unavailable" {
		t.Fatalf("code = %s", r.errorCode(t))
	}
}
