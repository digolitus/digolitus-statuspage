// Package statuspage assembles the public status summary from components,
// active incidents and open maintenances, applying statuspolicy.
package statuspage

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/incident"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/maintenance"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Basis states where the published status comes from. It is always
// "declared" in this version: no automated checks exist.
const Basis = "declared"

// PublicComponent is a component as shown on the public page.
type PublicComponent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	GroupID      *string   `json:"group_id"`
	URL          *string   `json:"url"`
	DisplayOrder int       `json:"display_order"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PublicGroup is a group with at least one public component. Its status is
// derived from its components (statuspolicy.Group).
type PublicGroup struct {
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	Description                string `json:"description"`
	DisplayOrder               int    `json:"display_order"`
	Status                     string `json:"status"`
	ComponentCount             int    `json:"component_count"`
	AffectedComponents         int    `json:"affected_components"`
	ComponentsUnderMaintenance int    `json:"components_under_maintenance"`
}

// Overall is the platform-level status.
type Overall struct {
	Status                     string `json:"status"`
	Basis                      string `json:"basis"`
	AffectedComponents         int    `json:"affected_components"`
	ComponentsUnderMaintenance int    `json:"components_under_maintenance"`
}

// Maintenances groups open maintenances.
type Maintenances struct {
	InProgress []maintenance.PublicMaintenance `json:"in_progress"`
	Upcoming   []maintenance.PublicMaintenance `json:"upcoming"`
}

// Summary is the response of GET /api/v1/status.
type Summary struct {
	Overall         Overall                   `json:"overall"`
	Components      []PublicComponent         `json:"components"`
	Groups          []PublicGroup             `json:"groups"`
	ActiveIncidents []incident.PublicIncident `json:"active_incidents"`
	Maintenances    Maintenances              `json:"maintenances"`
	// ThirdParty is reported by external providers and never affects Overall.
	ThirdParty  []external.PublicSource `json:"third_party"`
	GeneratedAt time.Time               `json:"generated_at"`
}

// Service builds summaries.
type Service struct {
	pool     *pgxpool.Pool
	now      clock.Clock
	external *external.Service
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock, ext *external.Service) *Service {
	return &Service{pool: pool, now: now, external: ext}
}

// Build reads everything in one read-only, repeatable-read transaction so
// the summary is a consistent snapshot.
func (s *Service) Build(ctx context.Context) (Summary, error) {
	var sum Summary
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		now := s.now()
		comps, err := component.AllActive(ctx, tx)
		if err != nil {
			return err
		}
		incs, err := incident.Active(ctx, tx)
		if err != nil {
			return err
		}
		mts, err := maintenance.Open(ctx, tx, now)
		if err != nil {
			return err
		}
		groups, err := component.AllGroups(ctx, tx)
		if err != nil {
			return err
		}
		if sum.ThirdParty, err = s.external.Public(ctx, tx); err != nil {
			return err
		}

		states := make([]statuspolicy.ComponentState, len(comps))
		for k, c := range comps {
			states[k] = statuspolicy.ComponentState{ID: c.ID, DeclaredStatus: c.Status, Public: c.IsPublic}
		}
		active := make([]statuspolicy.ActiveIncident, len(incs))
		for k, i := range incs {
			active[k] = statuspolicy.ActiveIncident{Impact: i.Impact, ComponentIDs: component.RefIDs(i.Components)}
		}
		var running []statuspolicy.ActiveMaintenance
		sum.Maintenances = Maintenances{InProgress: []maintenance.PublicMaintenance{}, Upcoming: []maintenance.PublicMaintenance{}}
		for _, m := range mts {
			if m.Status == maintenance.StatusInProgress {
				running = append(running, statuspolicy.ActiveMaintenance{ComponentIDs: component.RefIDs(m.Components)})
				sum.Maintenances.InProgress = append(sum.Maintenances.InProgress, m.Public())
			} else {
				sum.Maintenances.Upcoming = append(sum.Maintenances.Upcoming, m.Public())
			}
		}

		effective, overall := statuspolicy.Evaluate(states, active, running)
		sum.Overall = Overall{Status: overall.Status, Basis: Basis, AffectedComponents: overall.AffectedComponents,
			ComponentsUnderMaintenance: overall.ComponentsUnderMaintenance}
		sum.Components = []PublicComponent{}
		byGroup := map[string][]string{}
		for _, c := range comps {
			if c.IsPublic {
				// Descriptions are internal unless the operator chose to show them.
				desc := ""
				if c.ShowDescription {
					desc = c.Description
				}
				sum.Components = append(sum.Components, PublicComponent{ID: c.ID, Name: c.Name, Slug: c.Slug,
					Description: desc, Status: effective[c.ID], GroupID: c.GroupID, URL: c.URL, DisplayOrder: c.DisplayOrder, UpdatedAt: c.UpdatedAt})
				if c.GroupID != nil {
					byGroup[*c.GroupID] = append(byGroup[*c.GroupID], effective[c.ID])
				}
			}
		}
		// Groups without public components are omitted from the public page.
		sum.Groups = []PublicGroup{}
		for _, g := range groups {
			statuses := byGroup[g.ID]
			if len(statuses) == 0 {
				continue
			}
			gs := statuspolicy.Group(statuses)
			sum.Groups = append(sum.Groups, PublicGroup{ID: g.ID, Name: g.Name, Description: g.Description, DisplayOrder: g.DisplayOrder,
				Status: gs.Status, ComponentCount: len(statuses), AffectedComponents: gs.AffectedComponents,
				ComponentsUnderMaintenance: gs.ComponentsUnderMaintenance})
		}
		sum.ActiveIncidents = make([]incident.PublicIncident, len(incs))
		for k, i := range incs {
			sum.ActiveIncidents[k] = i.Public()
		}
		sum.GeneratedAt = now
		return nil
	})
	return sum, err
}

// Handler exposes the public summary endpoints.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts public routes.
func (h *Handler) Register(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/status", h.status)
	rt.Handle(http.MethodGet, "/api/v1/components", h.components)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	sum, err := h.svc.Build(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sum)
}

// components lists public components with their effective status. The full
// set is evaluated (it is small by nature) and then paginated.
func (h *Handler) components(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sum, err := h.svc.Build(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	all := sum.Components
	total := len(all)
	start := min(page.Offset, total)
	end := min(start+page.Limit, total)
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(all[start:end], page, total))
}
