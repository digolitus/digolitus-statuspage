package social

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
)

// facebookGraphVersion is the Graph API version used. Meta retires versions
// about two years after release; bump it when it nears its end of life.
const facebookGraphVersion = "v23.0"

// Facebook is Facebook Login (plain OAuth 2.0, no ID token): the code is
// exchanged for an access token, then the profile is read from the Graph
// API. Facebook only returns an e-mail address the person has confirmed.
type Facebook struct {
	AppID, AppSecret string
	DialogEndpoint   string
	TokenEndpoint    string
	MeEndpoint       string
	Client           *http.Client
}

// NewFacebook configures Facebook Login.
func NewFacebook(appID, appSecret string) *Facebook {
	return &Facebook{
		AppID: appID, AppSecret: appSecret,
		DialogEndpoint: "https://www.facebook.com/" + facebookGraphVersion + "/dialog/oauth",
		TokenEndpoint:  "https://graph.facebook.com/" + facebookGraphVersion + "/oauth/access_token",
		MeEndpoint:     "https://graph.facebook.com/" + facebookGraphVersion + "/me",
	}
}

func (f *Facebook) ID() string   { return "facebook" }
func (f *Facebook) Name() string { return "Facebook" }

func (f *Facebook) AuthURL(p Params) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {f.AppID},
		"redirect_uri":  {p.RedirectURI},
		"scope":         {"email"},
		"state":         {p.State},
	}
	return f.DialogEndpoint + "?" + q.Encode()
}

func (f *Facebook) Exchange(ctx context.Context, code string, p Params) (Identity, error) {
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := postForm(ctx, f.Client, f.TokenEndpoint, url.Values{
		"client_id": {f.AppID}, "client_secret": {f.AppSecret}, "redirect_uri": {p.RedirectURI}, "code": {code},
	}, &tok); err != nil {
		return Identity{}, err
	}
	if tok.AccessToken == "" {
		return Identity{}, providerErr("facebook: token response without access_token")
	}
	// appsecret_proof proves the call comes from the app's server.
	mac := hmac.New(sha256.New, []byte(f.AppSecret))
	mac.Write([]byte(tok.AccessToken))
	q := url.Values{"fields": {"id,email"}, "appsecret_proof": {hex.EncodeToString(mac.Sum(nil))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.MeEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := do(f.Client, req, &me); err != nil {
		return Identity{}, err
	}
	if me.ID == "" {
		return Identity{}, providerErr("facebook: profile without id")
	}
	return Identity{Subject: me.ID, Email: me.Email, EmailVerified: me.Email != ""}, nil
}
