package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/branding"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 25, G: 117, B: 82, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func brandingBody(over map[string]any) map[string]any {
	b := map[string]any{
		"title": "Acme Status", "subtitle": "Serviços da Acme", "description": "Status dos serviços da Acme.",
		"login_title": "Entrar", "login_message": "Acesso restrito.", "footer_text": "Equipe de operações Acme.",
		"footer_link_label": "Suporte", "footer_link_url": "https://acme.example.com/suporte", "font": "ibm-plex-sans",
		"colors":               map[string]any{"primary": "#1F5FA8", "header_background": "#0f2a47", "header_text": "#ffffff", "page_background": "#f4f7fb"},
		"show_title_with_logo": true,
		"logo":                 map[string]any{"source": "none", "url": nil},
		"favicon":              map[string]any{"source": "none", "url": nil},
	}
	for k, v := range over {
		b[k] = v
	}
	return b
}

func TestBrandingDefaultsAndUpdate(t *testing.T) {
	h := newHarnessWith(t, func(o *Options) { o.BrandingImageHosts = []string{"cdn.acme.example.com"} })
	pub := h.anonymous()

	got := mustStatus(t, pub.call(http.MethodGet, "/api/v1/branding", nil), http.StatusOK).json(t)
	if str(got, "title") != "Status Teste" || got["updated_at"] != nil || got["logo_url"] != nil || str(got, "font") != "inter" {
		t.Fatalf("defaults must use SITE_NAME and the current look: %v", got)
	}

	saved := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/branding", brandingBody(map[string]any{
		"logo": map[string]any{"source": "url", "url": "https://cdn.acme.example.com/logo.png"},
	})), http.StatusOK).json(t)
	if str(obj(saved, "colors"), "primary") != "#1f5fa8" || saved["updated_at"] == nil || len(list(saved, "allowed_image_hosts")) != 1 {
		t.Fatalf("saved branding: %v", saved)
	}
	got = mustStatus(t, pub.call(http.MethodGet, "/api/v1/branding", nil), http.StatusOK).json(t)
	if str(got, "title") != "Acme Status" || str(got, "logo_url") != "https://cdn.acme.example.com/logo.png" ||
		str(obj(got, "footer_link"), "label") != "Suporte" {
		t.Fatalf("public branding: %v", got)
	}

	// Feeds and e-mails use the configured title instead of SITE_NAME.
	feed := mustStatus(t, pub.call(http.MethodGet, "/api/v1/feed.atom", nil), http.StatusOK)
	if !bytes.Contains(feed.body, []byte("<title>Acme Status — atualizações de status</title>")) {
		t.Fatalf("feed title: %s", feed.body)
	}

	ev := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=branding", nil), http.StatusOK).json(t)
	if total(ev) != 1 || str(list(ev, "data")[0].(map[string]any), "action") != "updated" {
		t.Fatalf("branding change must be audited: %v", ev)
	}
}

func TestBrandingValidation(t *testing.T) {
	h := newHarnessWith(t, func(o *Options) { o.BrandingImageHosts = []string{"cdn.acme.example.com"} })
	cases := map[string]struct {
		over  map[string]any
		field string
	}{
		"low contrast button":  {map[string]any{"colors": map[string]any{"primary": "#9fd3b8", "header_background": "#ffffff", "header_text": "#17242c", "page_background": "#f6f8f9"}}, "colors.primary"},
		"low contrast header":  {map[string]any{"colors": map[string]any{"primary": "#197552", "header_background": "#ffffff", "header_text": "#dddddd", "page_background": "#f6f8f9"}}, "colors.header_text"},
		"dark page":            {map[string]any{"colors": map[string]any{"primary": "#197552", "header_background": "#ffffff", "header_text": "#17242c", "page_background": "#202020"}}, "colors.page_background"},
		"host not allowed":     {map[string]any{"logo": map[string]any{"source": "url", "url": "https://evil.example.net/x.png"}}, "logo.url"},
		"http image":           {map[string]any{"favicon": map[string]any{"source": "url", "url": "http://cdn.acme.example.com/f.png"}}, "favicon.url"},
		"upload without image": {map[string]any{"logo": map[string]any{"source": "upload", "url": nil}}, "logo.source"},
		"script footer link":   {map[string]any{"footer_link_url": "javascript:alert(1)"}, "footer_link_url"},
		"label without url":    {map[string]any{"footer_link_url": ""}, "footer_link_url"},
		"newline in title":     {map[string]any{"title": "a\nb"}, "title"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := mustStatus(t, h.with(t).call(http.MethodPut, "/api/v1/admin/branding", brandingBody(c.over), invalidRequest()), http.StatusUnprocessableEntity)
			found := false
			for _, d := range list(obj(r.json(t), "error"), "details") {
				if str(d.(map[string]any), "field") == c.field {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected an error on %s: %s", c.field, r.body)
			}
		})
	}
}

func TestBrandingImages(t *testing.T) {
	h := newHarness(t)
	logo := pngBytes(t, 120, 32)

	r := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/branding/images/logo", nil, rawBody(logo, "image/png")), http.StatusOK).json(t)
	up := obj(r, "logo_upload")
	if str(obj(r, "logo"), "source") != "upload" || str(up, "content_type") != "image/png" {
		t.Fatalf("upload must be selected: %v", r)
	}
	pub := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/branding", nil), http.StatusOK).json(t)
	if str(pub, "logo_url") != str(up, "url") {
		t.Fatalf("public logo_url: %v", pub)
	}

	img := mustStatus(t, h.anonymous().call(http.MethodGet, str(up, "url"), nil), http.StatusOK)
	if !bytes.Equal(img.body, logo) || img.header.Get("Content-Type") != "image/png" ||
		img.header.Get("Cache-Control") != "public, max-age=31536000, immutable" || img.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("served image: %v", img.header)
	}
	mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/branding/images/logo", nil, withHeader("If-None-Match", img.header.Get("ETag"))), http.StatusNotModified)

	// Content decides the type: SVG/HTML is refused whatever the header says.
	r2 := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/branding/images/favicon", nil,
		rawBody([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "image/png")), http.StatusUnprocessableEntity)
	if r2.errorCode(t) != "image_type_not_allowed" {
		t.Fatalf("svg must be refused: %s", r2.body)
	}
	// JPEG is a logo format, not a favicon one; size limits per kind.
	mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/branding/images/favicon", nil,
		rawBody(make([]byte, branding.MaxImageBytes[branding.KindFavicon]+1), "image/png")), http.StatusRequestEntityTooLarge)
	mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/branding/images/banner", nil, rawBody(logo, "image/png")), http.StatusNotFound)

	// Only branding.manage changes it; everyone signed in can read it.
	viewer := h.userWith("Leitor")
	mustStatus(t, viewer.call(http.MethodGet, "/api/v1/admin/branding", nil), http.StatusOK)
	mustStatus(t, viewer.call(http.MethodDelete, "/api/v1/admin/branding/images/logo", nil), http.StatusForbidden)
	designer := h.userWith("Designer", "branding.manage")
	r = mustStatus(t, designer.call(http.MethodDelete, "/api/v1/admin/branding/images/logo", nil), http.StatusOK).json(t)
	if str(obj(r, "logo"), "source") != "none" || r["logo_upload"] != nil {
		t.Fatalf("removing the image in use falls back to none: %v", r)
	}
	mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/branding/images/logo", nil), http.StatusNotFound)
}
