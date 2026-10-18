package component

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"API Pública":                "api-publica",
		"  Serviço de Autenticação ": "servico-de-autenticacao",
		"Banco/Leitura & Escrita":    "banco-leitura-escrita",
		"日本":                         "",
		strings.Repeat("a", 100):     strings.Repeat("a", 80),
	}
	for in, want := range tests {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateNormalizeRequiresUsableSlug(t *testing.T) {
	if _, err := (CreateInput{Name: "日本"}).normalize(); err == nil {
		t.Fatal("a name without ASCII letters needs an explicit slug")
	}
	slug := "nihon"
	if _, err := (CreateInput{Name: "日本", Slug: &slug}).normalize(); err != nil {
		t.Fatalf("explicit slug: %v", err)
	}
}
