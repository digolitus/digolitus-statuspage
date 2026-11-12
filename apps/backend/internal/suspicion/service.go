package suspicion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/incident"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/subscription"
)

// Options configures the service.
type Options struct {
	// Mailer nil disables sign-in links and e-mails to reporters.
	Mailer   subscription.Mailer
	BaseURL  string
	SiteName func() string
}

// Service implements the use cases.
type Service struct {
	pool      *pgxpool.Pool
	now       clock.Clock
	incidents *incident.Service
	mailer    subscription.Mailer
	baseURL   string
	siteName  func() string
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, now clock.Clock, incidents *incident.Service, o Options) *Service {
	s := &Service{pool: pool, now: now, incidents: incidents, mailer: o.Mailer, baseURL: o.BaseURL, siteName: o.SiteName}
	if s.siteName == nil {
		s.siteName = func() string { return "Status" }
	}
	return s
}

// EmailEnabled reports whether sign-in links can be sent.
func (s *Service) EmailEnabled() bool { return s.mailer != nil && s.baseURL != "" }

// ---------- public views ----------

// PublicSuspicion is a published suspicion as anyone sees it: no reporter
// names or texts, only counts and the most reported choices.
type PublicSuspicion struct {
	ID           string       `json:"id"`
	Number       int64        `json:"number"`
	Component    ComponentRef `json:"component"`
	Status       string       `json:"status"`
	OpenedAt     time.Time    `json:"opened_at"`
	PublishedAt  *time.Time   `json:"published_at"`
	LastReportAt time.Time    `json:"last_report_at"`
	ReportCount  int          `json:"report_count"`
	// RecentCount: reports in the last hour; Recent: per 10 minutes, oldest first.
	RecentCount int     `json:"recent_count"`
	Recent      []int   `json:"recent"`
	Summary     Summary `json:"summary"`
}

// recentBuckets counts the last hour's reports in six 10-minute buckets.
func recentBuckets(ctx context.Context, q database.Querier, ids []string, now time.Time) (map[string][]int, error) {
	out := map[string][]int{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT suspicion_id, floor(extract(epoch FROM ($3 - created_at)) / 600)::int AS b, count(*)
		FROM suspicion_reports WHERE suspicion_id = ANY($1::uuid[]) AND created_at > $2 AND created_at <= $3 GROUP BY 1, 2`,
		ids, now.Add(-time.Hour), now)
	if err != nil {
		return nil, fmt.Errorf("recent reports: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var b, n int
		if err := rows.Scan(&id, &b, &n); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = make([]int, 6)
		}
		if b >= 0 && b < 6 {
			out[id][5-b] += n
		}
	}
	return out, rows.Err()
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

// ListPublic returns the published suspicions, most recent activity first.
func (s *Service) ListPublic(ctx context.Context) ([]PublicSuspicion, error) {
	rows, err := queryRows(ctx, s.pool, "WHERE s.status = 'published' AND c.is_public ORDER BY s.last_report_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	recent, err := recentBuckets(ctx, s.pool, ids, s.now())
	if err != nil {
		return nil, err
	}
	out := make([]PublicSuspicion, len(rows))
	for i, r := range rows {
		b := recent[r.ID]
		if b == nil {
			b = make([]int, 6)
		}
		out[i] = PublicSuspicion{ID: r.ID, Number: r.Number, Component: r.Component, Status: r.Status, OpenedAt: r.OpenedAt,
			PublishedAt: r.PublishedAt, LastReportAt: r.LastReportAt, ReportCount: r.ReportCount, RecentCount: sum(b), Recent: b, Summary: r.Summary}
	}
	return out, nil
}

// ---------- reporter ----------

// MySuspicion is a suspicion the signed-in reporter took part in.
type MySuspicion struct {
	ID           string       `json:"id"`
	Number       int64        `json:"number"`
	Component    ComponentRef `json:"component"`
	Status       string       `json:"status"`
	ReportedAt   time.Time    `json:"reported_at"`
	ReportCount  int          `json:"report_count"`
	ClosedAt     *time.Time   `json:"closed_at"`
	CloseReason  *string      `json:"close_reason"`
	CloseMessage *string      `json:"close_message"`
	Incident     *IncidentRef `json:"incident"`
	Messages     []Message    `json:"messages"`
}

func (s *Service) mine(ctx context.Context, q database.Querier, reporterID, onlyID string) ([]MySuspicion, error) {
	where := "WHERE EXISTS (SELECT 1 FROM suspicion_reports r WHERE r.suspicion_id = s.id AND r.reporter_id = $1)"
	args := []any{reporterID}
	if onlyID != "" {
		where += " AND s.id = $2"
		args = append(args, onlyID)
	}
	rows, err := queryRows(ctx, q, where+" ORDER BY s.last_report_at DESC LIMIT 100", args...)
	if err != nil {
		return nil, err
	}
	out := make([]MySuspicion, 0, len(rows))
	for _, r := range rows {
		var reportedAt time.Time
		if err := q.QueryRow(ctx, "SELECT created_at FROM suspicion_reports WHERE suspicion_id = $1 AND reporter_id = $2", r.ID, reporterID).
			Scan(&reportedAt); err != nil {
			return nil, fmt.Errorf("my report: %w", err)
		}
		msgs, err := messages(ctx, q, r.ID, reporterID)
		if err != nil {
			return nil, err
		}
		for i := range msgs {
			// Reporters see the team, not who in it wrote.
			if msgs[i].Kind == MessageToReporter {
				msgs[i].AuthorName = "Equipe"
				if r.Team != nil {
					msgs[i].AuthorName = r.Team.Name
				}
			}
			msgs[i].Reporter = nil
		}
		out = append(out, MySuspicion{ID: r.ID, Number: r.Number, Component: r.Component, Status: r.Status, ReportedAt: reportedAt.UTC(),
			ReportCount: r.ReportCount, ClosedAt: r.ClosedAt, CloseReason: r.CloseReason, CloseMessage: r.CloseMessage, Incident: r.Incident, Messages: msgs})
	}
	return out, nil
}

// Mine lists the reporter's suspicions.
func (s *Service) Mine(ctx context.Context, rep *Reporter) ([]MySuspicion, error) {
	return s.mine(ctx, s.pool, rep.ID, "")
}

func (s *Service) mineOne(ctx context.Context, rep *Reporter, id string) (MySuspicion, error) {
	list, err := s.mine(ctx, s.pool, rep.ID, id)
	if err != nil {
		return MySuspicion{}, err
	}
	if len(list) == 0 {
		return MySuspicion{}, apperror.NotFound("suspicion")
	}
	return list[0], nil
}

// Report records a report, grouped into the component's open suspicion (or
// the one whose incident is still unresolved); otherwise a new suspicion
// starts pending triage.
func (s *Service) Report(ctx context.Context, rep *Reporter, in ReportInput) (MySuspicion, error) {
	now := s.now()
	r, err := in.normalize(now)
	if err != nil {
		return MySuspicion{}, err
	}
	var id string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT is_public AND is_active FROM components WHERE id = $1", r.componentID).Scan(&ok); err != nil || !ok {
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("check component: %w", err)
			}
			var v apperror.Validator
			v.Add("component_id", "Componente não encontrado.")
			return v.Err()
		}
		var recent int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM suspicion_reports WHERE reporter_id = $1 AND created_at > $2",
			rep.ID, now.Add(-time.Hour)).Scan(&recent); err != nil {
			return fmt.Errorf("count reports: %w", err)
		}
		if recent >= reportsPerHour {
			return tooMany("Você atingiu o limite de relatos por hora. Tente mais tarde.")
		}
		// One writer per component decides the grouping.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('suspicion_component:' || $1))", r.componentID); err != nil {
			return fmt.Errorf("lock component: %w", err)
		}
		err := tx.QueryRow(ctx, `SELECT s.id FROM suspicions s LEFT JOIN incidents i ON i.id = s.incident_id
			WHERE s.component_id = $1 AND (s.status IN ('pending', 'published') OR (s.status = 'incident' AND i.status <> 'resolved'))
			ORDER BY (s.status IN ('pending', 'published')) DESC, s.opened_at DESC LIMIT 1`, r.componentID).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if err := tx.QueryRow(ctx, `INSERT INTO suspicions (component_id, status, opened_at, last_report_at, updated_at)
				VALUES ($1, 'pending', $2, $2, $2) RETURNING id`, r.componentID, now).Scan(&id); err != nil {
				return fmt.Errorf("open suspicion: %w", err)
			}
		case err != nil:
			return fmt.Errorf("find suspicion: %w", err)
		default:
			if _, err := tx.Exec(ctx, "UPDATE suspicions SET last_report_at = $2, updated_at = $2 WHERE id = $1", id, now); err != nil {
				return fmt.Errorf("touch suspicion: %w", err)
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO suspicion_reports (suspicion_id, reporter_id, symptom, frequency, reach, noticed, noticed_at, location, error_ref, description, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, rep.ID, r.symptom, r.frequency, r.reach, r.noticed, r.noticedAt, r.location, r.errorRef, r.description, now)
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return apperror.Conflict("already_reported", "Você já reportou este problema. Acompanhe em Minhas suspeitas.")
		}
		if err != nil {
			return fmt.Errorf("insert report: %w", err)
		}
		return nil
	})
	if err != nil {
		return MySuspicion{}, err
	}
	return s.mineOne(ctx, rep, id)
}

// Reply adds the reporter's message to the conversation.
func (s *Service) Reply(ctx context.Context, rep *Reporter, id, body string) (MySuspicion, error) {
	b, err := checkBody("body", body)
	if err != nil {
		return MySuspicion{}, err
	}
	now := s.now()
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM suspicion_reports WHERE suspicion_id = $1 AND reporter_id = $2)", id, rep.ID).
			Scan(&exists); err != nil {
			return fmt.Errorf("check report: %w", err)
		}
		if !exists {
			return apperror.NotFound("suspicion")
		}
		var recent int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM suspicion_messages WHERE reporter_id = $1 AND kind = 'from_reporter' AND created_at > $2",
			rep.ID, now.Add(-time.Hour)).Scan(&recent); err != nil {
			return fmt.Errorf("count messages: %w", err)
		}
		if recent >= messagesPerHour {
			return tooMany("Você atingiu o limite de mensagens por hora. Tente mais tarde.")
		}
		_, err := tx.Exec(ctx, `INSERT INTO suspicion_messages (suspicion_id, kind, body, author_name, reporter_id, created_at)
			VALUES ($1, 'from_reporter', $2, $3, $4, $5)`, id, b, rep.Name, rep.ID, now)
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		_, err = tx.Exec(ctx, "UPDATE suspicions SET updated_at = $2 WHERE id = $1", id, now)
		return err
	})
	if err != nil {
		return MySuspicion{}, err
	}
	return s.mineOne(ctx, rep, id)
}

// ---------- admin ----------

// AdminSuspicion is a row of the triage queue.
type AdminSuspicion struct {
	ID           string       `json:"id"`
	Number       int64        `json:"number"`
	Component    ComponentRef `json:"component"`
	Team         *Ref         `json:"team"`
	Status       string       `json:"status"`
	Summary      Summary      `json:"summary"`
	ReportCount  int          `json:"report_count"`
	OpenedAt     time.Time    `json:"opened_at"`
	LastReportAt time.Time    `json:"last_report_at"`
	Assignee     *Ref         `json:"assignee"`
	Incident     *IncidentRef `json:"incident"`
}

func (r row) admin() AdminSuspicion {
	return AdminSuspicion{ID: r.ID, Number: r.Number, Component: r.Component, Team: r.Team, Status: r.Status, Summary: r.Summary,
		ReportCount: r.ReportCount, OpenedAt: r.OpenedAt, LastReportAt: r.LastReportAt, Assignee: r.Assignee, Incident: r.Incident}
}

// AdminFilter narrows the queue.
type AdminFilter struct {
	// Status: open (pending + published), a single status, or "" (all).
	Status string
	// Queue: mine (the user's teams), general (no owning team) or all.
	Queue string
}

// Counts are open suspicions in the user's scope, for the menu badge.
type Counts struct {
	Pending   int `json:"pending"`
	Published int `json:"published"`
}

// List returns the queue. Users whose permission is limited to their teams
// only ever see their teams' components.
func (s *Service) List(ctx context.Context, f AdminFilter, page httpx.Page) ([]AdminSuspicion, int, Counts, error) {
	var conds []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	scoped, teams := auth.TeamScoped(ctx, auth.PermSuspicionsManage)
	queue := f.Queue
	if scoped && queue != "mine" {
		if queue == "general" {
			return []AdminSuspicion{}, 0, Counts{}, nil
		}
		queue = "mine"
	}
	switch queue {
	case "mine":
		if !scoped {
			if p := auth.FromContext(ctx); p != nil {
				teams = p.Teams
			}
		}
		if teams == nil {
			teams = []string{}
		}
		conds = append(conds, effectiveTeam+"::text = ANY("+arg(teams)+")")
	case "general":
		conds = append(conds, effectiveTeam+" IS NULL")
	}
	scope := ""
	for _, c := range conds {
		scope += " AND " + c
	}
	var counts Counts
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE s.status = 'pending'), count(*) FILTER (WHERE s.status = 'published')
		FROM suspicions s JOIN components c ON c.id = s.component_id
		LEFT JOIN component_groups sg ON sg.id = c.subgroup_id LEFT JOIN component_groups g ON g.id = c.group_id
		WHERE true`+scope, args...).Scan(&counts.Pending, &counts.Published); err != nil {
		return nil, 0, Counts{}, fmt.Errorf("count suspicions: %w", err)
	}
	switch f.Status {
	case "open":
		scope += " AND s.status IN ('pending', 'published')"
	case "":
	default:
		scope += " AND s.status = " + arg(f.Status)
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM suspicions s JOIN components c ON c.id = s.component_id
		LEFT JOIN component_groups sg ON sg.id = c.subgroup_id LEFT JOIN component_groups g ON g.id = c.group_id
		WHERE true`+scope, args...).Scan(&total); err != nil {
		return nil, 0, Counts{}, fmt.Errorf("count suspicions: %w", err)
	}
	rows, err := queryRows(ctx, s.pool, "WHERE true"+scope+
		" ORDER BY (s.status = 'pending') DESC, s.last_report_at DESC, s.id LIMIT "+arg(page.Limit)+" OFFSET "+arg(page.Offset), args...)
	if err != nil {
		return nil, 0, Counts{}, err
	}
	out := make([]AdminSuspicion, len(rows))
	for i, r := range rows {
		out[i] = r.admin()
	}
	return out, total, counts, nil
}

// ReporterRef identifies who reported, for the team.
type ReporterRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Blocked bool   `json:"blocked"`
}

// Report is one report of a suspicion.
type Report struct {
	ID          string      `json:"id"`
	Reporter    ReporterRef `json:"reporter"`
	Symptom     string      `json:"symptom"`
	Frequency   string      `json:"frequency"`
	Reach       string      `json:"reach"`
	Noticed     string      `json:"noticed"`
	NoticedAt   time.Time   `json:"noticed_at"`
	Location    string      `json:"location"`
	ErrorRef    string      `json:"error_ref"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
}

// Count is how many reports chose a value.
type Count struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Distribution counts each choice across reports, most chosen first.
type Distribution struct {
	Symptom   []Count `json:"symptom"`
	Frequency []Count `json:"frequency"`
	Reach     []Count `json:"reach"`
}

// Detail is a suspicion as the team sees it.
type Detail struct {
	AdminSuspicion
	PublishedAt  *time.Time   `json:"published_at"`
	PublishedBy  *Ref         `json:"published_by"`
	ClosedAt     *time.Time   `json:"closed_at"`
	ClosedBy     *Ref         `json:"closed_by"`
	CloseReason  *string      `json:"close_reason"`
	CloseMessage *string      `json:"close_message"`
	Distribution Distribution `json:"distribution"`
	Reports      []Report     `json:"reports"`
	Messages     []Message    `json:"messages"`
}

func reports(ctx context.Context, q database.Querier, id string) ([]Report, error) {
	rows, err := q.Query(ctx, `SELECT r.id, p.id, p.name, p.email, p.is_blocked, r.symptom, r.frequency, r.reach, r.noticed, r.noticed_at,
		r.location, r.error_ref, r.description, r.created_at FROM suspicion_reports r JOIN reporters p ON p.id = r.reporter_id
		WHERE r.suspicion_id = $1 ORDER BY r.created_at, r.id`, id)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		if err := rows.Scan(&r.ID, &r.Reporter.ID, &r.Reporter.Name, &r.Reporter.Email, &r.Reporter.Blocked, &r.Symptom, &r.Frequency, &r.Reach,
			&r.Noticed, &r.NoticedAt, &r.Location, &r.ErrorRef, &r.Description, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.NoticedAt, r.CreatedAt = r.NoticedAt.UTC(), r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

func distribution(rs []Report) Distribution {
	count := func(order []string, pick func(Report) string) []Count {
		m := map[string]int{}
		for _, r := range rs {
			m[pick(r)]++
		}
		out := []Count{}
		for _, v := range order {
			if m[v] > 0 {
				out = append(out, Count{Value: v, Count: m[v]})
			}
		}
		slices.SortStableFunc(out, func(a, b Count) int { return b.Count - a.Count })
		return out
	}
	return Distribution{
		Symptom:   count(Symptoms, func(r Report) string { return r.Symptom }),
		Frequency: count(Frequencies, func(r Report) string { return r.Frequency }),
		Reach:     count(Reaches, func(r Report) string { return r.Reach }),
	}
}

// Get returns the detail, within the user's scope.
func (s *Service) Get(ctx context.Context, id string) (Detail, error) {
	r, err := getRow(ctx, s.pool, id)
	if err != nil {
		return Detail{}, err
	}
	if err := auth.RequireComponents(ctx, s.pool, auth.PermSuspicionsManage, []string{r.Component.ID}); err != nil {
		return Detail{}, err
	}
	rs, err := reports(ctx, s.pool, id)
	if err != nil {
		return Detail{}, err
	}
	msgs, err := messages(ctx, s.pool, id, "")
	if err != nil {
		return Detail{}, err
	}
	return Detail{AdminSuspicion: r.admin(), PublishedAt: r.PublishedAt, PublishedBy: r.PublishedBy, ClosedAt: r.ClosedAt, ClosedBy: r.ClosedBy,
		CloseReason: r.CloseReason, CloseMessage: r.CloseMessage, Distribution: distribution(rs), Reports: rs, Messages: msgs}, nil
}

func actorID(ctx context.Context) *string {
	if p := auth.FromContext(ctx); p != nil {
		return &p.UserID
	}
	return nil
}

func actorName(ctx context.Context) string {
	if p := auth.FromContext(ctx); p != nil && p.Name != "" {
		return p.Name
	}
	return "Equipe"
}

// change locks the suspicion, checks scope and the allowed states, then runs fn.
func (s *Service) change(ctx context.Context, id string, allowed []string, notAllowed string, fn func(tx pgx.Tx, componentID string, now time.Time) error) (Detail, error) {
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		status, componentID, err := lockRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := auth.RequireComponents(ctx, tx, auth.PermSuspicionsManage, []string{componentID}); err != nil {
			return err
		}
		if allowed != nil && !slices.Contains(allowed, status) {
			return invalidTransition(notAllowed)
		}
		return fn(tx, componentID, s.now())
	})
	if err != nil {
		return Detail{}, err
	}
	return s.Get(ctx, id)
}

// Publish makes a pending suspicion visible on the public page.
func (s *Service) Publish(ctx context.Context, id string) (Detail, error) {
	return s.change(ctx, id, []string{StatusPending}, "Só suspeitas pendentes de triagem podem ser publicadas.",
		func(tx pgx.Tx, _ string, now time.Time) error {
			if _, err := tx.Exec(ctx, "UPDATE suspicions SET status = 'published', published_at = $2, published_by = $3, updated_at = $2 WHERE id = $1",
				id, now, actorID(ctx)); err != nil {
				return fmt.Errorf("publish suspicion: %w", err)
			}
			return audit.Record(ctx, tx, audit.ResourceSuspicion, id, "published", audit.Changes{
				Before: map[string]any{"status": StatusPending}, After: map[string]any{"status": StatusPublished}}, now)
		})
}

// CloseInput ends the analysis.
type CloseInput struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Close ends the analysis with a reason and a message every reporter receives.
func (s *Service) Close(ctx context.Context, id string, in CloseInput) (Detail, error) {
	var v apperror.Validator
	v.Check(slices.Contains(CloseReasons, in.Reason), "reason", "Escolha o motivo.")
	msg, err := checkBody("message", in.Message)
	if err != nil {
		return Detail{}, err
	}
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	return s.change(ctx, id, []string{StatusPending, StatusPublished}, "Esta suspeita já foi encerrada.",
		func(tx pgx.Tx, _ string, now time.Time) error {
			if _, err := tx.Exec(ctx, `UPDATE suspicions SET status = 'closed', closed_at = $2, closed_by = $3, close_reason = $4, close_message = $5,
				updated_at = $2 WHERE id = $1`, id, now, actorID(ctx), in.Reason, msg); err != nil {
				return fmt.Errorf("close suspicion: %w", err)
			}
			if err := s.mailAll(ctx, tx, id, "Análise concluída", "A análise da sua suspeita foi concluída.\n\n"+msg, now); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.ResourceSuspicion, id, "closed", audit.Changes{
				After: map[string]any{"status": StatusClosed, "reason": in.Reason, "message": msg}}, now)
		})
}

// OpenIncident opens an incident from the suspicion and links them, in one
// transaction. Without components, the reported one is used.
func (s *Service) OpenIncident(ctx context.Context, id string, in incident.CreateInput) (Detail, error) {
	return s.change(ctx, id, []string{StatusPending, StatusPublished}, "Esta suspeita já foi encerrada.",
		func(tx pgx.Tx, componentID string, now time.Time) error {
			if len(in.ComponentIDs) == 0 {
				in.ComponentIDs = []string{componentID}
			}
			incID, err := s.incidents.CreateInTx(ctx, tx, in)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE suspicions SET status = 'incident', incident_id = $2, updated_at = $3 WHERE id = $1", id, incID, now); err != nil {
				return fmt.Errorf("link incident: %w", err)
			}
			if err := s.mailAll(ctx, tx, id, "Incidente aberto",
				"Confirmamos o problema que você reportou e abrimos um incidente: "+in.Title+".\nAcompanhe na página de status: "+s.baseURL+"/incidents/"+incID, now); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.ResourceSuspicion, id, "incident_opened", audit.Changes{
				After: map[string]any{"status": StatusIncident, "incident_id": incID}}, now)
		})
}

// PatchInput changes who is handling the suspicion.
type PatchInput struct {
	// AssigneeID: a user id, or "" to clear. Absent: unchanged.
	AssigneeID *string `json:"assignee_id"`
}

// Patch updates the suspicion's assignee.
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Detail, error) {
	return s.change(ctx, id, nil, "", func(tx pgx.Tx, _ string, now time.Time) error {
		if in.AssigneeID == nil {
			return nil
		}
		var assignee *string
		if *in.AssigneeID != "" {
			var ok bool
			err := tx.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", *in.AssigneeID).Scan(&ok)
			if !isUUID(*in.AssigneeID) || errors.Is(err, pgx.ErrNoRows) || (err == nil && !ok) {
				var v apperror.Validator
				v.Add("assignee_id", "Escolha um usuário ativo.")
				return v.Err()
			}
			if err != nil {
				return fmt.Errorf("check assignee: %w", err)
			}
			assignee = in.AssigneeID
		}
		if _, err := tx.Exec(ctx, "UPDATE suspicions SET assignee_id = $2, updated_at = $3 WHERE id = $1", id, assignee, now); err != nil {
			return fmt.Errorf("assign suspicion: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceSuspicion, id, "assigned", audit.Changes{After: map[string]any{"assignee_id": assignee}}, now)
	})
}

// MessageInput is a note or a message to a reporter.
type MessageInput struct {
	// Kind: internal (team only) or reporter.
	Kind string `json:"kind"`
	Body string `json:"body"`
	// ReporterID: who receives a reporter message; default the first reporter.
	ReporterID *string `json:"reporter_id"`
}

// AddMessage records an internal note or sends a message to a reporter.
func (s *Service) AddMessage(ctx context.Context, id string, in MessageInput) (Detail, error) {
	var v apperror.Validator
	v.Check(in.Kind == "internal" || in.Kind == "reporter", "kind", "Use internal ou reporter.")
	body, err := checkBody("body", in.Body)
	if err != nil {
		return Detail{}, err
	}
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	return s.change(ctx, id, nil, "", func(tx pgx.Tx, _ string, now time.Time) error {
		if in.Kind == "internal" {
			_, err := tx.Exec(ctx, `INSERT INTO suspicion_messages (suspicion_id, kind, body, author_user_id, author_name, created_at)
				VALUES ($1, 'internal', $2, $3, $4, $5)`, id, body, actorID(ctx), actorName(ctx), now)
			if err != nil {
				return fmt.Errorf("insert note: %w", err)
			}
			return nil
		}
		var reporterID string
		var err error
		if in.ReporterID != nil {
			err = tx.QueryRow(ctx, "SELECT reporter_id FROM suspicion_reports WHERE suspicion_id = $1 AND reporter_id::text = $2", id, *in.ReporterID).Scan(&reporterID)
		} else {
			err = tx.QueryRow(ctx, "SELECT reporter_id FROM suspicion_reports WHERE suspicion_id = $1 ORDER BY created_at, id LIMIT 1", id).Scan(&reporterID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			var v apperror.Validator
			v.Add("reporter_id", "Escolha alguém que reportou esta suspeita.")
			return v.Err()
		}
		if err != nil {
			return fmt.Errorf("find reporter: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO suspicion_messages (suspicion_id, kind, body, author_user_id, author_name, reporter_id, created_at)
			VALUES ($1, 'to_reporter', $2, $3, $4, $5, $6)`, id, body, actorID(ctx), actorName(ctx), reporterID, now); err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		if _, err := tx.Exec(ctx, "UPDATE suspicions SET updated_at = $2 WHERE id = $1", id, now); err != nil {
			return err
		}
		return s.mail(ctx, tx, reporterID, id, "Nova mensagem sobre sua suspeita", body, now)
	})
}

// BlockInput blocks or unblocks a reporter.
type BlockInput struct {
	Blocked *bool `json:"blocked"`
}

// SetBlocked blocks (ending their sessions) or unblocks a reporter. Only
// users holding the permission for the whole platform may do it.
func (s *Service) SetBlocked(ctx context.Context, reporterID string, in BlockInput) (ReporterRef, error) {
	if scoped, _ := auth.TeamScoped(ctx, auth.PermSuspicionsManage); scoped {
		return ReporterRef{}, apperror.Forbidden("Só quem tria suspeitas de toda a plataforma pode bloquear quem reporta.")
	}
	if in.Blocked == nil {
		var v apperror.Validator
		v.Add("blocked", "Informe true ou false.")
		return ReporterRef{}, v.Err()
	}
	now := s.now()
	var r ReporterRef
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "UPDATE reporters SET is_blocked = $2 WHERE id = $1 RETURNING id, name, email, is_blocked", reporterID, *in.Blocked).
			Scan(&r.ID, &r.Name, &r.Email, &r.Blocked)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("reporter")
		}
		if err != nil {
			return fmt.Errorf("block reporter: %w", err)
		}
		if r.Blocked {
			if _, err := tx.Exec(ctx, "DELETE FROM reporter_sessions WHERE reporter_id = $1", reporterID); err != nil {
				return fmt.Errorf("end reporter sessions: %w", err)
			}
		}
		action := "unblocked"
		if r.Blocked {
			action = "blocked"
		}
		return audit.Record(ctx, tx, audit.ResourceReporter, reporterID, action, audit.Changes{After: map[string]any{"blocked": r.Blocked}}, now)
	})
	return r, err
}
