// Package component manages the services and components published on the
// status page. Component status here is the status declared by operators;
// it is never the result of an automated check.
package component

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Component is the admin representation of a component.
type Component struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	IsPublic     bool      `json:"is_public"`
	IsActive     bool      `json:"is_active"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Ref is the minimal reference to a component embedded in other resources.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Public is used to filter references out of public responses.
	Public bool `json:"-"`
}

// PublicRefs returns only references to public components.
func PublicRefs(refs []Ref) []Ref {
	out := []Ref{}
	for _, r := range refs {
		if r.Public {
			out = append(out, r)
		}
	}
	return out
}

// CreateInput is the payload to create a component.
type CreateInput struct {
	Name         string  `json:"name"`
	Slug         *string `json:"slug"`
	Description  *string `json:"description"`
	Status       *string `json:"status"`
	IsPublic     *bool   `json:"is_public"`
	DisplayOrder *int    `json:"display_order"`
}

// UpdateInput is a partial update; nil fields are left unchanged.
type UpdateInput struct {
	Name         *string `json:"name"`
	Slug         *string `json:"slug"`
	Description  *string `json:"description"`
	Status       *string `json:"status"`
	IsPublic     *bool   `json:"is_public"`
	IsActive     *bool   `json:"is_active"`
	DisplayOrder *int    `json:"display_order"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slugify derives a URL-safe slug from a name, dropping accents.
func Slugify(name string) string {
	decomposed := norm.NFD.String(strings.ToLower(name))
	var b strings.Builder
	dash := false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 80 {
		s = strings.Trim(s[:80], "-")
	}
	return s
}

func validateName(v *apperror.Validator, name string) {
	n := utf8.RuneCountInString(name)
	v.Check(n >= 1 && n <= 120, "name", "Informe um nome com 1 a 120 caracteres.")
}

func validateSlug(v *apperror.Validator, slug string) {
	v.Check(len(slug) <= 80 && slugPattern.MatchString(slug), "slug",
		"Use de 1 a 80 caracteres: letras minúsculas, números e hífens (ex.: api-publica).")
}

func validateDescription(v *apperror.Validator, d string) {
	v.Check(utf8.RuneCountInString(d) <= 1000, "description", "A descrição deve ter no máximo 1000 caracteres.")
}

func validateStatus(v *apperror.Validator, s string) {
	v.Check(slices.Contains(statuspolicy.ComponentStatuses, s), "status",
		"Status inválido. Valores aceitos: "+strings.Join(statuspolicy.ComponentStatuses, ", ")+".")
}

func validateOrder(v *apperror.Validator, o int) {
	v.Check(o >= 0 && o <= 100000, "display_order", "Informe um inteiro entre 0 e 100000.")
}

// normalize validates a create payload and returns the component to insert.
func (in CreateInput) normalize() (Component, error) {
	var v apperror.Validator
	c := Component{
		Name:     strings.TrimSpace(in.Name),
		Status:   statuspolicy.Operational,
		IsPublic: true,
		IsActive: true,
	}
	validateName(&v, c.Name)
	if in.Slug != nil {
		c.Slug = strings.TrimSpace(*in.Slug)
	} else {
		c.Slug = Slugify(c.Name)
	}
	if !v.Has("name") {
		validateSlug(&v, c.Slug)
	}
	if in.Description != nil {
		c.Description = strings.TrimSpace(*in.Description)
		validateDescription(&v, c.Description)
	}
	if in.Status != nil {
		c.Status = *in.Status
		validateStatus(&v, c.Status)
	}
	if in.IsPublic != nil {
		c.IsPublic = *in.IsPublic
	}
	if in.DisplayOrder != nil {
		c.DisplayOrder = *in.DisplayOrder
		validateOrder(&v, c.DisplayOrder)
	}
	return c, v.Err()
}

// apply validates a partial update and returns the updated component.
func (in UpdateInput) apply(c Component) (Component, error) {
	var v apperror.Validator
	changed := false
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
		validateName(&v, c.Name)
		changed = true
	}
	if in.Slug != nil {
		c.Slug = strings.TrimSpace(*in.Slug)
		validateSlug(&v, c.Slug)
		changed = true
	}
	if in.Description != nil {
		c.Description = strings.TrimSpace(*in.Description)
		validateDescription(&v, c.Description)
		changed = true
	}
	if in.Status != nil {
		c.Status = *in.Status
		validateStatus(&v, c.Status)
		changed = true
	}
	if in.IsPublic != nil {
		c.IsPublic = *in.IsPublic
		changed = true
	}
	if in.IsActive != nil {
		c.IsActive = *in.IsActive
		changed = true
	}
	if in.DisplayOrder != nil {
		c.DisplayOrder = *in.DisplayOrder
		validateOrder(&v, c.DisplayOrder)
		changed = true
	}
	if !changed {
		return c, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return c, v.Err()
}
