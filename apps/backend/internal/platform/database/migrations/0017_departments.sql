-- Optional department hierarchy above teams (ADR 0017): up to four levels,
-- each team in at most one department, people either through their teams
-- or directly in a department. Organisational only: no permission depends
-- on it.

-- Labels for the levels (1 = top). A missing row shows as "Nível N".
CREATE TABLE department_levels (
    level smallint PRIMARY KEY CHECK (level BETWEEN 1 AND 4),
    name  text     NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40)
);

-- The level of a department is its depth in the tree. The service keeps the
-- depth at most four and refuses cycles; parent_id RESTRICT keeps a
-- department with children from being deleted.
CREATE TABLE departments (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   uuid        CONSTRAINT departments_parent_id_fkey REFERENCES departments (id) ON DELETE RESTRICT,
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT departments_parent_not_self CHECK (parent_id IS NULL OR parent_id <> id)
);

-- Sibling names are unique (case-insensitive), so "Infraestrutura" can
-- exist under two different areas.
CREATE UNIQUE INDEX departments_name_key ON departments (parent_id, lower(name)) NULLS NOT DISTINCT;

ALTER TABLE teams
    ADD COLUMN department_id uuid CONSTRAINT teams_department_id_fkey REFERENCES departments (id) ON DELETE RESTRICT;
CREATE INDEX teams_department_idx ON teams (department_id) WHERE department_id IS NOT NULL;

-- People placed directly in a department (e.g. a manager without a team).
CREATE TABLE user_departments (
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    department_id uuid NOT NULL REFERENCES departments (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, department_id)
);
CREATE INDEX user_departments_department_idx ON user_departments (department_id);

-- Free job title shown next to the person (e.g. "Gerente").
ALTER TABLE users ADD COLUMN job_title text NOT NULL DEFAULT '' CHECK (char_length(job_title) <= 60);

-- The list also carries monitoring_integration (0015) and branding (0016)
-- so the constraint is right whichever migration runs last.
ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription', 'announcement_template', 'monitoring_integration',
                             'branding', 'department'));
