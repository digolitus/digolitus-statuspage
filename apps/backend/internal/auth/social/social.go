// Package social signs users in through external identity providers
// (Google, Apple, Facebook) with the OAuth 2.0 authorization code flow.
//
// It only proves who the person is at the provider; deciding whether that
// identity may enter the admin area is up to package auth.
//
// ID tokens (Google, Apple) are received directly from the provider's token
// endpoint over TLS, as the confidential client, so their signature is not
// checked (OpenID Connect Core 1.0, section 3.1.3.7, item 6). Issuer,
// audience, expiry and nonce are.
package social

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Identity is the person authenticated by a provider.
type Identity struct {
	// Subject is the provider's stable user ID.
	Subject string
	Email   string
	// EmailVerified reports whether the provider vouches for Email.
	EmailVerified bool
}

// Params ties one authorization request to its callback. State, Nonce and
// Verifier are kept by the browser in a short-lived cookie.
type Params struct {
	State       string
	Nonce       string
	Verifier    string
	RedirectURI string
}

// NewParams generates fresh random values for an authorization request.
func NewParams(redirectURI string) (Params, error) {
	var vals [3]string
	for i := range vals {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return Params{}, fmt.Errorf("random: %w", err)
		}
		vals[i] = base64.RawURLEncoding.EncodeToString(b)
	}
	return Params{State: vals[0], Nonce: vals[1], Verifier: vals[2], RedirectURI: redirectURI}, nil
}

// Provider is an external identity provider.
type Provider interface {
	// ID is the stable key used in URLs and in the database ("google").
	ID() string
	// Name is shown on the login button ("Google").
	Name() string
	// AuthURL is where the browser is sent to sign in.
	AuthURL(p Params) string
	// Exchange trades the authorization code for the person's identity.
	Exchange(ctx context.Context, code string, p Params) (Identity, error)
}

// Error is a failure attributable to the provider or the response it sent
// (as opposed to a local fault). Its message is safe to log.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

func providerErr(format string, args ...any) error {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

// DefaultClient is used when a provider has no Client.
var DefaultClient = &http.Client{Timeout: 10 * time.Second}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// postForm sends a form to a token endpoint and decodes the JSON answer.
func postForm(ctx context.Context, c *http.Client, endpoint string, form url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return do(c, req, dst)
}

func do(c *http.Client, req *http.Request, dst any) error {
	if c == nil {
		c = DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return providerErr("request to %s failed: %v", req.URL.Host, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return providerErr("read response from %s: %v", req.URL.Host, err)
	}
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error any `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return providerErr("%s answered HTTP %d (%v)", req.URL.Host, res.StatusCode, e.Error)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return providerErr("invalid JSON from %s: %v", req.URL.Host, err)
	}
	return nil
}

// flexBool accepts true/false as JSON booleans or strings (Apple sends
// "true").
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `"`) {
	case "true":
		*b = true
	case "false", "null":
		*b = false
	default:
		return errors.New("not a boolean")
	}
	return nil
}

// audience accepts the "aud" claim as a string or an array.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}
