// Package announcement manages general notices for customers that are
// neither incidents nor maintenances, plus the categories and responsible
// teams used to organise them. Announcements never affect any status.
//
// Lifecycle (stored status → presented state):
//
//	draft ──publish──► published (state "scheduled" while published_at is
//	  ▲                    │      in the future, "published" afterwards)
//	  └────unpublish───────┤
//	                       └──archive──► archived ──restore──► published
//
// Only drafts can be deleted; published announcements are archived.
package announcement

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/optional"
)

// Stored statuses.
const (
	statusDraft     = "draft"
	statusPublished = "published"
	statusArchived  = "archived"
)

// Presented states.
const (
	StateDraft     = "draft"
	StateScheduled = "scheduled"
	StatePublished = "published"
	StateArchived  = "archived"
)

// States are the values of the admin "state" filter.
var States = []string{StateDraft, StateScheduled, StatePublished, StateArchived}

// Importances: "important" is highlighted on the public page.
var Importances = []string{"info", "important"}

// Limits.
const (
	maxTitle      = 200
	maxBody       = 10000
	maxScheduleIn = 366 * 24 * time.Hour
)

// CategoryRef is the category embedded in an announcement.
type CategoryRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// TeamRef is the team embedded in an announcement.
type TeamRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Announcement is the admin representation.
type Announcement struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	Importance  string          `json:"importance"`
	IsPinned    bool            `json:"is_pinned"`
	State       string          `json:"state"`
	Category    *CategoryRef    `json:"category"`
	Team        *TeamRef        `json:"team"`
	Components  []component.Ref `json:"components"`
	PublishedAt *time.Time      `json:"published_at"`
	ArchivedAt  *time.Time      `json:"archived_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`

	status string
}

// Public is an announcement as shown on the public page.
type Public struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	Importance  string          `json:"importance"`
	IsPinned    bool            `json:"is_pinned"`
	Category    *CategoryRef    `json:"category"`
	Team        *TeamRef        `json:"team"`
	Components  []component.Ref `json:"components"`
	PublishedAt time.Time       `json:"published_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func (a Announcement) public() Public {
	return Public{ID: a.ID, Title: a.Title, Body: a.Body, Importance: a.Importance, IsPinned: a.IsPinned,
		Category: a.Category, Team: a.Team, Components: component.PublicRefs(a.Components),
		PublishedAt: *a.PublishedAt, UpdatedAt: a.UpdatedAt}
}

// CreateInput creates a draft.
type CreateInput struct {
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Importance   *string  `json:"importance"`
	IsPinned     *bool    `json:"is_pinned"`
	CategoryID   *string  `json:"category_id"`
	TeamID       *string  `json:"team_id"`
	ComponentIDs []string `json:"component_ids"`
}

// PatchInput edits content; nil/absent fields are unchanged and null clears
// the category or team.
type PatchInput struct {
	Title        *string                   `json:"title"`
	Body         *string                   `json:"body"`
	Importance   *string                   `json:"importance"`
	IsPinned     *bool                     `json:"is_pinned"`
	CategoryID   optional.Nullable[string] `json:"category_id"`
	TeamID       optional.Nullable[string] `json:"team_id"`
	ComponentIDs *[]string                 `json:"component_ids"`
}

// PublishInput optionally schedules the publication.
type PublishInput struct {
	PublishAt *string `json:"publish_at"`
}

// content holds the validated editable fields.
type content struct {
	title, body, importance string
	pinned                  bool
	categoryID, teamID      *string
}

func checkTitle(v *apperror.Validator, s string) {
	n := utf8.RuneCountInString(s)
	v.Check(n >= 1 && n <= maxTitle, "title", fmt.Sprintf("Informe um título com 1 a %d caracteres.", maxTitle))
}

func checkBody(v *apperror.Validator, s string) {
	n := utf8.RuneCountInString(s)
	v.Check(n >= 1 && n <= maxBody, "body", fmt.Sprintf("Informe o texto com 1 a %d caracteres.", maxBody))
}

func checkImportance(v *apperror.Validator, s string) {
	v.Check(slices.Contains(Importances, s), "importance", "Valor inválido. Valores aceitos: "+strings.Join(Importances, ", ")+".")
}

func normalizeRef(v *apperror.Validator, field string, id *string) *string {
	if id == nil {
		return nil
	}
	s := strings.ToLower(strings.TrimSpace(*id))
	v.Check(httpx.IsUUID(s), field, "Informe um UUID válido ou null.")
	return &s
}

func (in CreateInput) normalize() (content, error) {
	var v apperror.Validator
	c := content{title: strings.TrimSpace(in.Title), body: strings.TrimSpace(in.Body), importance: "info"}
	checkTitle(&v, c.title)
	checkBody(&v, c.body)
	if in.Importance != nil {
		c.importance = *in.Importance
		checkImportance(&v, c.importance)
	}
	if in.IsPinned != nil {
		c.pinned = *in.IsPinned
	}
	c.categoryID = normalizeRef(&v, "category_id", in.CategoryID)
	c.teamID = normalizeRef(&v, "team_id", in.TeamID)
	return c, v.Err()
}

func (in PatchInput) apply(a Announcement) (content, error) {
	var v apperror.Validator
	c := content{title: a.Title, body: a.Body, importance: a.Importance, pinned: a.IsPinned}
	if a.Category != nil {
		c.categoryID = &a.Category.ID
	}
	if a.Team != nil {
		c.teamID = &a.Team.ID
	}
	changed := in.ComponentIDs != nil
	if in.Title != nil {
		c.title = strings.TrimSpace(*in.Title)
		checkTitle(&v, c.title)
		changed = true
	}
	if in.Body != nil {
		c.body = strings.TrimSpace(*in.Body)
		checkBody(&v, c.body)
		changed = true
	}
	if in.Importance != nil {
		c.importance = *in.Importance
		checkImportance(&v, c.importance)
		changed = true
	}
	if in.IsPinned != nil {
		c.pinned = *in.IsPinned
		changed = true
	}
	if in.CategoryID.Set {
		c.categoryID = normalizeRef(&v, "category_id", in.CategoryID.Value)
		changed = true
	}
	if in.TeamID.Set {
		c.teamID = normalizeRef(&v, "team_id", in.TeamID.Value)
		changed = true
	}
	if !changed {
		return c, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return c, v.Err()
}

// --- persistence ---

const selectSQL = `SELECT a.id, a.title, a.body, a.importance, a.is_pinned, a.status,
	a.category_id, c.name, c.color, a.team_id, t.name, a.published_at, a.archived_at, a.created_at, a.updated_at
	FROM announcements a
	LEFT JOIN announcement_categories c ON c.id = a.category_id
	LEFT JOIN teams t ON t.id = a.team_id `

func scan(row pgx.Row, now time.Time) (Announcement, error) {
	var a Announcement
	var catID, catName, catColor, teamID, teamName *string
	err := row.Scan(&a.ID, &a.Title, &a.Body, &a.Importance, &a.IsPinned, &a.status,
		&catID, &catName, &catColor, &teamID, &teamName, &a.PublishedAt, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if catID != nil {
		a.Category = &CategoryRef{ID: *catID, Name: *catName, Color: *catColor}
	}
	if teamID != nil {
		a.Team = &TeamRef{ID: *teamID, Name: *teamName}
	}
	a.CreatedAt, a.UpdatedAt = a.CreatedAt.UTC(), a.UpdatedAt.UTC()
	if a.PublishedAt != nil {
		p := a.PublishedAt.UTC()
		a.PublishedAt = &p
	}
	if a.ArchivedAt != nil {
		x := a.ArchivedAt.UTC()
		a.ArchivedAt = &x
	}
	a.State = stateOf(a.status, a.PublishedAt, now)
	a.Components = []component.Ref{}
	return a, nil
}

func stateOf(status string, publishedAt *time.Time, now time.Time) string {
	switch status {
	case statusDraft:
		return StateDraft
	case statusArchived:
		return StateArchived
	}
	if publishedAt != nil && publishedAt.After(now) {
		return StateScheduled
	}
	return StatePublished
}

func get(ctx context.Context, q database.Querier, id string, now time.Time, lock bool) (Announcement, error) {
	sql := selectSQL + "WHERE a.id = $1"
	if lock {
		sql += " FOR UPDATE OF a"
	}
	a, err := scan(q.QueryRow(ctx, sql, id), now)
	if errors.Is(err, pgx.ErrNoRows) {
		return Announcement{}, apperror.NotFound("announcement")
	}
	if err != nil {
		return Announcement{}, fmt.Errorf("get announcement: %w", err)
	}
	refs, err := component.RefsByOwner(ctx, q, component.OwnerAnnouncement, []string{a.ID})
	if err != nil {
		return Announcement{}, err
	}
	if r := refs[a.ID]; r != nil {
		a.Components = r
	}
	return a, nil
}

// Filter narrows a listing. Public listings only see published,
// non-scheduled announcements.
type Filter struct {
	Public      bool
	State       string
	CategoryID  string
	TeamID      string
	ComponentID string
	Pinned      *bool
	Search      string
}

var sortColumns = map[string]string{
	"published_at": "a.published_at", "updated_at": "a.updated_at", "created_at": "a.created_at", "title": "lower(a.title)",
}

func list(ctx context.Context, q database.Querier, f Filter, sort httpx.Sort, page httpx.Page, now time.Time) ([]Announcement, int, error) {
	where := `WHERE ($1 = '' OR a.category_id = NULLIF($1, '')::uuid)
		AND ($2 = '' OR a.team_id = NULLIF($2, '')::uuid)
		AND ($3 = '' OR EXISTS (SELECT 1 FROM announcement_components j WHERE j.announcement_id = a.id AND j.component_id = NULLIF($3, '')::uuid))
		AND ($4::boolean IS NULL OR a.is_pinned = $4)
		AND ($5 = '' OR a.title ILIKE '%' || $5 || '%')
		AND CASE $6
			WHEN 'draft' THEN a.status = 'draft'
			WHEN 'archived' THEN a.status = 'archived'
			WHEN 'scheduled' THEN a.status = 'published' AND a.published_at > $7
			WHEN 'published' THEN a.status = 'published' AND a.published_at <= $7
			ELSE true END `
	state := f.State
	if f.Public {
		state = StatePublished
	}
	search := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Search)
	args := []any{f.CategoryID, f.TeamID, f.ComponentID, f.Pinned, search, state, now}
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM announcements a "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count announcements: %w", err)
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	order := sortColumns[sort.Field] + " " + dir + " NULLS LAST, a.id " + dir
	// #nosec G202 -- ORDER BY comes from the sortColumns whitelist.
	rows, err := q.Query(ctx, selectSQL+where+"ORDER BY "+order+" LIMIT $8 OFFSET $9", append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list announcements: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Announcement, error) { return scan(r, now) })
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, len(out))
	for i, a := range out {
		ids[i] = a.ID
	}
	refs, err := component.RefsByOwner(ctx, q, component.OwnerAnnouncement, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		if r := refs[out[i].ID]; r != nil {
			out[i].Components = r
		}
	}
	return out, total, nil
}

// checkActiveRef ensures a referenced category or team exists and, unless
// it is the one already assigned, is active.
func checkActiveRef(ctx context.Context, q database.Querier, table, field string, id, current *string) error {
	if id == nil || (current != nil && *current == *id) {
		return nil
	}
	var active bool
	// #nosec G202 -- table is a constant chosen by the callers below.
	err := q.QueryRow(ctx, "SELECT is_active FROM "+table+" WHERE id = $1", *id).Scan(&active)
	msg := ""
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		msg = "Não encontrado."
	case err != nil:
		return fmt.Errorf("check %s: %w", field, err)
	case !active:
		msg = "Está desativado e não pode ser associado a novos conteúdos."
	}
	if msg != "" {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: msg}}}
	}
	return nil
}

func replaceComponents(ctx context.Context, tx pgx.Tx, id string, ids []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM announcement_components WHERE announcement_id = $1", id); err != nil {
		return fmt.Errorf("clear announcement components: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO announcement_components (announcement_id, component_id)
		SELECT $1, unnest($2::uuid[])`, id, ids)
	if err != nil {
		return fmt.Errorf("set announcement components: %w", err)
	}
	return nil
}

// --- service ---

// Service implements the announcement use cases.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

func refID(r *CategoryRef) *string {
	if r == nil {
		return nil
	}
	return &r.ID
}

func teamID(r *TeamRef) *string {
	if r == nil {
		return nil
	}
	return &r.ID
}

// Create stores a draft.
func (s *Service) Create(ctx context.Context, in CreateInput) (Announcement, error) {
	c, err := in.normalize()
	if err != nil {
		return Announcement{}, err
	}
	now := s.now()
	var created Announcement
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := checkActiveRef(ctx, tx, "announcement_categories", "category_id", c.categoryID, nil); err != nil {
			return err
		}
		if err := checkActiveRef(ctx, tx, "teams", "team_id", c.teamID, nil); err != nil {
			return err
		}
		refs, err := component.ResolveRefs(ctx, tx, in.ComponentIDs, nil, "component_ids")
		if err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO announcements (title, body, importance, is_pinned, category_id, team_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7) RETURNING id`,
			c.title, c.body, c.importance, c.pinned, c.categoryID, c.teamID, now).Scan(&id); err != nil {
			return fmt.Errorf("insert announcement: %w", err)
		}
		if err := replaceComponents(ctx, tx, id, component.RefIDs(refs)); err != nil {
			return err
		}
		if created, err = get(ctx, tx, id, now, false); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncement, id, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// Update edits content in any state. Published announcements show the
// update time publicly.
func (s *Service) Update(ctx context.Context, id string, in PatchInput) (Announcement, error) {
	now := s.now()
	var updated Announcement
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, now, true)
		if err != nil {
			return err
		}
		c, err := in.apply(current)
		if err != nil {
			return err
		}
		if err := checkActiveRef(ctx, tx, "announcement_categories", "category_id", c.categoryID, refID(current.Category)); err != nil {
			return err
		}
		if err := checkActiveRef(ctx, tx, "teams", "team_id", c.teamID, teamID(current.Team)); err != nil {
			return err
		}
		if in.ComponentIDs != nil {
			keep := map[string]bool{}
			for _, r := range current.Components {
				keep[r.ID] = true
			}
			refs, err := component.ResolveRefs(ctx, tx, *in.ComponentIDs, keep, "component_ids")
			if err != nil {
				return err
			}
			if err := replaceComponents(ctx, tx, id, component.RefIDs(refs)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET title = $2, body = $3, importance = $4, is_pinned = $5,
			category_id = $6, team_id = $7, updated_at = $8 WHERE id = $1`,
			id, c.title, c.body, c.importance, c.pinned, c.categoryID, c.teamID, now); err != nil {
			return fmt.Errorf("update announcement: %w", err)
		}
		if updated, err = get(ctx, tx, id, now, false); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncement, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// transition applies a lifecycle action under a row lock.
func (s *Service) transition(ctx context.Context, id, action string, fn func(a Announcement, now time.Time) (status string, publishedAt, archivedAt *time.Time, err error)) (Announcement, error) {
	now := s.now()
	var out Announcement
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, now, true)
		if err != nil {
			return err
		}
		status, publishedAt, archivedAt, err := fn(current, now)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE announcements SET status = $2, published_at = $3, archived_at = $4, updated_at = $5 WHERE id = $1`,
			id, status, publishedAt, archivedAt, now); err != nil {
			return fmt.Errorf("%s announcement: %w", action, err)
		}
		if out, err = get(ctx, tx, id, now, false); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncement, id, action, audit.Diff(current, out), now)
	})
	return out, err
}

func invalidTransition(state, allowed string) error {
	return apperror.Conflict("invalid_announcement_transition",
		fmt.Sprintf("Ação indisponível para um comunicado no estado %q (permitida apenas em: %s).", state, allowed))
}

// Publish publishes a draft now or at publish_at (scheduling).
func (s *Service) Publish(ctx context.Context, id string, in PublishInput) (Announcement, error) {
	return s.transition(ctx, id, "published", func(a Announcement, now time.Time) (string, *time.Time, *time.Time, error) {
		if a.State != StateDraft {
			return "", nil, nil, invalidTransition(a.State, "draft")
		}
		at := now
		if in.PublishAt != nil && strings.TrimSpace(*in.PublishAt) != "" {
			var v apperror.Validator
			t, err := clock.ParseInstant(*in.PublishAt, nil)
			switch {
			case err != nil:
				v.Add("publish_at", "Informe data e hora RFC 3339 com fuso (ex.: 2026-11-20T09:00:00-03:00).")
			case t.Before(now.Add(-time.Minute)):
				v.Add("publish_at", "Informe um horário futuro ou deixe em branco para publicar agora.")
			case t.After(now.Add(maxScheduleIn)):
				v.Add("publish_at", "O agendamento pode ser feito com até um ano de antecedência.")
			}
			if err := v.Err(); err != nil {
				return "", nil, nil, err
			}
			if t.After(now) {
				at = clock.Normalize(t)
			}
		}
		return statusPublished, &at, nil, nil
	})
}

// Unpublish returns a published or scheduled announcement to draft.
func (s *Service) Unpublish(ctx context.Context, id string) (Announcement, error) {
	return s.transition(ctx, id, "unpublished", func(a Announcement, _ time.Time) (string, *time.Time, *time.Time, error) {
		if a.State != StatePublished && a.State != StateScheduled {
			return "", nil, nil, invalidTransition(a.State, "published, scheduled")
		}
		return statusDraft, nil, nil, nil
	})
}

// Archive removes a published announcement from the public page while
// keeping it for reference.
func (s *Service) Archive(ctx context.Context, id string) (Announcement, error) {
	return s.transition(ctx, id, "archived", func(a Announcement, now time.Time) (string, *time.Time, *time.Time, error) {
		if a.State != StatePublished {
			return "", nil, nil, invalidTransition(a.State, "published")
		}
		return statusArchived, a.PublishedAt, &now, nil
	})
}

// Restore republishes an archived announcement with its original date.
func (s *Service) Restore(ctx context.Context, id string) (Announcement, error) {
	return s.transition(ctx, id, "restored", func(a Announcement, _ time.Time) (string, *time.Time, *time.Time, error) {
		if a.State != StateArchived {
			return "", nil, nil, invalidTransition(a.State, "archived")
		}
		return statusPublished, a.PublishedAt, nil, nil
	})
}

// Delete removes a draft.
func (s *Service) Delete(ctx context.Context, id string) error {
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, now, true)
		if err != nil {
			return err
		}
		if current.State != StateDraft {
			return apperror.Conflict("announcement_not_draft", "Somente rascunhos podem ser excluídos. Arquive o comunicado ou despublique-o antes.")
		}
		if _, err := tx.Exec(ctx, "DELETE FROM announcements WHERE id = $1", id); err != nil {
			return fmt.Errorf("delete announcement: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncement, id, "deleted", audit.Changes{Before: current}, now)
	})
}

// --- HTTP ---

// Handler exposes announcement endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts the public, read-only routes.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/announcements", h.listPublic)
	rt.Handle(http.MethodGet, "/api/v1/announcements/{id}", h.getPublic)
	rt.Handle(http.MethodGet, "/api/v1/announcement-categories", h.listPublicCategories)
}

// RegisterAdmin mounts the admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/announcements", admin(h.listAdmin))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcements", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/announcements/{id}", admin(h.getAdmin))
	rt.Handle(http.MethodPatch, "/api/v1/admin/announcements/{id}", admin(h.update))
	rt.Handle(http.MethodDelete, "/api/v1/admin/announcements/{id}", admin(h.remove))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcements/{id}/publish", admin(h.publish))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcements/{id}/unpublish", admin(h.simple(h.svc.Unpublish)))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcements/{id}/archive", admin(h.simple(h.svc.Archive)))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcements/{id}/restore", admin(h.simple(h.svc.Restore)))
	h.registerTaxonomy(rt, admin)
}

func (h *Handler) parseFilter(r *http.Request, public bool) (Filter, httpx.Sort, httpx.Page, error) {
	values := r.URL.Query()
	q := httpx.NewQuery(values)
	f := Filter{Public: public, CategoryID: q.UUID("category_id"), ComponentID: q.UUID("component_id"), Pinned: q.Bool("pinned")}
	page := q.Page()
	sort := httpx.Sort{Field: "published_at", Desc: true}
	var v apperror.Validator
	if !public {
		f.State = q.Enum("state", States...)
		f.TeamID = q.UUID("team_id")
		f.Search = strings.TrimSpace(values.Get("q"))
		v.Check(utf8.RuneCountInString(f.Search) <= 100, "q", "Informe no máximo 100 caracteres.")
		sort = q.Sort([]string{"updated_at", "created_at", "published_at", "title"}, httpx.Sort{Field: "updated_at", Desc: true})
	}
	if err := q.Err(); err != nil {
		return f, sort, page, err
	}
	return f, sort, page, v.Err()
}

func (h *Handler) listPublic(w http.ResponseWriter, r *http.Request) {
	f, sort, page, err := h.parseFilter(r, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := list(r.Context(), h.svc.pool, f, sort, page, h.svc.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]Public, len(items))
	for i, a := range items {
		out[i] = a.public()
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(out, page, total))
}

func (h *Handler) getPublic(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement")
	if err == nil {
		var a Announcement
		if a, err = get(r.Context(), h.svc.pool, id, h.svc.now(), false); err == nil {
			if a.State != StatePublished {
				err = apperror.NotFound("announcement")
			} else {
				httpx.WriteJSON(w, http.StatusOK, a.public())
				return
			}
		}
	}
	httpx.Fail(w, r, err)
}

// PublicCategory is a category offered as a public filter.
type PublicCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

func (h *Handler) listPublicCategories(w http.ResponseWriter, r *http.Request) {
	active := true
	cs, err := listCategories(r.Context(), h.svc.pool, &active)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]PublicCategory, len(cs))
	for i, c := range cs {
		out[i] = PublicCategory{ID: c.ID, Name: c.Name, Description: c.Description, Color: c.Color}
	}
	httpx.WriteJSON(w, http.StatusOK, items(out))
}

func (h *Handler) listAdmin(w http.ResponseWriter, r *http.Request) {
	f, sort, page, err := h.parseFilter(r, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := list(r.Context(), h.svc.pool, f, sort, page, h.svc.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}

func (h *Handler) getAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement")
	if err == nil {
		var a Announcement
		if a, err = get(r.Context(), h.svc.pool, id, h.svc.now(), false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, a)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/announcements/"+a.ID)
	httpx.WriteJSON(w, http.StatusCreated, a)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in PatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement")
	if err == nil {
		err = h.svc.Delete(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in PublishInput
	// The body is optional: an empty request publishes now.
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	a, err := h.svc.Publish(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) simple(fn func(context.Context, string) (Announcement, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathID(r, "announcement")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		a, err := fn(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, a)
	}
}
