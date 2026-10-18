package auth

import (
	"strings"
	"testing"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding: %s", h)
	}
	h2, _ := HashPassword("correct horse battery staple")
	if h == h2 {
		t.Fatal("salt must differ between hashes")
	}
	if ok, rehash, err := VerifyPassword(h, "correct horse battery staple"); !ok || rehash || err != nil {
		t.Fatalf("verify: %v %v %v", ok, rehash, err)
	}
	if ok, _, _ := VerifyPassword(h, "correct horse battery stapl"); ok {
		t.Fatal("wrong password accepted")
	}
	weak := strings.Replace(h, "m=19456,t=2", "m=8192,t=1", 1)
	if _, rehash, err := VerifyPassword(weak, "x"); err != nil || !rehash {
		t.Fatalf("weaker parameters must ask for a rehash: %v %v", rehash, err)
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$a$b", strings.Replace(h, "m=19456", "m=99999999", 1)} {
		if _, _, err := VerifyPassword(bad, "x"); err == nil {
			t.Errorf("malformed hash accepted: %q", bad)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	cases := map[string]bool{
		"short":                      false,
		"            ":               false,
		"ana@example.com":            false,
		"a long enough password":     true,
		strings.Repeat("x", 129):     false,
		"senha com acentuação ótima": true,
	}
	for pw, ok := range cases {
		var v apperror.Validator
		CheckPasswordPolicy(&v, "password", pw, "ana@example.com")
		if (v.Err() == nil) != ok {
			t.Errorf("%q: valid=%v, want %v", pw, v.Err() == nil, ok)
		}
	}
}
