package social

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"time"
)

const appleIssuer = "https://appleid.apple.com"

// NewApple configures Sign in with Apple. clientID is the Services ID; the
// client secret is a short-lived JWT signed with the key downloaded from the
// Apple Developer portal. Apple posts the callback as a form (form_post),
// which it requires when the e-mail scope is requested.
func NewApple(clientID, teamID, keyID string, key *ecdsa.PrivateKey) *OIDC {
	return &OIDC{
		Key: "apple", Label: "Apple", ClientID: clientID,
		AuthEndpoint:    appleIssuer + "/auth/authorize",
		TokenEndpoint:   appleIssuer + "/auth/token",
		Issuers:         []string{appleIssuer},
		Scopes:          []string{"email"},
		Secret:          func(now time.Time) (string, error) { return appleSecret(clientID, teamID, keyID, key, now) },
		ExtraAuthParams: url.Values{"response_mode": {"form_post"}},
	}
}

// ParseAppleKey reads the PKCS #8 PEM private key (.p8 file) of an Apple
// Sign in with Apple key.
func ParseAppleKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("not a PEM file")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS #8 key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, errors.New("not a P-256 (ES256) key")
	}
	return ec, nil
}

// appleSecret signs the ES256 client secret JWT Apple expects.
func appleSecret(clientID, teamID, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error) {
	enc := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b), err
	}
	header, err := enc(map[string]string{"alg": "ES256", "kid": keyID})
	if err != nil {
		return "", err
	}
	claims, err := enc(map[string]any{"iss": teamID, "sub": clientID, "aud": appleIssuer,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()})
	if err != nil {
		return "", err
	}
	signing := header + "." + claims
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign apple client secret: %w", err)
	}
	// JWS ES256 signatures are r || s, each left-padded to 32 bytes.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
