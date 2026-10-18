-- Teams become owners of components and groups, and users can belong to
-- teams. Roles gain a scope: "global" (whole platform) or "team" (only the
-- components owned by the user's teams).

ALTER TABLE teams
    ADD COLUMN kind text NOT NULL DEFAULT '' CHECK (char_length(kind) <= 40),
    ADD COLUMN documentation_url text CHECK (documentation_url IS NULL OR (char_length(documentation_url) <= 500 AND documentation_url ~ '^https?://')),
    ADD COLUMN support_url text CHECK (support_url IS NULL OR (char_length(support_url) <= 500 AND support_url ~ '^https?://'));

-- Ownership. A component without its own team inherits its group's team.
-- RESTRICT: a team that owns something is deactivated, not deleted.
ALTER TABLE component_groups
    ADD COLUMN team_id uuid CONSTRAINT component_groups_team_id_fkey REFERENCES teams (id) ON DELETE RESTRICT;
ALTER TABLE components
    ADD COLUMN team_id uuid CONSTRAINT components_team_id_fkey REFERENCES teams (id) ON DELETE RESTRICT;

CREATE INDEX component_groups_team_idx ON component_groups (team_id) WHERE team_id IS NOT NULL;
CREATE INDEX components_team_idx ON components (team_id) WHERE team_id IS NOT NULL;

CREATE TABLE user_teams (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, team_id)
);

CREATE INDEX user_teams_team_idx ON user_teams (team_id);

ALTER TABLE roles
    ADD COLUMN scope text NOT NULL DEFAULT 'global' CHECK (scope IN ('global', 'team')),
    ADD CONSTRAINT roles_system_is_global CHECK (NOT is_system OR scope = 'global');

-- Team management moved from "announcements.settings" to its own
-- permission; roles that could manage teams keep that ability.
UPDATE roles SET permissions = array_append(permissions, 'teams.manage')
WHERE 'announcements.settings' = ANY (permissions) AND NOT ('teams.manage' = ANY (permissions));

-- Starting point for squads operating only their own components.
INSERT INTO roles (name, description, permissions, scope) VALUES
('Operador do time',
 'Opera apenas os componentes dos seus times: status, incidentes, manutenções e comunicados do time.',
 '{components.status,incidents.manage,maintenances.manage,announcements.write,announcements.publish}', 'team')
ON CONFLICT DO NOTHING;
