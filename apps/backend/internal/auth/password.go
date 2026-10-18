package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

// Argon2id parameters (OWASP minimum recommendation: 19 MiB, t=2, p=1).
// They are stored in each hash, so they can be raised later; old hashes are
// upgraded on the next successful login.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// Password length bounds. The upper bound limits hashing cost.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// hashSlots bounds concurrent hashing so login bursts cannot exhaust memory
// (each hash uses argonMemory KiB).
var hashSlots = make(chan struct{}, 4)

// HashPassword returns an encoded Argon2id hash (PHC string format).
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hashSlots <- struct{}{}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	<-hashSlots
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

var errBadHash = errors.New("unsupported password hash")

// VerifyPassword checks password against an encoded hash in constant time.
// needsRehash reports parameters weaker than the current ones.
func VerifyPassword(encoded, password string) (ok, needsRehash bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, false, errBadHash
	}
	var version int
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, false, errBadHash
	}
	if memory == 0 || memory > 1<<20 || iterations == 0 || iterations > 10 || threads == 0 {
		return false, false, errBadHash
	}
	b64 := base64.RawStdEncoding
	salt, err1 := b64.DecodeString(parts[4])
	want, err2 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 || len(want) > 64 {
		return false, false, errBadHash
	}
	hashSlots <- struct{}{}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	<-hashSlots
	ok = subtle.ConstantTimeCompare(got, want) == 1
	return ok, memory < argonMemory || iterations < argonTime, nil
}

// dummyHash is verified when a login names an unknown user, so response
// time does not reveal which e-mails exist. Computed on first use.
var dummyHash = sync.OnceValue(func() string {
	h, err := HashPassword("not-a-real-password-for-timing")
	if err != nil {
		panic(err)
	}
	return h
})

// CheckPasswordPolicy validates a new password.
func CheckPasswordPolicy(v *apperror.Validator, field, password, email string) {
	n := utf8.RuneCountInString(password)
	switch {
	case n < MinPasswordLen || n > MaxPasswordLen:
		v.Add(field, fmt.Sprintf("Use de %d a %d caracteres. Frases longas são mais seguras e fáceis de lembrar.", MinPasswordLen, MaxPasswordLen))
	case strings.TrimSpace(password) == "":
		v.Add(field, "A senha não pode ser só espaços.")
	case email != "" && strings.EqualFold(strings.TrimSpace(password), strings.TrimSpace(email)):
		v.Add(field, "A senha não pode ser igual ao e-mail.")
	}
}
