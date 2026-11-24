// Package suspicion implements "Reportar suspeita" (ADR 0019): visitors with
// a light account report a suspected failure of a public component, reports
// about the same component are grouped into one suspicion, and the team that
// owns the component triages it (publish, open an incident or close).
package suspicion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Suspicion states.
const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusIncident  = "incident"
	StatusClosed    = "closed"
)

// Closed choices of a report; fixed lists so reports can be grouped and counted.
var (
	Statuses     = []string{StatusPending, StatusPublished, StatusIncident, StatusClosed}
	Symptoms     = []string{"down", "slow", "error", "partial", "wrong_data", "other"}
	Frequencies  = []string{"continuous", "intermittent", "once"}
	Reaches      = []string{"only_me", "my_team", "many_places", "unknown"}
	NoticedTimes = []string{"now", "last_30m", "last_2h", "other"}
	CloseReasons = []string{"no_evidence", "duplicate", "out_of_scope"}
)

// Message kinds.
const (
	MessageInternal     = "internal"
	MessageToReporter   = "to_reporter"
	MessageFromReporter = "from_reporter"
)

// Limits against abuse.
const (
	reportsPerHour  = 5
	messagesPerHour = 20
)

// ComponentRef names the reported component.
type ComponentRef struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Group *string `json:"group"`
}

// Ref is an id/name pair (team, user).
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IncidentRef is the incident opened from a suspicion.
type IncidentRef struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

// Summary has the most reported value of each choice.
type Summary struct {
	Symptom   string `json:"symptom"`
	Frequency string `json:"frequency"`
	Reach     string `json:"reach"`
}

// row is a suspicion with its joined data.
type row struct {
	ID           string
	Number       int64
	Status       string
	OpenedAt     time.Time
	LastReportAt time.Time
	PublishedAt  *time.Time
	ClosedAt     *time.Time
	CloseReason  *string
	CloseMessage *string
	Component    ComponentRef
	Team         *Ref
	Assignee     *Ref
	PublishedBy  *Ref
	ClosedBy     *Ref
	Incident     *IncidentRef
	ReportCount  int
	Summary      Summary
}

const selectRows = `SELECT s.id, s.number, s.status, s.opened_at, s.last_report_at, s.published_at, s.closed_at, s.close_reason, s.close_message,
	c.id, c.name, g.name, t.id, t.name, a.id, a.name, pb.id, pb.name, cb.id, cb.name, i.id, i.title, i.status, i.resolved_at,
	(SELECT count(*) FROM suspicion_reports r WHERE r.suspicion_id = s.id),
	COALESCE((SELECT symptom FROM suspicion_reports r WHERE r.suspicion_id = s.id GROUP BY symptom ORDER BY count(*) DESC, min(created_at) LIMIT 1), ''),
	COALESCE((SELECT frequency FROM suspicion_reports r WHERE r.suspicion_id = s.id GROUP BY frequency ORDER BY count(*) DESC, min(created_at) LIMIT 1), ''),
	COALESCE((SELECT reach FROM suspicion_reports r WHERE r.suspicion_id = s.id GROUP BY reach ORDER BY count(*) DESC, min(created_at) LIMIT 1), '')
	FROM suspicions s
	JOIN components c ON c.id = s.component_id
	LEFT JOIN component_groups sg ON sg.id = c.subgroup_id
	LEFT JOIN component_groups g ON g.id = c.group_id
	LEFT JOIN teams t ON t.id = COALESCE(c.team_id, sg.team_id, g.team_id)
	LEFT JOIN users a ON a.id = s.assignee_id
	LEFT JOIN users pb ON pb.id = s.published_by
	LEFT JOIN users cb ON cb.id = s.closed_by
	LEFT JOIN incidents i ON i.id = s.incident_id`

// effectiveTeam is the owner of the suspicion's component, for filters.
const effectiveTeam = "COALESCE(c.team_id, sg.team_id, g.team_id)"

func optRef(id, name *string) *Ref {
	if id == nil {
		return nil
	}
	n := ""
	if name != nil {
		n = *name
	}
	return &Ref{ID: *id, Name: n}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scanRow(r pgx.Row) (row, error) {
	var s row
	var teamID, teamName, aID, aName, pbID, pbName, cbID, cbName, incID, incTitle, incStatus *string
	var incResolved *time.Time
	err := r.Scan(&s.ID, &s.Number, &s.Status, &s.OpenedAt, &s.LastReportAt, &s.PublishedAt, &s.ClosedAt, &s.CloseReason, &s.CloseMessage,
		&s.Component.ID, &s.Component.Name, &s.Component.Group, &teamID, &teamName, &aID, &aName, &pbID, &pbName, &cbID, &cbName,
		&incID, &incTitle, &incStatus, &incResolved, &s.ReportCount, &s.Summary.Symptom, &s.Summary.Frequency, &s.Summary.Reach)
	if err != nil {
		return row{}, err
	}
	s.OpenedAt, s.LastReportAt = s.OpenedAt.UTC(), s.LastReportAt.UTC()
	s.PublishedAt, s.ClosedAt = utc(s.PublishedAt), utc(s.ClosedAt)
	s.Team, s.Assignee, s.PublishedBy, s.ClosedBy = optRef(teamID, teamName), optRef(aID, aName), optRef(pbID, pbName), optRef(cbID, cbName)
	if incID != nil {
		s.Incident = &IncidentRef{ID: *incID, Title: *incTitle, Status: *incStatus, ResolvedAt: utc(incResolved)}
	}
	return s, nil
}

func queryRows(ctx context.Context, q database.Querier, where string, args ...any) ([]row, error) {
	rows, err := q.Query(ctx, selectRows+" "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query suspicions: %w", err)
	}
	defer rows.Close()
	out := []row{}
	for rows.Next() {
		s, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan suspicion: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func getRow(ctx context.Context, q database.Querier, id string) (row, error) {
	rows, err := queryRows(ctx, q, "WHERE s.id = $1", id)
	if err != nil {
		return row{}, err
	}
	if len(rows) == 0 {
		return row{}, apperror.NotFound("suspicion")
	}
	return rows[0], nil
}

// lockRow locks the suspicion for a state change and returns its state and
// component.
func lockRow(ctx context.Context, tx pgx.Tx, id string) (status, componentID string, err error) {
	err = tx.QueryRow(ctx, "SELECT status, component_id FROM suspicions WHERE id = $1 FOR UPDATE", id).Scan(&status, &componentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", apperror.NotFound("suspicion")
	}
	if err != nil {
		return "", "", fmt.Errorf("lock suspicion: %w", err)
	}
	return status, componentID, nil
}

// Message is a note or a message of the conversation.
type Message struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Body       string    `json:"body"`
	AuthorName string    `json:"author_name"`
	Reporter   *Ref      `json:"reporter"`
	CreatedAt  time.Time `json:"created_at"`
}

// messages lists a suspicion's messages; reporterID limits them to the
// conversation with that reporter (internal notes excluded).
func messages(ctx context.Context, q database.Querier, suspicionID, reporterID string) ([]Message, error) {
	sql := `SELECT m.id, m.kind, m.body, m.author_name, r.id, r.name, m.created_at FROM suspicion_messages m
		LEFT JOIN reporters r ON r.id = m.reporter_id WHERE m.suspicion_id = $1`
	args := []any{suspicionID}
	if reporterID != "" {
		sql += " AND m.kind <> 'internal' AND m.reporter_id = $2"
		args = append(args, reporterID)
	}
	rows, err := q.Query(ctx, sql+" ORDER BY m.created_at, m.id", args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var rid, rname *string
		if err := rows.Scan(&m.ID, &m.Kind, &m.Body, &m.AuthorName, &rid, &rname, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Reporter, m.CreatedAt = optRef(rid, rname), m.CreatedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReportInput is a new report.
type ReportInput struct {
	ComponentID string  `json:"component_id"`
	Symptom     string  `json:"symptom"`
	Frequency   string  `json:"frequency"`
	Reach       string  `json:"reach"`
	Noticed     string  `json:"noticed"`
	NoticedAt   *string `json:"noticed_at"`
	Location    string  `json:"location"`
	ErrorRef    string  `json:"error_ref"`
	Description string  `json:"description"`
}

type report struct {
	componentID, symptom, frequency, reach, noticed, location, errorRef, description string
	noticedAt                                                                        time.Time
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func (in ReportInput) normalize(now time.Time) (report, error) {
	var v apperror.Validator
	r := report{componentID: in.ComponentID, symptom: in.Symptom, frequency: in.Frequency, reach: in.Reach, noticed: in.Noticed,
		location: oneLine(in.Location), errorRef: oneLine(in.ErrorRef), description: strings.TrimSpace(in.Description)}
	v.Check(isUUID(r.componentID), "component_id", "Escolha um componente.")
	v.Check(slices.Contains(Symptoms, r.symptom), "symptom", "Escolha o que você percebeu.")
	v.Check(slices.Contains(Frequencies, r.frequency), "frequency", "Escolha a frequência.")
	v.Check(slices.Contains(Reaches, r.reach), "reach", "Escolha a abrangência.")
	v.Check(len([]rune(r.location)) <= 120, "location", "Use até 120 caracteres.")
	v.Check(len([]rune(r.errorRef)) <= 300, "error_ref", "Use até 300 caracteres.")
	n := len([]rune(r.description))
	v.Check(n >= 1 && n <= 1000, "description", "Descreva o problema em até 1000 caracteres.")
	switch r.noticed {
	case "now":
		r.noticedAt = now
	case "last_30m":
		r.noticedAt = now.Add(-30 * time.Minute)
	case "last_2h":
		r.noticedAt = now.Add(-2 * time.Hour)
	case "other":
		var t time.Time
		var err error
		if in.NoticedAt != nil {
			t, err = time.Parse(time.RFC3339, *in.NoticedAt)
		}
		switch {
		case in.NoticedAt == nil || err != nil:
			v.Add("noticed_at", "Informe data e hora.")
		case t.After(now.Add(5*time.Minute)) || t.Before(now.Add(-7*24*time.Hour)):
			v.Add("noticed_at", "Informe um horário dos últimos 7 dias.")
		default:
			r.noticedAt = t.UTC()
		}
	default:
		v.Add("noticed", "Escolha quando percebeu.")
	}
	return r, v.Err()
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

func tooMany(msg string) error {
	return &apperror.Error{Kind: apperror.KindTooMany, Code: "rate_limited", Message: msg}
}

func invalidTransition(msg string) error {
	return apperror.Conflict("invalid_transition", msg)
}

func checkBody(field, body string) (string, error) {
	b := strings.TrimSpace(body)
	var v apperror.Validator
	n := len([]rune(b))
	v.Check(n >= 1 && n <= 2000, field, "Escreva até 2000 caracteres.")
	return b, v.Err()
}
