-- Local authentication and role-based access control for the admin area.
--
-- Permissions are a fixed catalogue defined in code (internal/auth); a role
-- stores the subset it grants. The system role "Administrador" grants every
-- permission, including ones added in future versions, and cannot be edited.

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    permissions text[]      NOT NULL DEFAULT '{}' CHECK (cardinality(permissions) <= 100),
    is_system   boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX roles_name_key ON roles (lower(name));
CREATE UNIQUE INDEX roles_single_system ON roles (is_system) WHERE is_system;

CREATE TABLE users (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email               text        NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254 AND email ~ '^[^@\s]+@[^@\s]+$'),
    name                text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    password_hash       text        NOT NULL,
    is_active           boolean     NOT NULL DEFAULT true,
    failed_logins       integer     NOT NULL DEFAULT 0 CHECK (failed_logins >= 0),
    locked_until        timestamptz,
    last_login_at       timestamptz,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    -- Set when an administrator chose the password (creation or reset): the
    -- user must pick their own before using the admin area.
    must_change_password boolean    NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- RESTRICT: a role in use cannot be deleted.
    role_id uuid NOT NULL CONSTRAINT user_roles_role_id_fkey REFERENCES roles (id) ON DELETE RESTRICT,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX user_roles_role_idx ON user_roles (role_id);

-- Server-side sessions. Only a SHA-256 of the cookie token is stored, so a
-- database leak does not expose usable session tokens.
CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash   bytea       NOT NULL UNIQUE,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    user_agent   text        NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 300)
);

CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- One-time tokens that authorise creating the first administrator. They are
-- generated at startup while no user exists and removed once setup is done.
CREATE TABLE setup_tokens (
    token_hash bytea       PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO roles (name, description, is_system) VALUES
('Administrador', 'Acesso total à área administrativa, inclusive usuários e papéis. Concedido ao primeiro usuário.', true);

-- Starting points; ordinary roles that can be edited or deleted.
INSERT INTO roles (name, description, permissions) VALUES
('Operador de incidentes',
 'Comunica indisponibilidades: incidentes, manutenções e status dos componentes. Não altera o cadastro nem comunicados.',
 '{incidents.manage,maintenances.manage,components.status}'),
('Gestor de catálogo e comunicados',
 'Administra componentes, grupos, serviços de terceiros, templates e comunicados. Sem acesso a usuários e papéis.',
 '{components.manage,components.status,third_party.manage,templates.manage,announcements.write,announcements.publish,announcements.settings}');

ALTER TABLE audit_events
    ADD COLUMN actor_id uuid,
    ADD COLUMN actor_name text CHECK (actor_name IS NULL OR char_length(actor_name) <= 200);

CREATE INDEX audit_events_actor_idx ON audit_events (actor_id) WHERE actor_id IS NOT NULL;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role'));
