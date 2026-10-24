package social

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// clockSkew tolerates small clock differences when checking "exp".
const clockSkew = time.Minute

// OIDC is an OpenID Connect provider using the authorization code flow.
type OIDC struct {
	Key, Label    string
	ClientID      string
	AuthEndpoint  string
	TokenEndpoint string
	// Issuers lists the accepted "iss" values.
	Issuers []string
	Scopes  []string
	// Secret returns the client secret (Apple signs a fresh one).
	Secret func(now time.Time) (string, error)
	// PKCE adds an S256 code challenge.
	PKCE bool
	// ExtraAuthParams are added to the authorization URL.
	ExtraAuthParams url.Values
	Client          *http.Client
	Now             func() time.Time
}

// NewGoogle configures Sign in with Google.
func NewGoogle(clientID, clientSecret string) *OIDC {
	return &OIDC{
		Key: "google", Label: "Google", ClientID: clientID,
		AuthEndpoint:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint: "https://oauth2.googleapis.com/token",
		Issuers:       []string{"https://accounts.google.com", "accounts.google.com"},
		Scopes:        []string{"openid", "email"},
		Secret:        func(time.Time) (string, error) { return clientSecret, nil },
		PKCE:          true,
		// Always show the account chooser: the admin may use several.
		ExtraAuthParams: url.Values{"prompt": {"select_account"}},
	}
}

func (o *OIDC) ID() string   { return o.Key }
func (o *OIDC) Name() string { return o.Label }

func (o *OIDC) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *OIDC) AuthURL(p Params) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {o.ClientID},
		"redirect_uri":  {p.RedirectURI},
		"scope":         {strings.Join(o.Scopes, " ")},
		"state":         {p.State},
		"nonce":         {p.Nonce},
	}
	if o.PKCE {
		q.Set("code_challenge", challenge(p.Verifier))
		q.Set("code_challenge_method", "S256")
	}
	for k, v := range o.ExtraAuthParams {
		q[k] = v
	}
	return o.AuthEndpoint + "?" + q.Encode()
}

type idClaims struct {
	Iss           string   `json:"iss"`
	Sub           string   `json:"sub"`
	Aud           audience `json:"aud"`
	Exp           int64    `json:"exp"`
	Nonce         string   `json:"nonce"`
	Email         string   `json:"email"`
	EmailVerified flexBool `json:"email_verified"`
}

func (o *OIDC) Exchange(ctx context.Context, code string, p Params) (Identity, error) {
	now := o.now()
	secret, err := o.Secret(now)
	if err != nil {
		return Identity{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.RedirectURI},
		"client_id":     {o.ClientID},
		"client_secret": {secret},
	}
	if o.PKCE {
		form.Set("code_verifier", p.Verifier)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := postForm(ctx, o.Client, o.TokenEndpoint, form, &tok); err != nil {
		return Identity{}, err
	}
	parts := strings.Split(tok.IDToken, ".")
	if len(parts) != 3 {
		return Identity{}, providerErr("%s: token response without a valid id_token", o.Key)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, providerErr("%s: undecodable id_token", o.Key)
	}
	var c idClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Identity{}, providerErr("%s: invalid id_token claims: %v", o.Key, err)
	}
	switch {
	case !slices.Contains(o.Issuers, c.Iss):
		return Identity{}, providerErr("%s: unexpected issuer %q", o.Key, c.Iss)
	case !slices.Contains(c.Aud, o.ClientID):
		return Identity{}, providerErr("%s: id_token issued for another client", o.Key)
	case now.After(time.Unix(c.Exp, 0).Add(clockSkew)):
		return Identity{}, providerErr("%s: expired id_token", o.Key)
	case c.Nonce != p.Nonce:
		return Identity{}, providerErr("%s: id_token nonce mismatch", o.Key)
	case c.Sub == "":
		return Identity{}, providerErr("%s: id_token without subject", o.Key)
	}
	return Identity{Subject: c.Sub, Email: c.Email, EmailVerified: bool(c.EmailVerified)}, nil
}
