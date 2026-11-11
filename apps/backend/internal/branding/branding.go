// Package branding stores the visual identity of the public page and the
// login screen: names, texts, font, colors, logo and favicon (ADR 0016).
//
// Settings are one validated document. Images are either uploaded (stored in
// the database and served by the API) or referenced by an https URL on a host
// the operator allowed in BRANDING_IMAGE_HOSTS, which the frontend's CSP
// allows too.
package branding

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

// Fonts the frontend bundles. "system" uses the visitor's system font.
var Fonts = []string{"inter", "ibm-plex-sans", "source-sans-3", "nunito-sans", "roboto", "merriweather-sans", "system"}

// Image kinds.
const (
	KindLogo    = "logo"
	KindFavicon = "favicon"
)

// Image sources.
const (
	SourceNone   = "none"
	SourceUpload = "upload"
	SourceURL    = "url"
)

var sources = []string{SourceNone, SourceUpload, SourceURL}

// MaxImageBytes is the upload limit per kind.
var MaxImageBytes = map[string]int{KindLogo: 512 << 10, KindFavicon: 128 << 10}

// Colors of the brand. Status colors are fixed and not part of it.
type Colors struct {
	Primary          string `json:"primary"`
	HeaderBackground string `json:"header_background"`
	HeaderText       string `json:"header_text"`
	PageBackground   string `json:"page_background"`
}

// ImageChoice says where an image comes from.
type ImageChoice struct {
	Source string `json:"source"`
	// URL is set only when Source is "url".
	URL *string `json:"url"`
}

// Settings is the stored document and the body of PUT /admin/branding.
type Settings struct {
	Title             string      `json:"title"`
	Subtitle          string      `json:"subtitle"`
	Description       string      `json:"description"`
	LoginTitle        string      `json:"login_title"`
	LoginMessage      string      `json:"login_message"`
	FooterText        string      `json:"footer_text"`
	FooterLinkLabel   string      `json:"footer_link_label"`
	FooterLinkURL     string      `json:"footer_link_url"`
	Font              string      `json:"font"`
	Colors            Colors      `json:"colors"`
	ShowTitleWithLogo bool        `json:"show_title_with_logo"`
	Logo              ImageChoice `json:"logo"`
	Favicon           ImageChoice `json:"favicon"`
}

// Defaults returns the identity used before anyone saves one: the product's
// current look, with the configured SITE_NAME as title.
func Defaults(siteName string) Settings {
	if strings.TrimSpace(siteName) == "" {
		siteName = "Digolitus Status Page"
	}
	return Settings{
		Title:             siteName,
		LoginTitle:        "Entrar na administração",
		FooterText:        "Status informado pela equipe de operações.",
		Font:              "inter",
		Colors:            Colors{Primary: "#197552", HeaderBackground: "#ffffff", HeaderText: "#17242c", PageBackground: "#f6f8f9"},
		ShowTitleWithLogo: true,
		Logo:              ImageChoice{Source: SourceNone},
		Favicon:           ImageChoice{Source: SourceNone},
	}
}

// Fixed text colors of the public page, used by the contrast checks.
const (
	buttonText = "#ffffff"
	mutedText  = "#5d6b74"
)

// MinContrast is the WCAG AA ratio for normal text.
const MinContrast = 4.5

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalize trims texts and lower-cases colors and hosts in place.
func (s *Settings) normalize() {
	for _, p := range []*string{&s.Title, &s.Subtitle, &s.Description, &s.LoginTitle, &s.LoginMessage, &s.FooterText, &s.FooterLinkLabel, &s.FooterLinkURL} {
		*p = strings.TrimSpace(*p)
	}
	for _, p := range []*string{&s.Colors.Primary, &s.Colors.HeaderBackground, &s.Colors.HeaderText, &s.Colors.PageBackground} {
		*p = strings.ToLower(strings.TrimSpace(*p))
	}
	for _, c := range []*ImageChoice{&s.Logo, &s.Favicon} {
		if c.URL != nil {
			u := strings.TrimSpace(*c.URL)
			c.URL = &u
		}
		if c.Source != SourceURL {
			c.URL = nil
		}
	}
}

// validate checks the settings. hasUpload reports whether an image of the
// kind was uploaded; allowedHosts are the hosts accepted for image URLs.
func (s *Settings) validate(hasUpload func(kind string) bool, allowedHosts []string) error {
	var v apperror.Validator
	text := func(field, value string, minLen, maxLen int, multiline bool) {
		n := utf8.RuneCountInString(value)
		switch {
		case minLen > 0 && (n < minLen || n > maxLen):
			v.Add(field, fmt.Sprintf("Informe um texto com %d a %d caracteres.", minLen, maxLen))
		case n > maxLen:
			v.Add(field, fmt.Sprintf("Use no máximo %d caracteres.", maxLen))
		case hasControl(value, multiline):
			v.Add(field, "Remova quebras de linha e caracteres de controle.")
		}
	}
	text("title", s.Title, 1, 60, false)
	text("subtitle", s.Subtitle, 0, 80, false)
	text("description", s.Description, 0, 160, false)
	text("login_title", s.LoginTitle, 1, 60, false)
	text("login_message", s.LoginMessage, 0, 200, false)
	text("footer_text", s.FooterText, 0, 280, true)
	text("footer_link_label", s.FooterLinkLabel, 0, 40, false)
	switch {
	case s.FooterLinkURL == "" && s.FooterLinkLabel == "":
	case s.FooterLinkURL == "" || s.FooterLinkLabel == "":
		v.Add("footer_link_url", "Informe o texto e o endereço do link do rodapé, ou deixe os dois vazios.")
	case len(s.FooterLinkURL) > 300 || !isWebURL(s.FooterLinkURL):
		v.Add("footer_link_url", "Informe um endereço http:// ou https:// com até 300 caracteres.")
	}
	v.Check(slices.Contains(Fonts, s.Font), "font", "Fonte inválida. Valores aceitos: "+strings.Join(Fonts, ", ")+".")

	colors := map[string]string{"colors.primary": s.Colors.Primary, "colors.header_background": s.Colors.HeaderBackground,
		"colors.header_text": s.Colors.HeaderText, "colors.page_background": s.Colors.PageBackground}
	valid := true
	for _, field := range []string{"colors.primary", "colors.header_background", "colors.header_text", "colors.page_background"} {
		if !hexColor.MatchString(colors[field]) {
			v.Add(field, "Informe a cor no formato #RRGGBB.")
			valid = false
		}
	}
	if valid {
		if r := Contrast(s.Colors.Primary, buttonText); r < MinContrast {
			v.Add("colors.primary", contrastMessage("o texto branco dos botões", r))
		}
		if r := Contrast(s.Colors.HeaderText, s.Colors.HeaderBackground); r < MinContrast {
			v.Add("colors.header_text", contrastMessage("o fundo do cabeçalho", r))
		}
		if r := Contrast(s.Colors.PageBackground, mutedText); r < MinContrast {
			v.Add("colors.page_background", contrastMessage("o texto secundário da página", r)+" Use um fundo claro.")
		}
	}

	for kind, c := range map[string]ImageChoice{KindLogo: s.Logo, KindFavicon: s.Favicon} {
		switch c.Source {
		case SourceNone:
		case SourceUpload:
			v.Check(hasUpload(kind), kind+".source", "Envie a imagem antes de escolher esta opção.")
		case SourceURL:
			if c.URL == nil || *c.URL == "" {
				v.Add(kind+".url", "Informe a URL da imagem.")
			} else if msg := checkImageURL(*c.URL, allowedHosts); msg != "" {
				v.Add(kind+".url", msg)
			}
		default:
			v.Add(kind+".source", "Origem inválida. Valores aceitos: "+strings.Join(sources, ", ")+".")
		}
	}
	return v.Err()
}

func contrastMessage(against string, ratio float64) string {
	return fmt.Sprintf("Contraste de %s:1 com %s; o mínimo é 4,5:1.", strings.Replace(strconv.FormatFloat(ratio, 'f', 1, 64), ".", ",", 1), against)
}

func hasControl(s string, allowNewline bool) bool {
	for _, r := range s {
		if r == '\n' && allowNewline {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func isWebURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

// checkImageURL returns an error message, or "" when raw is an https URL on
// an allowed host (default port, no credentials).
func checkImageURL(raw string, allowedHosts []string) string {
	if len(raw) > 500 {
		return "Use uma URL com até 500 caracteres."
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Port() != "" {
		return "Informe uma URL https:// completa, sem porta e sem usuário."
	}
	if len(allowedHosts) == 0 {
		return "Nenhum domínio está liberado para imagens (BRANDING_IMAGE_HOSTS). Envie o arquivo."
	}
	if !slices.Contains(allowedHosts, strings.ToLower(u.Hostname())) {
		return "O domínio " + u.Hostname() + " não está liberado. Domínios aceitos: " + strings.Join(allowedHosts, ", ") + "."
	}
	return ""
}

// Contrast returns the WCAG contrast ratio of two #rrggbb colors.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	ch := func(i int) float64 {
		n, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(n) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(1) + 0.7152*ch(3) + 0.0722*ch(5)
}

var hostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// ParseHosts parses BRANDING_IMAGE_HOSTS: a comma-separated list of exact
// host names (no scheme, port, path or wildcard).
func ParseHosts(raw string) ([]string, error) {
	hosts := []string{}
	for _, h := range strings.Split(raw, ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if !hostPattern.MatchString(h) {
			return nil, fmt.Errorf("BRANDING_IMAGE_HOSTS: %q is not a host name (use e.g. cdn.example.com, without https:// or wildcards)", h)
		}
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}
