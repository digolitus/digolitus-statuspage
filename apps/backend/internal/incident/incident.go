// Package incident manages the incident lifecycle and its public timeline.
//
// Lifecycle: investigating, identified and monitoring are open states and
// any open state may move to any other (a fix that does not hold can send an
// incident back to investigating). resolved is terminal: a recurrence is a
// new incident. Published updates are immutable.
package incident

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Statuses.
const (
	StatusInvestigating = "investigating"
	StatusIdentified    = "identified"
	StatusMonitoring    = "monitoring"
	StatusResolved      = "resolved"
)

// Statuses lists every incident status.
var Statuses = []string{StatusInvestigating, StatusIdentified, StatusMonitoring, StatusResolved}

// OpenStatuses lists the non-terminal statuses.
var OpenStatuses = []string{StatusInvestigating, StatusIdentified, StatusMonitoring}

// CanTransition reports whether an incident may move from one status to
// another. Staying in the same open status is allowed (message-only update).
func CanTransition(from, to string) bool {
	return slices.Contains(OpenStatuses, from) && slices.Contains(Statuses, to)
}

// Incident is the admin representation.
type Incident struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Impact        string          `json:"impact"`
	Status        string          `json:"status"`
	InternalNotes string          `json:"internal_notes"`
	StartedAt     time.Time       `json:"started_at"`
	ResolvedAt    *time.Time      `json:"resolved_at"`
	TemplateID    *string         `json:"template_id"`
	Components    []component.Ref `json:"components"`
	LatestUpdate  *Update         `json:"latest_update,omitempty"`
	Updates       []Update        `json:"updates,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Update is one immutable entry in an incident timeline. Components is set
// only when the update changed the affected components.
type Update struct {
	ID         string           `json:"id"`
	Status     string           `json:"status"`
	Message    string           `json:"message"`
	Components *[]component.Ref `json:"components"`
	CreatedAt  time.Time        `json:"created_at"`

	componentIDs []string
}

// PublicIncident is what unauthenticated clients see: no internal notes,
// no template linkage and only public components.
type PublicIncident struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Impact       string          `json:"impact"`
	Status       string          `json:"status"`
	StartedAt    time.Time       `json:"started_at"`
	ResolvedAt   *time.Time      `json:"resolved_at"`
	Components   []component.Ref `json:"components"`
	LatestUpdate *Update         `json:"latest_update,omitempty"`
	Updates      []Update        `json:"updates,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func publicUpdate(u Update) Update {
	if u.Components != nil {
		refs := component.PublicRefs(*u.Components)
		u.Components = &refs
	}
	return u
}

// Public converts to the public representation.
func (i Incident) Public() PublicIncident {
	p := PublicIncident{
		ID: i.ID, Title: i.Title, Impact: i.Impact, Status: i.Status,
		StartedAt: i.StartedAt, ResolvedAt: i.ResolvedAt,
		Components: component.PublicRefs(i.Components),
		CreatedAt:  i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
	if i.LatestUpdate != nil {
		u := publicUpdate(*i.LatestUpdate)
		p.LatestUpdate = &u
	}
	if i.Updates != nil {
		p.Updates = make([]Update, len(i.Updates))
		for k, u := range i.Updates {
			p.Updates[k] = publicUpdate(u)
		}
	}
	return p
}

// CreateInput opens an incident and publishes its first update.
type CreateInput struct {
	Title         string   `json:"title"`
	Message       string   `json:"message"`
	Status        *string  `json:"status"`
	Impact        string   `json:"impact"`
	ComponentIDs  []string `json:"component_ids"`
	StartedAt     *string  `json:"started_at"`
	InternalNotes *string  `json:"internal_notes"`
	TemplateID    *string  `json:"template_id"`
}

// PatchInput edits incident metadata. Status, messages and affected
// components change only through published updates.
type PatchInput struct {
	Title         *string `json:"title"`
	Impact        *string `json:"impact"`
	InternalNotes *string `json:"internal_notes"`
	StartedAt     *string `json:"started_at"`
}

// UpdateInput publishes a timeline entry, optionally changing status and
// affected components.
type UpdateInput struct {
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	ComponentIDs *[]string `json:"component_ids"`
}

// ResolveInput resolves an incident with a closing message.
type ResolveInput struct {
	Message string `json:"message"`
}

func validateTitle(v *apperror.Validator, t string) {
	n := utf8.RuneCountInString(t)
	v.Check(n >= 1 && n <= 200, "title", "Informe um título com 1 a 200 caracteres.")
}

func validateMessage(v *apperror.Validator, m string) {
	n := utf8.RuneCountInString(m)
	v.Check(n >= 1 && n <= 5000, "message", "Informe uma mensagem com 1 a 5000 caracteres.")
}

func validateImpact(v *apperror.Validator, i string) {
	v.Check(slices.Contains(statuspolicy.Impacts, i), "impact", "Impacto inválido. Valores aceitos: "+strings.Join(statuspolicy.Impacts, ", ")+".")
}

func validateNotes(v *apperror.Validator, n string) {
	v.Check(utf8.RuneCountInString(n) <= 5000, "internal_notes", "As notas internas devem ter no máximo 5000 caracteres.")
}

// parseStartedAt validates a start instant: RFC 3339 with offset, not in the
// future.
func parseStartedAt(v *apperror.Validator, raw string, now time.Time) time.Time {
	t, err := clock.ParseInstant(raw, nil)
	if err != nil {
		v.Add("started_at", "Use o formato RFC 3339 com fuso horário (ex.: 2026-10-17T14:30:00-03:00).")
		return time.Time{}
	}
	if t.After(now) {
		v.Add("started_at", "O início não pode estar no futuro.")
	}
	return t
}

type createPlan struct {
	incident     Incident
	message      string
	componentIDs []string
}

func (in CreateInput) normalize(now time.Time) (createPlan, error) {
	var v apperror.Validator
	inc := Incident{
		Title:     strings.TrimSpace(in.Title),
		Impact:    in.Impact,
		Status:    StatusInvestigating,
		StartedAt: now,
	}
	msg := strings.TrimSpace(in.Message)
	validateTitle(&v, inc.Title)
	validateMessage(&v, msg)
	validateImpact(&v, inc.Impact)
	if in.Status != nil {
		inc.Status = *in.Status
		v.Check(slices.Contains(OpenStatuses, inc.Status), "status",
			"O status inicial deve ser investigating, identified ou monitoring. Para registrar um incidente já resolvido, crie-o e resolva-o em seguida.")
	}
	if in.StartedAt != nil {
		inc.StartedAt = parseStartedAt(&v, *in.StartedAt, now)
	}
	if in.InternalNotes != nil {
		inc.InternalNotes = strings.TrimSpace(*in.InternalNotes)
		validateNotes(&v, inc.InternalNotes)
	}
	if in.TemplateID != nil {
		id := strings.ToLower(*in.TemplateID)
		v.Check(httpx.IsUUID(id), "template_id", "Informe um UUID válido.")
		inc.TemplateID = &id
	}
	return createPlan{incident: inc, message: msg, componentIDs: in.ComponentIDs}, v.Err()
}

func (in PatchInput) apply(inc Incident, now time.Time) (Incident, error) {
	var v apperror.Validator
	if in.Title == nil && in.Impact == nil && in.InternalNotes == nil && in.StartedAt == nil {
		return inc, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	if in.Title != nil {
		inc.Title = strings.TrimSpace(*in.Title)
		validateTitle(&v, inc.Title)
	}
	if in.Impact != nil {
		inc.Impact = *in.Impact
		validateImpact(&v, inc.Impact)
	}
	if in.InternalNotes != nil {
		inc.InternalNotes = strings.TrimSpace(*in.InternalNotes)
		validateNotes(&v, inc.InternalNotes)
	}
	if in.StartedAt != nil {
		inc.StartedAt = parseStartedAt(&v, *in.StartedAt, now)
		if inc.ResolvedAt != nil && inc.StartedAt.After(*inc.ResolvedAt) {
			v.Add("started_at", "O início não pode ser posterior à resolução.")
		}
	}
	return inc, v.Err()
}

func (in UpdateInput) validate(current string) (string, error) {
	var v apperror.Validator
	msg := strings.TrimSpace(in.Message)
	validateMessage(&v, msg)
	if !slices.Contains(Statuses, in.Status) {
		v.Add("status", "Status inválido. Valores aceitos: "+strings.Join(Statuses, ", ")+".")
	}
	if err := v.Err(); err != nil {
		return "", err
	}
	if !CanTransition(current, in.Status) {
		return "", apperror.Conflict("incident_resolved", "O incidente já foi resolvido e não aceita novas atualizações. Abra um novo incidente se o problema voltar a ocorrer.")
	}
	return msg, nil
}
