package subscription

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/notify"
)

// Rendering of notifications per channel. Texts are plain (e-mail) or
// escaped (Slack); nothing from events is interpreted as markup.

func eventHeadline(e notify.Event) string {
	return notify.KindLabel[e.Kind] + ": " + e.Title
}

func statusLine(e notify.Event) string {
	var parts []string
	switch e.ResourceType {
	case "incident":
		if l := notify.IncidentStatusLabel[e.Status]; l != "" {
			parts = append(parts, "Status: "+l)
		}
		if l := notify.ImpactLabel[e.Impact]; l != "" {
			parts = append(parts, "Impacto: "+l)
		}
	case "maintenance":
		if l := notify.ImpactLabel[e.Impact]; l != "" {
			parts = append(parts, "Impacto esperado: "+l)
		}
	}
	return strings.Join(parts, " · ")
}

func componentsLine(e notify.Event) string {
	if len(e.Components) == 0 {
		return ""
	}
	names := make([]string, len(e.Components))
	for i, c := range e.Components {
		names[i] = c.Name
	}
	return "Componentes: " + strings.Join(names, ", ")
}

// windowLine formats a maintenance window in its planning time zone.
func windowLine(e notify.Event) string {
	if e.ScheduledStart == nil || e.ScheduledEnd == nil {
		return ""
	}
	loc, err := time.LoadLocation(e.Timezone)
	if err != nil {
		loc = time.UTC
	}
	const f = "02/01/2006 15:04"
	return fmt.Sprintf("Janela: %s → %s (%s)", e.ScheduledStart.In(loc).Format(f), e.ScheduledEnd.In(loc).Format(f), loc.String())
}

func lines(xs ...string) string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return strings.Join(out, "\n")
}

// eventMail renders an event e-mail.
func (s *Service) eventMail(to string, e notify.Event, manage string) Mail {
	text := lines(eventHeadline(e), statusLine(e), componentsLine(e), windowLine(e)) + "\n\n"
	if e.Message != "" {
		text += e.Message + "\n\n"
	}
	text += "Detalhes: " + s.baseURL + e.Path + "\n\n" +
		"—\nVocê recebe este e-mail porque se inscreveu em " + s.baseURL + ".\n" +
		"Gerenciar ou cancelar a inscrição: " + manage + "\n"
	return Mail{To: to, Subject: "[" + s.siteName + "] " + eventHeadline(e), Text: text, Unsubscribe: s.unsubscribeURL(manage)}
}

func (s *Service) confirmMail(to string, sub Subscription, link string) Mail {
	what := "as atualizações de status de " + s.siteName + " neste e-mail"
	switch sub.Channel {
	case ChannelWebhook:
		what = "as atualizações de status de " + s.siteName + " no webhook " + deref(sub.TargetURL)
	case ChannelSlack:
		what = "as atualizações de status de " + s.siteName + " no Slack (incoming webhook informado)"
	}
	text := "Recebemos um pedido para enviar " + what + ".\n\n" +
		"Para confirmar, abra o link abaixo e clique em \"Confirmar inscrição\":\n" + link + "\n\n" +
		"Se não foi você, ignore este e-mail: nada será enviado sem confirmação.\n"
	return Mail{To: to, Subject: "Confirme sua inscrição — " + s.siteName, Text: text}
}

func (s *Service) manageMail(to string, link string) Mail {
	text := "Você já tem uma inscrição ativa em " + s.siteName + ".\n\n" +
		"Para escolher componentes ou cancelar a inscrição, use o link:\n" + link + "\n\n" +
		"Se não foi você quem pediu, ignore este e-mail.\n"
	return Mail{To: to, Subject: "Gerencie sua inscrição — " + s.siteName, Text: text, Unsubscribe: s.unsubscribeURL(link)}
}

func (s *Service) testMail(to, manage string) Mail {
	return Mail{To: to, Subject: "[" + s.siteName + "] Mensagem de teste",
		Text:        "Esta é uma mensagem de teste enviada por um administrador.\n\nGerenciar ou cancelar: " + manage + "\n",
		Unsubscribe: s.unsubscribeURL(manage)}
}

// unsubscribeURL turns a manage link into the one-click API endpoint.
func (s *Service) unsubscribeURL(manage string) string {
	if i := strings.Index(manage, "?token="); i >= 0 {
		return s.baseURL + "/api/v1/subscriptions/unsubscribe" + manage[i:]
	}
	return ""
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// slackBody renders a Slack incoming-webhook payload.
func (s *Service) slackBody(e notify.Event) []byte {
	text := "*" + slackEscape(eventHeadline(e)) + "*"
	if l := lines(statusLine(e), componentsLine(e), windowLine(e)); l != "" {
		text += "\n" + slackEscape(l)
	}
	if e.Message != "" {
		text += "\n" + slackEscape(e.Message)
	}
	if e.Path != "" && s.baseURL != "" {
		text += "\n<" + s.baseURL + e.Path + "|Ver detalhes>"
	}
	b, _ := json.Marshal(map[string]string{"text": text})
	return b
}

// webhookPayload is the JSON delivered to webhooks.
type webhookPayload struct {
	Event      string       `json:"event"`
	DeliveryID string       `json:"delivery_id"`
	Page       string       `json:"page_url,omitempty"`
	URL        string       `json:"url,omitempty"`
	Data       notify.Event `json:"data"`
}

// webhookRequest renders the body and signed headers. The signature is
// HMAC-SHA256(secret, timestamp + "." + body), hex, so receivers can verify
// origin and reject replays.
func (s *Service) webhookRequest(secret string, deliveryID int64, e notify.Event, now time.Time) ([]byte, map[string]string) {
	p := webhookPayload{Event: e.Kind, DeliveryID: strconv.FormatInt(deliveryID, 10), Page: s.baseURL, Data: e}
	if e.Path != "" && s.baseURL != "" {
		p.URL = s.baseURL + e.Path
	}
	body, _ := json.Marshal(p)
	ts := strconv.FormatInt(now.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return body, map[string]string{
		"X-Digolitus-Event":     e.Kind,
		"X-Digolitus-Delivery":  p.DeliveryID,
		"X-Digolitus-Timestamp": ts,
		"X-Digolitus-Signature": "sha256=" + hex.EncodeToString(m.Sum(nil)),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
