package monitoring

import (
	"encoding/json"
	"errors"
	"mime"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Signal kinds.
const (
	KindDown    = "down"
	KindUp      = "up"
	KindIgnored = "ignored"
	KindInvalid = "invalid"
)

// Signal is a provider alert translated to the platform's vocabulary.
type Signal struct {
	ExternalID string
	Name       string
	// Kind is KindDown, KindUp or KindIgnored (an alert we understand but
	// that says nothing about availability, e.g. an SSL expiry warning).
	Kind   string
	Detail string
}

// errPayload is returned when a payload cannot be translated; its message
// is stored with the event and shown to operators.
type errPayload struct{ msg string }

func (e *errPayload) Error() string { return e.msg }

// Fields flattens a webhook request into lower-cased keys: the query string
// (minus the token), then a JSON object or form body. Tools differ in how
// they send data, and operators paste templates, so both are accepted.
func Fields(query url.Values, contentType string, body []byte) (map[string]string, error) {
	out := map[string]string{}
	for k, vs := range query {
		if k == "token" || len(vs) == 0 {
			continue
		}
		out[strings.ToLower(k)] = vs[0]
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return out, nil
	}
	mt, _, _ := mime.ParseMediaType(contentType)
	if mt == "application/x-www-form-urlencoded" {
		vals, err := url.ParseQuery(trimmed)
		if err != nil {
			return nil, &errPayload{"corpo de formulário inválido"}
		}
		for k, vs := range vals {
			if len(vs) > 0 {
				out[strings.ToLower(k)] = vs[0]
			}
		}
		return out, nil
	}
	// Anything else must be a JSON object: some tools send JSON as text/plain.
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, &errPayload{"o corpo não é um objeto JSON"}
	}
	for k, v := range obj {
		switch t := v.(type) {
		case string:
			out[strings.ToLower(k)] = t
		case float64:
			out[strings.ToLower(k)] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			out[strings.ToLower(k)] = strconv.FormatBool(t)
		}
	}
	return out, nil
}

func first(f map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(f[k]); v != "" {
			return v
		}
	}
	return ""
}

// unfilled reports a template placeholder the tool did not replace, e.g. a
// UptimeRobot "*monitorID*" or a Healthchecks "$CODE" sent from a test.
func unfilled(v string) bool {
	return (strings.HasPrefix(v, "*") && strings.HasSuffix(v, "*")) || strings.HasPrefix(v, "$")
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// Parse translates the flattened fields of a provider's webhook.
func Parse(provider string, f map[string]string) (Signal, error) {
	var s Signal
	switch provider {
	case ProviderUptimeRobot:
		s = Signal{
			ExternalID: first(f, "monitorid", "monitor_id", "id"),
			Name:       first(f, "monitorfriendlyname", "monitor_name", "name"),
			Detail:     first(f, "alertdetails", "details"),
		}
		alert := strings.ToLower(first(f, "alerttype", "alert_type", "alerttypefriendlyname", "alert"))
		switch alert {
		case "1", "down":
			s.Kind = KindDown
		case "2", "up":
			s.Kind = KindUp
		case "":
			return s, &errPayload{"campo alertType ausente"}
		default:
			s.Kind = KindIgnored
			s.Detail = clip("alerta "+alert+" não altera o status", 500)
		}
	case ProviderHealthchecks:
		s = Signal{
			ExternalID: first(f, "check_id", "code", "uuid", "id"),
			Name:       first(f, "name", "check_name"),
			Detail:     first(f, "detail", "details"),
		}
		switch strings.ToLower(first(f, "status", "state")) {
		case "down":
			s.Kind = KindDown
		case "up":
			s.Kind = KindUp
		case "":
			return s, &errPayload{"campo status ausente"}
		default:
			s.Kind = KindIgnored
			s.Detail = "status desconhecido"
		}
	default:
		return s, errors.New("unknown provider")
	}
	if s.ExternalID == "" {
		return s, &errPayload{"identificador do monitor ausente"}
	}
	if unfilled(s.ExternalID) {
		return s, &errPayload{"o modelo não foi preenchido pela ferramenta (mensagem de teste?)"}
	}
	if utf8.RuneCountInString(s.ExternalID) > 128 {
		return s, &errPayload{"identificador do monitor muito longo"}
	}
	if unfilled(s.Name) {
		s.Name = ""
	}
	s.Name = clip(s.Name, 255)
	s.Detail = clip(s.Detail, 500)
	return s, nil
}
