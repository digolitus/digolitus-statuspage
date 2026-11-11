package branding

import (
	"math"
	"testing"
)

func TestContrast(t *testing.T) {
	if r := Contrast("#000000", "#ffffff"); math.Abs(r-21) > 0.01 {
		t.Fatalf("black on white = %v", r)
	}
	if r := Contrast("#197552", "#ffffff"); r < MinContrast {
		t.Fatalf("default primary must pass its own check: %v", r)
	}
}

func TestDefaultsAreValid(t *testing.T) {
	d := Defaults("")
	d.normalize()
	if err := d.validate(func(string) bool { return false }, nil); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if d.Title != "Digolitus Status Page" {
		t.Fatalf("fallback title: %q", d.Title)
	}
}

func TestParseHosts(t *testing.T) {
	got, err := ParseHosts(" CDN.Example.com, example.com,,cdn.example.com ")
	if err != nil || len(got) != 2 || got[0] != "cdn.example.com" {
		t.Fatalf("ParseHosts: %v %v", got, err)
	}
	for _, bad := range []string{"https://cdn.example.com", "*.example.com", "cdn.example.com:8443", "localhost", "a b.com"} {
		if _, err := ParseHosts(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestCheckImageURL(t *testing.T) {
	hosts := []string{"cdn.example.com"}
	ok := []string{"https://cdn.example.com/logo.png", "https://CDN.example.com/a?b=c"}
	bad := []string{"http://cdn.example.com/x.png", "https://user@cdn.example.com/x", "https://cdn.example.com:444/x", "https://other.com/x", "cdn.example.com/x", "data:image/png;base64,AAAA"}
	for _, u := range ok {
		if msg := checkImageURL(u, hosts); msg != "" {
			t.Errorf("%s: %s", u, msg)
		}
	}
	for _, u := range bad {
		if checkImageURL(u, hosts) == "" {
			t.Errorf("%s must be rejected", u)
		}
	}
	if checkImageURL("https://cdn.example.com/x", nil) == "" {
		t.Error("no hosts allowed means no URL")
	}
}
