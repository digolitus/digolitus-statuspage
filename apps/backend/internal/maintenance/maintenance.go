// Package maintenance manages scheduled maintenance windows.
//
// Lifecycle: scheduled → in_progress → completed, and scheduled → cancelled.
// Transitions are always explicit: nothing starts or completes automatically
// when the planned time passes; responses flag overdue windows instead.
package maintenance

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
	StatusScheduled  = "scheduled"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

// Statuses lists every maintenance status.
var Statuses = []string{StatusScheduled, StatusInProgress, StatusCompleted, StatusCancelled}

// Impacts lists the expected impact values.
var Impacts = statuspolicy.Impacts

// Transition names a lifecycle action.
type Transition string

// Lifecycle actions.
const (
	Start    Transition = "start"
	Complete Transition = "complete"
	Cancel   Transition = "cancel"
)

var transitions = map[Transition]struct{ from, to string }{
	Start:    {StatusScheduled, StatusInProgress},
	Complete: {StatusInProgress, StatusCompleted},
	Cancel:   {StatusScheduled, StatusCancelled},
}

// Target returns the status reached by t from the given status, and whether
// the transition is allowed.
func Target(t Transition, from string) (string, bool) {
	tr, ok := transitions[t]
	if !ok || tr.from != from {
		return "", false
	}
	return tr.to, true
}

// IsOpen reports whether a maintenance still accepts edits and messages.
func IsOpen(status string) bool {
	return status == StatusScheduled || status == StatusInProgress
}

// Maintenance is the admin representation.
type Maintenance struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	ExpectedImpact string          `json:"expected_impact"`
	Status         string          `json:"status"`
	Timezone       string          `json:"timezone"`
	ScheduledStart time.Time       `json:"scheduled_start"`
	ScheduledEnd   time.Time       `json:"scheduled_end"`
	StartedAt      *time.Time      `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
	CancelledAt    *time.Time      `json:"cancelled_at"`
	StartOverdue   bool            `json:"start_overdue"`
	EndOverdue     bool            `json:"end_overdue"`
	InternalNotes  string          `json:"internal_notes"`
	TemplateID     *string         `json:"template_id"`
	Components     []component.Ref `json:"components"`
	LatestUpdate   *Update         `json:"latest_update,omitempty"`
	Updates        []Update        `json:"updates,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// Update is an immutable timeline entry. Message is null for transitions
// published without a custom message.
type Update struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Message   *string   `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// PublicMaintenance omits internal fields and non-public components.
type PublicMaintenance struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	ExpectedImpact string          `json:"expected_impact"`
	Status         string          `json:"status"`
	Timezone       string          `json:"timezone"`
	ScheduledStart time.Time       `json:"scheduled_start"`
	ScheduledEnd   time.Time       `json:"scheduled_end"`
	StartedAt      *time.Time      `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
	CancelledAt    *time.Time      `json:"cancelled_at"`
	StartOverdue   bool            `json:"start_overdue"`
	EndOverdue     bool            `json:"end_overdue"`
	Components     []component.Ref `json:"components"`
	LatestUpdate   *Update         `json:"latest_update,omitempty"`
	Updates        []Update        `json:"updates,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// Public converts to the public representation.
func (m Maintenance) Public() PublicMaintenance {
	return PublicMaintenance{
		ID: m.ID, Title: m.Title, Description: m.Description, ExpectedImpact: m.ExpectedImpact,
		Status: m.Status, Timezone: m.Timezone, ScheduledStart: m.ScheduledStart, ScheduledEnd: m.ScheduledEnd,
		StartedAt: m.StartedAt, CompletedAt: m.CompletedAt, CancelledAt: m.CancelledAt,
		StartOverdue: m.StartOverdue, EndOverdue: m.EndOverdue,
		Components:   component.PublicRefs(m.Components),
		LatestUpdate: m.LatestUpdate, Updates: m.Updates,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// flag computes the overdue indicators relative to now. They inform
// operators and readers; they never change the status.
func (m *Maintenance) flag(now time.Time) {
	m.StartOverdue = m.Status == StatusScheduled && !now.Before(m.ScheduledStart)
	m.EndOverdue = m.Status == StatusInProgress && !now.Before(m.ScheduledEnd)
}

// CreateInput schedules a maintenance. scheduled_start and scheduled_end
// accept RFC 3339 with offset, or a local date-time interpreted in timezone.
type CreateInput struct {
	Title          string   `json:"title"`
	Description    *string  `json:"description"`
	Message        *string  `json:"message"`
	ExpectedImpact *string  `json:"expected_impact"`
	Timezone       string   `json:"timezone"`
	ScheduledStart string   `json:"scheduled_start"`
	ScheduledEnd   string   `json:"scheduled_end"`
	ComponentIDs   []string `json:"component_ids"`
	InternalNotes  *string  `json:"internal_notes"`
	TemplateID     *string  `json:"template_id"`
}

// PatchInput edits an open maintenance. A window that already started keeps
// its start time.
type PatchInput struct {
	Title          *string   `json:"title"`
	Description    *string   `json:"description"`
	ExpectedImpact *string   `json:"expected_impact"`
	Timezone       *string   `json:"timezone"`
	ScheduledStart *string   `json:"scheduled_start"`
	ScheduledEnd   *string   `json:"scheduled_end"`
	ComponentIDs   *[]string `json:"component_ids"`
	InternalNotes  *string   `json:"internal_notes"`
}

// MessageInput publishes a timeline message, or accompanies a transition.
type MessageInput struct {
	Message *string `json:"message"`
}

func validateText(v *apperror.Validator, field, value string, min, max int, msg string) {
	n := utf8.RuneCountInString(value)
	v.Check(n >= min && n <= max, field, msg)
}

func optionalMessage(v *apperror.Validator, raw *string) *string {
	if raw == nil {
		return nil
	}
	m := strings.TrimSpace(*raw)
	if m == "" {
		return nil
	}
	validateText(v, "message", m, 1, 5000, "A mensagem deve ter no máximo 5000 caracteres.")
	return &m
}

const instantHelp = "Use RFC 3339 com fuso (ex.: 2026-10-20T02:00:00-03:00) ou data e hora locais (ex.: 2026-10-20T02:00) interpretadas no timezone informado."

type createPlan struct {
	m            Maintenance
	message      *string
	componentIDs []string
}

func (in CreateInput) normalize(now time.Time) (createPlan, error) {
	var v apperror.Validator
	m := Maintenance{Title: strings.TrimSpace(in.Title), ExpectedImpact: statuspolicy.Operational, Status: StatusScheduled, Timezone: in.Timezone}
	validateText(&v, "title", m.Title, 1, 200, "Informe um título com 1 a 200 caracteres.")
	if in.Description != nil {
		m.Description = strings.TrimSpace(*in.Description)
		validateText(&v, "description", m.Description, 0, 5000, "A descrição deve ter no máximo 5000 caracteres.")
	}
	if in.ExpectedImpact != nil {
		m.ExpectedImpact = *in.ExpectedImpact
		v.Check(slices.Contains(Impacts, m.ExpectedImpact), "expected_impact", "Valor inválido. Valores aceitos: "+strings.Join(Impacts, ", ")+".")
	}
	if in.InternalNotes != nil {
		m.InternalNotes = strings.TrimSpace(*in.InternalNotes)
		validateText(&v, "internal_notes", m.InternalNotes, 0, 5000, "As notas internas devem ter no máximo 5000 caracteres.")
	}
	if in.TemplateID != nil {
		id := strings.ToLower(*in.TemplateID)
		v.Check(httpx.IsUUID(id), "template_id", "Informe um UUID válido.")
		m.TemplateID = &id
	}
	msg := optionalMessage(&v, in.Message)
	loc, err := clock.LoadZone(in.Timezone)
	if err != nil {
		v.Add("timezone", "Informe um timezone IANA válido (ex.: America/Sao_Paulo, UTC).")
	}
	if loc != nil {
		var okStart, okEnd bool
		m.ScheduledStart, okStart = parseInstant(&v, "scheduled_start", in.ScheduledStart, loc)
		m.ScheduledEnd, okEnd = parseInstant(&v, "scheduled_end", in.ScheduledEnd, loc)
		if okStart && okEnd {
			checkWindow(&v, m.ScheduledStart, m.ScheduledEnd)
			v.Check(m.ScheduledEnd.After(now), "scheduled_end", "O término previsto deve estar no futuro.")
		}
	}
	return createPlan{m: m, message: msg, componentIDs: in.ComponentIDs}, v.Err()
}

func parseInstant(v *apperror.Validator, field, raw string, loc *time.Location) (time.Time, bool) {
	if strings.TrimSpace(raw) == "" {
		v.Add(field, "Campo obrigatório. "+instantHelp)
		return time.Time{}, false
	}
	t, err := clock.ParseInstant(raw, loc)
	if err != nil {
		v.Add(field, instantHelp)
		return time.Time{}, false
	}
	return t, true
}

func checkWindow(v *apperror.Validator, start, end time.Time) {
	v.Check(end.After(start), "scheduled_end", "O término previsto deve ser posterior ao início previsto.")
	v.Check(end.Sub(start) <= 31*24*time.Hour, "scheduled_end", "A janela de manutenção deve ter no máximo 31 dias.")
}

func (in PatchInput) apply(m Maintenance, now time.Time) (Maintenance, bool, error) {
	if in.Title == nil && in.Description == nil && in.ExpectedImpact == nil && in.Timezone == nil &&
		in.ScheduledStart == nil && in.ScheduledEnd == nil && in.ComponentIDs == nil && in.InternalNotes == nil {
		return m, false, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	if !IsOpen(m.Status) {
		return m, false, apperror.Conflict("maintenance_closed", "Manutenções concluídas ou canceladas não podem ser editadas.")
	}
	var v apperror.Validator
	if in.Title != nil {
		m.Title = strings.TrimSpace(*in.Title)
		validateText(&v, "title", m.Title, 1, 200, "Informe um título com 1 a 200 caracteres.")
	}
	if in.Description != nil {
		m.Description = strings.TrimSpace(*in.Description)
		validateText(&v, "description", m.Description, 0, 5000, "A descrição deve ter no máximo 5000 caracteres.")
	}
	if in.ExpectedImpact != nil {
		m.ExpectedImpact = *in.ExpectedImpact
		v.Check(slices.Contains(Impacts, m.ExpectedImpact), "expected_impact", "Valor inválido. Valores aceitos: "+strings.Join(Impacts, ", ")+".")
	}
	if in.InternalNotes != nil {
		m.InternalNotes = strings.TrimSpace(*in.InternalNotes)
		validateText(&v, "internal_notes", m.InternalNotes, 0, 5000, "As notas internas devem ter no máximo 5000 caracteres.")
	}
	if in.Timezone != nil {
		m.Timezone = *in.Timezone
	}
	loc, err := clock.LoadZone(m.Timezone)
	if err != nil {
		v.Add("timezone", "Informe um timezone IANA válido (ex.: America/Sao_Paulo, UTC).")
	}
	rescheduled := false
	if loc != nil && (in.ScheduledStart != nil || in.ScheduledEnd != nil) {
		ok := true
		if in.ScheduledStart != nil {
			if m.Status != StatusScheduled {
				return m, false, apperror.Conflict("maintenance_already_started", "O início previsto não pode ser alterado depois que a manutenção começou.")
			}
			var okS bool
			m.ScheduledStart, okS = parseInstant(&v, "scheduled_start", *in.ScheduledStart, loc)
			ok = ok && okS
		}
		if in.ScheduledEnd != nil {
			var okE bool
			m.ScheduledEnd, okE = parseInstant(&v, "scheduled_end", *in.ScheduledEnd, loc)
			ok = ok && okE
		}
		if ok {
			checkWindow(&v, m.ScheduledStart, m.ScheduledEnd)
			if m.Status == StatusScheduled {
				v.Check(m.ScheduledEnd.After(now), "scheduled_end", "O término previsto deve estar no futuro.")
			}
		}
		rescheduled = true
	}
	return m, rescheduled, v.Err()
}
