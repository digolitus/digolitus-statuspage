package social

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func idToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	// The signature is not checked (token received from the token endpoint).
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}

// tokenServer answers the token endpoint with body and records the form.
func tokenServer(t *testing.T, body func() any) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		got = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body())
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestGoogleAuthURLAndExchange(t *testing.T) {
	claims := map[string]any{"iss": "https://accounts.google.com", "aud": "client", "sub": "123", "exp": now.Add(time.Hour).Unix(),
		"nonce": "n", "email": "Ana@Example.com", "email_verified": true}
	srv, form := tokenServer(t, func() any { return map[string]string{"id_token": idToken(t, claims)} })
	g := NewGoogle("client", "secret")
	g.TokenEndpoint, g.Now = srv.URL, func() time.Time { return now }
	p := Params{State: "s", Nonce: "n", Verifier: "v", RedirectURI: "https://status.example.com/cb"}

	u, err := url.Parse(g.AuthURL(p))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != "s" || q.Get("nonce") != "n" || q.Get("code_challenge") != challenge("v") ||
		q.Get("code_challenge_method") != "S256" || q.Get("scope") != "openid email" || q.Get("redirect_uri") != p.RedirectURI {
		t.Fatalf("auth URL: %s", u)
	}

	id, err := g.Exchange(context.Background(), "code", p)
	if err != nil {
		t.Fatal(err)
	}
	if id != (Identity{Subject: "123", Email: "Ana@Example.com", EmailVerified: true}) {
		t.Fatalf("identity: %+v", id)
	}
	if form.Get("code") != "code" || form.Get("code_verifier") != "v" || form.Get("client_secret") != "secret" {
		t.Fatalf("token form: %v", form)
	}

	for name, change := range map[string]func(){
		"issuer":   func() { claims["iss"] = "https://evil.example" },
		"audience": func() { claims["aud"] = []string{"other"} },
		"expired":  func() { claims["exp"] = now.Add(-2 * time.Minute).Unix() },
		"nonce":    func() { claims["nonce"] = "replayed" },
	} {
		saved := map[string]any{}
		for k, v := range claims {
			saved[k] = v
		}
		change()
		if _, err := g.Exchange(context.Background(), "code", p); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		claims = saved
	}
}

func TestAppleClientSecretAndStringEmailVerified(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseAppleKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{"iss": appleIssuer, "aud": "com.example.web", "sub": "000.abc", "exp": now.Add(time.Hour).Unix(),
		"nonce": "n", "email": "a@example.com", "email_verified": "true"}
	srv, form := tokenServer(t, func() any { return map[string]string{"id_token": idToken(t, claims)} })
	a := NewApple("com.example.web", "TEAM", "KEY", parsed)
	a.TokenEndpoint, a.Now = srv.URL, func() time.Time { return now }

	if u := a.AuthURL(Params{State: "s", Nonce: "n"}); !strings.Contains(u, "response_mode=form_post") {
		t.Fatalf("auth URL: %s", u)
	}
	id, err := a.Exchange(context.Background(), "code", Params{Nonce: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if !id.EmailVerified || id.Subject != "000.abc" {
		t.Fatalf("identity: %+v", id)
	}

	// The client secret is an ES256 JWT verifiable with the public key.
	parts := strings.Split(form.Get("client_secret"), ".")
	if len(parts) != 3 {
		t.Fatalf("client secret: %q", form.Get("client_secret"))
	}
	var header, body map[string]any
	for i, dst := range []*map[string]any{&header, &body} {
		raw, _ := base64.RawURLEncoding.DecodeString(parts[i])
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatal(err)
		}
	}
	if header["alg"] != "ES256" || header["kid"] != "KEY" || body["iss"] != "TEAM" || body["sub"] != "com.example.web" || body["aud"] != appleIssuer {
		t.Fatalf("client secret header %v claims %v", header, body)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("client secret signature does not verify")
	}
}

func TestParseAppleKeyRejectsOtherKeys(t *testing.T) {
	if _, err := ParseAppleKey([]byte("not pem")); err == nil {
		t.Fatal("expected error")
	}
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	if _, err := ParseAppleKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err == nil {
		t.Fatal("P-384 key should be rejected")
	}
}

func TestFacebookExchange(t *testing.T) {
	var proof, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.PostForm.Get("code") != "code" || r.PostForm.Get("client_secret") != "secret" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
		case "/me":
			proof, auth = r.URL.Query().Get("appsecret_proof"), r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"42","email":"ana@example.com"}`))
		}
	}))
	defer srv.Close()
	f := NewFacebook("app", "secret")
	f.TokenEndpoint, f.MeEndpoint = srv.URL+"/token", srv.URL+"/me"

	id, err := f.Exchange(context.Background(), "code", Params{RedirectURI: "https://status.example.com/cb"})
	if err != nil {
		t.Fatal(err)
	}
	if id != (Identity{Subject: "42", Email: "ana@example.com", EmailVerified: true}) {
		t.Fatalf("identity: %+v", id)
	}
	if auth != "Bearer tok" || len(proof) != 64 {
		t.Fatalf("profile request: auth=%q proof=%q", auth, proof)
	}
	if _, err := f.Exchange(context.Background(), "wrong", Params{}); err == nil {
		t.Fatal("expected error for rejected code")
	}
}
