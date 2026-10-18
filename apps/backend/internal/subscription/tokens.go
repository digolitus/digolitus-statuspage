package subscription

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Link tokens identify a subscription in e-mailed links without storing
// them: token = base64url(uuid || purpose || HMAC-SHA256(key, uuid||purpose)[:20]).
// The key is generated once and kept in app_secrets. A token stops working
// when its subscription is deleted (unsubscribe).

// Token purposes.
const (
	purposeConfirm byte = 'c'
	purposeManage  byte = 'm'
)

const tokenKeyName = "subscription_link_key"

type tokenKey struct {
	once sync.Mutex
	key  []byte
}

func (k *tokenKey) get(ctx context.Context, q database.Querier) ([]byte, error) {
	k.once.Lock()
	defer k.once.Unlock()
	if k.key != nil {
		return k.key, nil
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	if _, err := q.Exec(ctx, "INSERT INTO app_secrets (name, value) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING", tokenKeyName, fresh); err != nil {
		return nil, fmt.Errorf("store token key: %w", err)
	}
	var key []byte
	if err := q.QueryRow(ctx, "SELECT value FROM app_secrets WHERE name = $1", tokenKeyName).Scan(&key); err != nil {
		return nil, fmt.Errorf("load token key: %w", err)
	}
	k.key = key
	return key, nil
}

func uuidBytes(id string) ([]byte, bool) {
	b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return b, err == nil && len(b) == 16
}

func formatUUID(b []byte) string {
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func mac(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)[:20]
}

func sign(key []byte, id string, purpose byte) string {
	b, ok := uuidBytes(id)
	if !ok {
		return ""
	}
	data := append(b, purpose)
	return base64.RawURLEncoding.EncodeToString(append(data, mac(key, data)...))
}

// verify returns the subscription ID of a valid token for purpose.
func verify(key []byte, token string, purpose byte) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(raw) != 16+1+20 || raw[16] != purpose {
		return "", false
	}
	if !hmac.Equal(raw[17:], mac(key, raw[:17])) {
		return "", false
	}
	return formatUUID(raw[:16]), true
}
