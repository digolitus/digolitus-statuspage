package subscription

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/safehttp"
)

// Mail is a plain-text message to one recipient.
type Mail struct {
	To      string
	Subject string
	Text    string
	// Unsubscribe is the one-click URL (RFC 8058); empty for confirmations.
	Unsubscribe string
}

// Mailer sends e-mail.
type Mailer interface {
	Send(ctx context.Context, m Mail) error
}

// Poster delivers HTTP POSTs to webhook and Slack endpoints.
type Poster interface {
	Post(ctx context.Context, url string, body []byte, headers map[string]string) error
}

// SMTPConfig configures SMTPMailer.
type SMTPConfig struct {
	Host, Username, Password, From string
	Port                           int
	// TLS: "starttls", "tls" (implicit) or "none" (local servers only).
	TLS string
}

// SMTPMailer sends through an SMTP relay.
type SMTPMailer struct {
	cfg  SMTPConfig
	from *mail.Address
}

// NewSMTPMailer validates the sender address.
func NewSMTPMailer(cfg SMTPConfig) (*SMTPMailer, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("SMTP_FROM: %w", err)
	}
	return &SMTPMailer{cfg: cfg, from: from}, nil
}

// headerValue removes line breaks so user-provided text (titles) can never
// inject headers.
func headerValue(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// Build renders the RFC 5322 message (exported for tests).
func (s *SMTPMailer) Build(m Mail, now time.Time) ([]byte, error) {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return nil, fmt.Errorf("recipient: %w", err)
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	domain := s.from.Address[strings.LastIndexByte(s.from.Address, '@')+1:]
	var b bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", s.from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", headerValue(m.Subject)))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id[:])+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Content-Type", "text/plain; charset=utf-8")
	h("Content-Transfer-Encoding", "quoted-printable")
	h("Auto-Submitted", "auto-generated")
	if m.Unsubscribe != "" {
		h("List-Unsubscribe", "<"+headerValue(m.Unsubscribe)+">")
		h("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(m.Text, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Send delivers m, bounded by ctx.
func (s *SMTPMailer) Send(ctx context.Context, m Mail) error {
	msg, err := s.Build(m, time.Now())
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	if s.cfg.TLS == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer func() { _ = c.Close() }()
	if s.cfg.TLS == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	to, _ := mail.ParseAddress(m.To)
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data end: %w", err)
	}
	return c.Quit()
}

// HTTPPoster posts to user-chosen URLs through the SSRF-safe client
// (public addresses only unless allowPrivate) without following redirects.
type HTTPPoster struct {
	client *http.Client
}

// NewHTTPPoster builds the poster.
func NewHTTPPoster(timeout time.Duration, allowPrivate bool) *HTTPPoster {
	return &HTTPPoster{client: safehttp.NewClient(safehttp.Options{Timeout: timeout, AllowPrivate: allowPrivate, MaxRedirects: 0})}
}

// Post sends body; any non-2xx response is an error.
func (p *HTTPPoster) Post(ctx context.Context, url string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DigolitusStatusPage/1 (+notifications)")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) && strings.Contains(err.Error(), "not public") {
			return errors.New("endereço de destino não é público")
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("endpoint respondeu HTTP %d", resp.StatusCode)
	}
	return nil
}
