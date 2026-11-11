// Package department organises teams and people in an optional hierarchy of
// up to four levels (ADR 0017). It is organisational only: no permission
// or ownership rule depends on it.
package department

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/optional"
)

// Limits.
const (
	MaxDepth       = 4
	maxDepartments = 200
	maxMembers     = 200
	maxLevelName   = 40
)

// levelsResourceID identifies the level labels in the audit log (they are
// a singleton, not a row with its own id).
const levelsResourceID = "00000000-0000-0000-0000-000000000000"

// Ref is an ancestor in a department's path.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Member is a person placed directly in a department.
type Member struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	JobTitle string `json:"job_title"`
	IsActive bool   `json:"is_active"`
}

// Department is one node of the hierarchy.
type Department struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ParentID    *string `json:"parent_id"`
	// Level is the depth in the tree (1 = top).
	Level int `json:"level"`
	// Path goes from the top-level department down to this one (inclusive).
	Path []Ref `json:"path"`
	// Members are the people placed directly here (not through a team).
	Members []Member `json:"members"`
	// Counts. Direct counts cover this department only; total counts
	// include every department below it. TotalPeopleCount counts each
	// person once, whether they are here directly or through a team.
	ChildCount       int       `json:"child_count"`
	TeamCount        int       `json:"team_count"`
	TotalTeamCount   int       `json:"total_team_count"`
	TotalPeopleCount int       `json:"total_people_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Levels are the labels of the four levels ("" = unnamed).
type Levels struct {
	Names []string `json:"names"`
}

// Input creates (Name required) or patches (nil/absent = unchanged).
type Input struct {
	Name        *string                   `json:"name"`
	Description *string                   `json:"description"`
	ParentID    optional.Nullable[string] `json:"parent_id"`
	// MemberIDs replace the people placed directly here when present.
	MemberIDs *[]string `json:"member_ids"`
}

// Service manages departments.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

// --- reading ---

type row struct {
	Department
	parent string
}

// load reads the whole hierarchy (bounded by maxDepartments) and derives
// levels, paths and counts. It returns the departments in tree order.
func load(ctx context.Context, q database.Querier) ([]Department, error) {
	rows, err := q.Query(ctx, `SELECT d.id, d.name, d.description, d.parent_id, d.created_at, d.updated_at,
		(SELECT count(*) FROM teams t WHERE t.department_id = d.id)::int,
		COALESCE((SELECT json_agg(json_build_object('id', u.id, 'name', u.name, 'email', u.email, 'job_title', u.job_title, 'is_active', u.is_active)
			ORDER BY lower(u.name), u.id) FROM user_departments ud JOIN users u ON u.id = ud.user_id WHERE ud.department_id = d.id), '[]')
		FROM departments d`)
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (*row, error) {
		var d row
		err := r.Scan(&d.ID, &d.Name, &d.Description, &d.ParentID, &d.CreatedAt, &d.UpdatedAt, &d.TeamCount, &d.Members)
		d.CreatedAt, d.UpdatedAt = d.CreatedAt.UTC(), d.UpdatedAt.UTC()
		if d.ParentID != nil {
			d.parent = *d.ParentID
		}
		return &d, err
	})
	if err != nil {
		return nil, err
	}

	// People per department: direct members plus members of its teams.
	prow, err := q.Query(ctx, `SELECT department_id, user_id FROM user_departments
		UNION SELECT t.department_id, ut.user_id FROM teams t JOIN user_teams ut ON ut.team_id = t.id WHERE t.department_id IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list department people: %w", err)
	}
	people := map[string]map[string]bool{}
	for prow.Next() {
		var dep, user string
		if err := prow.Scan(&dep, &user); err != nil {
			prow.Close()
			return nil, err
		}
		if people[dep] == nil {
			people[dep] = map[string]bool{}
		}
		people[dep][user] = true
	}
	prow.Close()
	if err := prow.Err(); err != nil {
		return nil, err
	}

	children := map[string][]*row{}
	for _, d := range all {
		children[d.parent] = append(children[d.parent], d)
	}
	for _, cs := range children {
		slices.SortFunc(cs, func(a, b *row) int {
			return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.ID, b.ID))
		})
	}
	out := make([]Department, 0, len(all))
	// walk fills levels and paths top-down, and totals bottom-up; it
	// returns the people of the subtree.
	var walk func(d *row, path []Ref) map[string]bool
	walk = func(d *row, path []Ref) map[string]bool {
		d.Path = append(slices.Clone(path), Ref{ID: d.ID, Name: d.Name})
		d.Level = len(d.Path)
		d.ChildCount = len(children[d.ID])
		d.TotalTeamCount = d.TeamCount
		idx := len(out)
		out = append(out, Department{})
		set := map[string]bool{}
		for u := range people[d.ID] {
			set[u] = true
		}
		for _, c := range children[d.ID] {
			for u := range walk(c, d.Path) {
				set[u] = true
			}
			d.TotalTeamCount += c.TotalTeamCount
		}
		d.TotalPeopleCount = len(set)
		out[idx] = d.Department
		return set
	}
	for _, root := range children[""] {
		walk(root, nil)
	}
	return out, nil
}

func find(ds []Department, id string) (Department, bool) {
	i := slices.IndexFunc(ds, func(d Department) bool { return d.ID == id })
	if i < 0 {
		return Department{}, false
	}
	return ds[i], true
}

// List returns every department in tree order.
func (s *Service) List(ctx context.Context) ([]Department, error) {
	return load(ctx, s.pool)
}

// Get returns one department.
func (s *Service) Get(ctx context.Context, id string) (Department, error) {
	ds, err := load(ctx, s.pool)
	if err != nil {
		return Department{}, err
	}
	d, ok := find(ds, id)
	if !ok {
		return Department{}, apperror.NotFound("department")
	}
	return d, nil
}

// GetLevels returns the four level labels.
func (s *Service) GetLevels(ctx context.Context) (Levels, error) {
	return getLevels(ctx, s.pool)
}

func getLevels(ctx context.Context, q database.Querier) (Levels, error) {
	names := make([]string, MaxDepth)
	rows, err := q.Query(ctx, "SELECT level, name FROM department_levels")
	if err != nil {
		return Levels{}, fmt.Errorf("list department levels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level int
		var name string
		if err := rows.Scan(&level, &name); err != nil {
			return Levels{}, err
		}
		names[level-1] = name
	}
	return Levels{Names: names}, rows.Err()
}

// --- writing ---

// SetLevels replaces the level labels.
func (s *Service) SetLevels(ctx context.Context, in Levels) (Levels, error) {
	var v apperror.Validator
	v.Check(len(in.Names) <= MaxDepth, "names", fmt.Sprintf("Informe no máximo %d níveis.", MaxDepth))
	names := make([]string, MaxDepth)
	for i, n := range in.Names {
		if i >= MaxDepth {
			break
		}
		names[i] = strings.Join(strings.Fields(n), " ")
		v.Check(utf8.RuneCountInString(names[i]) <= maxLevelName, fmt.Sprintf("names[%d]", i), fmt.Sprintf("Use no máximo %d caracteres.", maxLevelName))
	}
	if err := v.Err(); err != nil {
		return Levels{}, err
	}
	out := Levels{Names: names}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getLevels(ctx, tx)
		if err != nil {
			return err
		}
		if slices.Equal(current.Names, names) {
			return nil
		}
		if _, err := tx.Exec(ctx, "DELETE FROM department_levels"); err != nil {
			return fmt.Errorf("clear department levels: %w", err)
		}
		for i, n := range names {
			if n == "" {
				continue
			}
			if _, err := tx.Exec(ctx, "INSERT INTO department_levels (level, name) VALUES ($1, $2)", i+1, n); err != nil {
				return fmt.Errorf("set department level: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.ResourceDepartment, levelsResourceID, "levels_updated", audit.Diff(current, out), s.now())
	})
	return out, err
}

// lockTree serialises structural changes so depth and cycle checks see a
// stable tree. Reads and team edits are not blocked.
func lockTree(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "LOCK TABLE departments IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("lock departments: %w", err)
	}
	return nil
}

func checkText(v *apperror.Validator, name, description *string, create bool) {
	if name != nil || create {
		n := ""
		if name != nil {
			n = *name
		}
		v.Check(n != "", "name", "Informe o nome.")
		v.Check(utf8.RuneCountInString(n) <= 80, "name", "Use no máximo 80 caracteres.")
	}
	if description != nil {
		v.Check(utf8.RuneCountInString(*description) <= 500, "description", "Use no máximo 500 caracteres.")
	}
}

func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	t := strings.TrimSpace(*p)
	return &t
}

func uniqueIDs(v *apperror.Validator, field string, ids []string) []string {
	out := []string{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !httpx.IsUUID(id) {
			v.Add(field, "Identificador inválido.")
			return nil
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	v.Check(len(out) <= maxMembers, field, fmt.Sprintf("Informe no máximo %d pessoas.", maxMembers))
	return out
}

// height is the number of levels of the subtree rooted at d (1 = leaf).
func height(ds []Department, d Department) int {
	h := 1
	for _, x := range ds {
		if len(x.Path) > len(d.Path) && x.Path[len(d.Path)-1].ID == d.ID {
			h = max(h, len(x.Path)-len(d.Path)+1)
		}
	}
	return h
}

// checkParent validates a new parent for d (d.ID == "" when creating).
func checkParent(ds []Department, d Department, parentID *string) error {
	fieldErr := func(msg string) error {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "parent_id", Message: msg}}}
	}
	level := 1
	if parentID != nil {
		parent, ok := find(ds, *parentID)
		if !ok {
			return fieldErr("O departamento superior não existe.")
		}
		if d.ID != "" && slices.ContainsFunc(parent.Path, func(r Ref) bool { return r.ID == d.ID }) {
			return fieldErr("Um departamento não pode ficar abaixo dele mesmo nem de um dos seus subdepartamentos.")
		}
		level = parent.Level + 1
	}
	h := 1
	if d.ID != "" {
		h = height(ds, d)
	}
	if level+h-1 > MaxDepth {
		if h > 1 {
			return fieldErr(fmt.Sprintf("Com os subdepartamentos, a estrutura passaria de %d níveis.", MaxDepth))
		}
		return fieldErr(fmt.Sprintf("A estrutura tem no máximo %d níveis.", MaxDepth))
	}
	return nil
}

func nameConflict(err error) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "departments_name_key" {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "department_name_conflict", Message: "Já existe um departamento com este nome neste nível.",
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	return err
}

func setMembers(ctx context.Context, tx pgx.Tx, id string, userIDs []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM user_departments WHERE department_id = $1", id); err != nil {
		return fmt.Errorf("clear department members: %w", err)
	}
	if len(userIDs) == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, "INSERT INTO user_departments (user_id, department_id) SELECT u.id, $1 FROM users u WHERE u.id = ANY($2::uuid[])", id, userIDs)
	if err != nil {
		return fmt.Errorf("set department members: %w", err)
	}
	if int(tag.RowsAffected()) != len(userIDs) {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "member_ids", Message: "Uma ou mais pessoas não existem."}}}
	}
	return nil
}

// Create stores a new department.
func (s *Service) Create(ctx context.Context, in Input) (Department, error) {
	var v apperror.Validator
	name, description := trimmed(in.Name), trimmed(in.Description)
	checkText(&v, name, description, true)
	var parentID *string
	if in.ParentID.Value != nil {
		p := strings.ToLower(*in.ParentID.Value)
		v.Check(httpx.IsUUID(p), "parent_id", "Identificador inválido.")
		parentID = &p
	}
	var members []string
	if in.MemberIDs != nil {
		members = uniqueIDs(&v, "member_ids", *in.MemberIDs)
	}
	if err := v.Err(); err != nil {
		return Department{}, err
	}
	if description == nil {
		description = new(string)
	}
	now := s.now()
	var created Department
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockTree(ctx, tx); err != nil {
			return err
		}
		ds, err := load(ctx, tx)
		if err != nil {
			return err
		}
		if len(ds) >= maxDepartments {
			return apperror.Conflict("department_limit", fmt.Sprintf("Limite de %d departamentos atingido.", maxDepartments))
		}
		if err := checkParent(ds, Department{}, parentID); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO departments (parent_id, name, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4) RETURNING id`, parentID, *name, *description, now).Scan(&id); err != nil {
			return nameConflict(err)
		}
		if err := setMembers(ctx, tx, id, members); err != nil {
			return err
		}
		if ds, err = load(ctx, tx); err != nil {
			return err
		}
		created, _ = find(ds, id)
		return audit.Record(ctx, tx, audit.ResourceDepartment, id, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// Update patches a department. Moving it takes its subdepartments, teams
// and people along, as long as the tree stays within MaxDepth levels.
func (s *Service) Update(ctx context.Context, id string, in Input) (Department, error) {
	var v apperror.Validator
	name, description := trimmed(in.Name), trimmed(in.Description)
	checkText(&v, name, description, false)
	var parentID *string
	if in.ParentID.Value != nil {
		p := strings.ToLower(*in.ParentID.Value)
		v.Check(httpx.IsUUID(p), "parent_id", "Identificador inválido.")
		parentID = &p
	}
	var members []string
	if in.MemberIDs != nil {
		members = uniqueIDs(&v, "member_ids", *in.MemberIDs)
	}
	if name == nil && description == nil && !in.ParentID.Set && in.MemberIDs == nil {
		return Department{}, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	if err := v.Err(); err != nil {
		return Department{}, err
	}
	now := s.now()
	var updated Department
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockTree(ctx, tx); err != nil {
			return err
		}
		ds, err := load(ctx, tx)
		if err != nil {
			return err
		}
		current, ok := find(ds, id)
		if !ok {
			return apperror.NotFound("department")
		}
		next := current
		if name != nil {
			next.Name = *name
		}
		if description != nil {
			next.Description = *description
		}
		if in.ParentID.Set {
			if err := checkParent(ds, current, parentID); err != nil {
				return err
			}
			next.ParentID = parentID
		}
		if _, err := tx.Exec(ctx, `UPDATE departments SET name = $2, description = $3, parent_id = $4, updated_at = $5 WHERE id = $1`,
			id, next.Name, next.Description, next.ParentID, now); err != nil {
			return nameConflict(err)
		}
		if in.MemberIDs != nil {
			if err := setMembers(ctx, tx, id, members); err != nil {
				return err
			}
		}
		if ds, err = load(ctx, tx); err != nil {
			return err
		}
		updated, _ = find(ds, id)
		action := "updated"
		if !equalPtr(current.ParentID, updated.ParentID) {
			action = "moved"
		}
		return audit.Record(ctx, tx, audit.ResourceDepartment, id, action, audit.Diff(current, updated), now)
	})
	return updated, err
}

func equalPtr(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// Delete removes a department without subdepartments or teams. People
// placed directly in it are simply unlinked.
func (s *Service) Delete(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockTree(ctx, tx); err != nil {
			return err
		}
		ds, err := load(ctx, tx)
		if err != nil {
			return err
		}
		current, ok := find(ds, id)
		if !ok {
			return apperror.NotFound("department")
		}
		if current.ChildCount > 0 || current.TeamCount > 0 {
			return apperror.Conflict("department_in_use", fmt.Sprintf(
				"O departamento tem %d subdepartamento(s) e %d time(s). Mova-os ou exclua-os antes.", current.ChildCount, current.TeamCount))
		}
		if _, err := tx.Exec(ctx, "DELETE FROM departments WHERE id = $1", id); err != nil {
			if _, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok {
				return apperror.Conflict("department_in_use", "O departamento ainda tem subdepartamentos ou times.")
			}
			return fmt.Errorf("delete department: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceDepartment, id, "deleted", audit.Changes{Before: current}, s.now())
	})
}

// IsUnknownDepartment reports whether err is the foreign-key violation of a
// team pointing at a department that does not exist.
func IsUnknownDepartment(err error) bool {
	name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation)
	return ok && name == "teams_department_id_fkey"
}
