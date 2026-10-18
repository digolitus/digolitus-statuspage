package subscription

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/notify"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes subscription endpoints.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts the public endpoints (self-service and feeds).
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/subscriptions/options", h.options)
	rt.Handle(http.MethodPost, "/api/v1/subscriptions", h.subscribe)
	rt.Handle(http.MethodPost, "/api/v1/subscriptions/confirm", h.confirm)
	rt.Handle(http.MethodPost, "/api/v1/subscriptions/manage", h.manage)
	rt.Handle(http.MethodPut, "/api/v1/subscriptions/manage", h.preferences)
	rt.Handle(http.MethodPost, "/api/v1/subscriptions/unsubscribe", h.unsubscribe)
	rt.Handle(http.MethodGet, "/api/v1/feed.atom", h.atom)
	rt.Handle(http.MethodGet, "/api/v1/feed.rss", h.rss)
}

// RegisterAdmin mounts subscriber management.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/subscriptions", admin(h.list))
	rt.Handle(http.MethodPost, "/api/v1/admin/subscriptions", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/subscriptions/{id}", admin(h.get))
	rt.Handle(http.MethodPatch, "/api/v1/admin/subscriptions/{id}", admin(h.update))
	rt.Handle(http.MethodDelete, "/api/v1/admin/subscriptions/{id}", admin(h.remove))
	rt.Handle(http.MethodPost, "/api/v1/admin/subscriptions/{id}/test", admin(h.test))
}

// ChannelOptions tells clients which channels are available.
type ChannelOptions struct {
	Email   bool `json:"email"`
	Webhook bool `json:"webhook"`
	Slack   bool `json:"slack"`
	// SMS is not offered: it needs a paid provider (see docs).
	SMS  bool   `json:"sms"`
	Atom string `json:"atom_url"`
	RSS  string `json:"rss_url"`
}

func (h *Handler) options(w http.ResponseWriter, _ *http.Request) {
	on := h.svc.EmailEnabled()
	httpx.WriteJSON(w, http.StatusOK, ChannelOptions{Email: on, Webhook: on, Slack: on, Atom: "/api/v1/feed.atom", RSS: "/api/v1/feed.rss"})
}

type accepted struct {
	Message string `json:"message"`
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	var in SubscribeInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Subscribe(r.Context(), in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, accepted{Message: "Se os dados estiverem corretos, você receberá um e-mail para confirmar a inscrição."})
}

type tokenInput struct {
	Token string `json:"token"`
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	var in tokenInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.Confirm(r.Context(), in.Token)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (h *Handler) manage(w http.ResponseWriter, r *http.Request) {
	var in tokenInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.Manage(r.Context(), in.Token)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request) {
	var in PreferencesInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.UpdatePreferences(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// unsubscribe accepts the token as a query parameter: mail clients'
// one-click unsubscribe (RFC 8058) POSTs a form body to the URL from the
// List-Unsubscribe header. A JSON body {"token"} is also accepted.
func (h *Handler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var in tokenInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err == nil {
			token = in.Token
		}
	}
	if token == "" {
		httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "token", Message: "Informe o token do link recebido por e-mail."}}})
		return
	}
	if err := h.svc.Unsubscribe(r.Context(), token); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- admin ---

// AdminList is the admin listing with totals per status.
type AdminList struct {
	httpx.List[Subscription]
	Counts map[string]int `json:"counts"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := httpx.NewQuery(values)
	f := AdminFilter{Channel: q.Enum("channel", ChannelEmail, ChannelWebhook, ChannelSlack),
		Status: q.Enum("status", StatusPending, StatusActive, StatusDisabled), Search: strings.TrimSpace(values.Get("q"))}
	page := q.Page()
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if utf8.RuneCountInString(f.Search) > 100 {
		httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "q", Message: "Informe no máximo 100 caracteres."}}})
		return
	}
	items, total, counts, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, AdminList{List: httpx.NewList(items, page, total), Counts: counts})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "subscription")
	if err == nil {
		var s Subscription
		if s, err = get(r.Context(), h.svc.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, s)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in AdminCreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.AdminCreate(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/subscriptions/"+c.ID)
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "subscription")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in AdminPatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := h.svc.AdminUpdate(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "subscription")
	if err == nil {
		err = h.svc.AdminDelete(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "subscription")
	if err == nil {
		err = h.svc.AdminTest(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, accepted{Message: "Mensagem de teste na fila de envio."})
}

// --- feeds ---

type feedItem struct {
	id   int64
	ev   notify.Event
	when time.Time
}

const feedSize = 50

func (h *Handler) feedItems(r *http.Request) ([]feedItem, error) {
	rows, err := h.svc.pool.Query(r.Context(), `SELECT id, payload, created_at FROM notification_events ORDER BY created_at DESC, id DESC LIMIT $1`, feedSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []feedItem
	for rows.Next() {
		var it feedItem
		var raw []byte
		if err := rows.Scan(&it.id, &raw, &it.when); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &it.ev) == nil {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}

// base is the absolute public URL: configured, or derived from the request
// as forwarded by the proxy.
func (h *Handler) base(r *http.Request) string {
	if h.svc.baseURL != "" {
		return h.svc.baseURL
	}
	scheme := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p == "https" || r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func feedText(e notify.Event) string {
	return lines(statusLine(e), componentsLine(e), windowLine(e), e.Message)
}

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Link    atomLink `xml:"link"`
	Summary string   `xml:"summary"`
}

func (h *Handler) atom(w http.ResponseWriter, r *http.Request) {
	items, err := h.feedItems(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	base := h.base(r)
	host := hostOf(base)
	updated := h.svc.now()
	if len(items) > 0 {
		updated = items[0].when
	}
	f := atomFeed{Title: h.svc.siteName + " — atualizações de status", ID: base + "/", Updated: updated.UTC().Format(time.RFC3339),
		Links: []atomLink{{Href: base + "/"}, {Rel: "self", Href: base + "/api/v1/feed.atom"}}}
	for _, it := range items {
		f.Entries = append(f.Entries, atomEntry{Title: eventHeadline(it.ev), ID: fmt.Sprintf("tag:%s,2026:event-%d", host, it.id),
			Updated: it.when.UTC().Format(time.RFC3339), Link: atomLink{Href: base + it.ev.Path}, Summary: feedText(it.ev)})
	}
	writeXML(w, "application/atom+xml; charset=utf-8", f)
}

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func (h *Handler) rss(w http.ResponseWriter, r *http.Request) {
	items, err := h.feedItems(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	base := h.base(r)
	doc := rssDoc{Version: "2.0", Channel: rssChannel{Title: h.svc.siteName + " — atualizações de status", Link: base + "/",
		Description: "Incidentes, manutenções e comunicados publicados."}}
	for _, it := range items {
		doc.Channel.Items = append(doc.Channel.Items, rssItem{Title: eventHeadline(it.ev), Link: base + it.ev.Path,
			GUID: rssGUID{IsPermaLink: "false", Value: fmt.Sprintf("event-%d", it.id)}, PubDate: it.when.UTC().Format(time.RFC1123Z),
			Description: feedText(it.ev)})
	}
	writeXML(w, "application/rss+xml; charset=utf-8", doc)
}

func writeXML(w http.ResponseWriter, contentType string, v any) {
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, "feed error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	// Feeds are public and change slowly: short shared caching is fine.
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(out)
}

func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "localhost"
}
