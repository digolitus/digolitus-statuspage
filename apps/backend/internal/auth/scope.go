package auth

import (
	"context"
	"fmt"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Resource-level checks for team-scoped roles.
//
// The admin guard already verified that the principal holds the route's
// permission in some scope. When it is held only for the user's teams,
// domain services call these helpers to confirm that every resource touched
// is owned by one of those teams. A component's owner is its own team or,
// when unset, its group's team.
//
// Without a principal in the context (server-side callers such as tests or
// commands) the checks pass: the HTTP admin API always has one.

func scopeDenied(msg string) error {
	return &apperror.Error{Kind: apperror.KindForbidden, Code: "outside_team_scope", Message: msg}
}

// teamOnly returns the principal when perm is held only for its teams, or
// nil when no resource check is needed (global grant or no principal).
func teamOnly(ctx context.Context, perm string) (*Principal, error) {
	p := FromContext(ctx)
	if p == nil || p.HasGlobal(perm) {
		return nil, nil
	}
	if !p.teamPerms[perm] {
		return nil, apperror.Forbidden("")
	}
	return p, nil
}

// RequireComponents checks that perm covers all componentIDs. With a team
// grant the list must be non-empty and every component owned by the user's
// teams (actions without components affect the whole platform).
func RequireComponents(ctx context.Context, q database.Querier, perm string, componentIDs []string) error {
	p, err := teamOnly(ctx, perm)
	if p == nil || err != nil {
		return err
	}
	if len(componentIDs) == 0 {
		return scopeDenied("Seu papel vale apenas para componentes dos seus times: selecione ao menos um componente do seu time.")
	}
	var outside []string
	rows, err := q.Query(ctx, `SELECT c.name FROM components c LEFT JOIN component_groups g ON g.id = c.group_id
		WHERE c.id = ANY($1::uuid[]) AND (COALESCE(c.team_id, g.team_id) IS NULL OR NOT (COALESCE(c.team_id, g.team_id)::text = ANY($2)))
		ORDER BY c.name`, componentIDs, p.Teams)
	if err != nil {
		return fmt.Errorf("check component ownership: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		outside = append(outside, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(outside) > 0 {
		return scopeDenied(fmt.Sprintf("Seu papel vale apenas para componentes dos seus times. Fora do seu escopo: %v.", outside))
	}
	return nil
}

// RequireTeam checks that perm covers a resource owned by teamID (e.g. a
// group or an announcement). A nil team is outside every team scope.
func RequireTeam(ctx context.Context, perm string, teamID *string) error {
	p, err := teamOnly(ctx, perm)
	if p == nil || err != nil {
		return err
	}
	if teamID == nil || !p.inTeam(*teamID) {
		return scopeDenied("Seu papel vale apenas para itens dos seus times: escolha um dos seus times como responsável.")
	}
	return nil
}

// RequireGroup checks that perm covers the group (owned by its team).
func RequireGroup(ctx context.Context, q database.Querier, perm, groupID string) error {
	p, err := teamOnly(ctx, perm)
	if p == nil || err != nil {
		return err
	}
	var team *string
	if err := q.QueryRow(ctx, "SELECT team_id::text FROM component_groups WHERE id = $1", groupID).Scan(&team); err != nil {
		return fmt.Errorf("check group ownership: %w", err)
	}
	return RequireTeam(ctx, perm, team)
}

// TeamScoped reports whether perm is held only for the user's teams, and
// those teams (for filtering lists or defaults in handlers).
func TeamScoped(ctx context.Context, perm string) (bool, []string) {
	p := FromContext(ctx)
	if p == nil || p.HasGlobal(perm) || !p.teamPerms[perm] {
		return false, nil
	}
	return true, p.Teams
}
