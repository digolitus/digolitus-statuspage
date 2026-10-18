-- Initial schema for components, incidents, maintenances, templates and the
-- operational audit log. Current state (e.g. incidents.status) is kept apart
-- from history (incident_updates, maintenance_updates, audit_events), which is
-- append-only from the application's point of view.

CREATE TABLE components (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    slug          text        NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND char_length(slug) <= 80),
    description   text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    status        text        NOT NULL DEFAULT 'operational'
                  CHECK (status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance')),
    is_public     boolean     NOT NULL DEFAULT true,
    is_active     boolean     NOT NULL DEFAULT true,
    display_order integer     NOT NULL DEFAULT 0 CHECK (display_order BETWEEN 0 AND 100000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT components_slug_key UNIQUE (slug)
);

CREATE INDEX components_listing_idx ON components (display_order, name, id);

CREATE TABLE incident_templates (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    title       text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    message     text        NOT NULL CHECK (char_length(message) BETWEEN 1 AND 5000),
    impact      text        NOT NULL CHECK (impact IN ('minor', 'major', 'critical')),
    status      text        NOT NULL CHECK (status IN ('investigating', 'identified', 'monitoring', 'resolved')),
    guidance    text        NOT NULL DEFAULT '' CHECK (char_length(guidance) <= 2000),
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX incident_templates_name_key ON incident_templates (lower(name));

CREATE TABLE incidents (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title          text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    impact         text        NOT NULL CHECK (impact IN ('minor', 'major', 'critical')),
    status         text        NOT NULL CHECK (status IN ('investigating', 'identified', 'monitoring', 'resolved')),
    internal_notes text        NOT NULL DEFAULT '' CHECK (char_length(internal_notes) <= 5000),
    started_at     timestamptz NOT NULL,
    resolved_at    timestamptz,
    template_id    uuid REFERENCES incident_templates (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT incidents_resolution_consistent CHECK ((status = 'resolved') = (resolved_at IS NOT NULL)),
    CONSTRAINT incidents_resolved_after_start CHECK (resolved_at IS NULL OR resolved_at >= started_at)
);

CREATE INDEX incidents_started_idx ON incidents (started_at DESC, id DESC);
CREATE INDEX incidents_active_idx ON incidents (started_at DESC, id DESC) WHERE status <> 'resolved';

-- Components currently associated with an incident.
CREATE TABLE incident_components (
    incident_id  uuid NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    component_id uuid NOT NULL REFERENCES components (id) ON DELETE RESTRICT,
    PRIMARY KEY (incident_id, component_id)
);

CREATE INDEX incident_components_component_idx ON incident_components (component_id);

-- Published timeline. Rows are never updated: each update keeps the status
-- and message as they were published. component_ids is a snapshot of the
-- affected components when they changed with this update (NULL = unchanged).
CREATE TABLE incident_updates (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id   uuid        NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    status        text        NOT NULL CHECK (status IN ('investigating', 'identified', 'monitoring', 'resolved')),
    message       text        NOT NULL CHECK (char_length(message) BETWEEN 1 AND 5000),
    component_ids uuid[],
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX incident_updates_timeline_idx ON incident_updates (incident_id, created_at DESC, id DESC);

CREATE TABLE maintenance_templates (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description     text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    title           text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    message         text        NOT NULL CHECK (char_length(message) BETWEEN 1 AND 5000),
    expected_impact text        NOT NULL CHECK (expected_impact IN ('none', 'degraded_performance', 'partial_outage', 'major_outage')),
    guidance        text        NOT NULL DEFAULT '' CHECK (char_length(guidance) <= 2000),
    is_active       boolean     NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX maintenance_templates_name_key ON maintenance_templates (lower(name));

-- Instants are stored as timestamptz (UTC). timezone keeps the IANA zone the
-- window was planned in, for presentation.
CREATE TABLE maintenances (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title           text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description     text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    expected_impact text        NOT NULL CHECK (expected_impact IN ('none', 'degraded_performance', 'partial_outage', 'major_outage')),
    status          text        NOT NULL CHECK (status IN ('scheduled', 'in_progress', 'completed', 'cancelled')),
    timezone        text        NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    scheduled_start timestamptz NOT NULL,
    scheduled_end   timestamptz NOT NULL,
    started_at      timestamptz,
    completed_at    timestamptz,
    cancelled_at    timestamptz,
    internal_notes  text        NOT NULL DEFAULT '' CHECK (char_length(internal_notes) <= 5000),
    template_id     uuid REFERENCES maintenance_templates (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT maintenances_window_valid CHECK (scheduled_end > scheduled_start),
    CONSTRAINT maintenances_lifecycle_consistent CHECK (
        (status = 'scheduled'   AND started_at IS NULL     AND completed_at IS NULL     AND cancelled_at IS NULL) OR
        (status = 'in_progress' AND started_at IS NOT NULL AND completed_at IS NULL     AND cancelled_at IS NULL) OR
        (status = 'completed'   AND started_at IS NOT NULL AND completed_at IS NOT NULL AND cancelled_at IS NULL) OR
        (status = 'cancelled'   AND started_at IS NULL     AND completed_at IS NULL     AND cancelled_at IS NOT NULL)
    ),
    CONSTRAINT maintenances_completed_after_start CHECK (completed_at IS NULL OR completed_at >= started_at)
);

CREATE INDEX maintenances_schedule_idx ON maintenances (scheduled_start, id);
CREATE INDEX maintenances_open_idx ON maintenances (scheduled_start, id) WHERE status IN ('scheduled', 'in_progress');

CREATE TABLE maintenance_components (
    maintenance_id uuid NOT NULL REFERENCES maintenances (id) ON DELETE CASCADE,
    component_id   uuid NOT NULL REFERENCES components (id) ON DELETE RESTRICT,
    PRIMARY KEY (maintenance_id, component_id)
);

CREATE INDEX maintenance_components_component_idx ON maintenance_components (component_id);

-- Published maintenance timeline; message may be empty for pure transitions.
CREATE TABLE maintenance_updates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    maintenance_id uuid        NOT NULL REFERENCES maintenances (id) ON DELETE CASCADE,
    kind           text        NOT NULL CHECK (kind IN ('created', 'message', 'started', 'completed', 'cancelled')),
    status         text        NOT NULL CHECK (status IN ('scheduled', 'in_progress', 'completed', 'cancelled')),
    message        text        CHECK (message IS NULL OR char_length(message) BETWEEN 1 AND 5000),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX maintenance_updates_timeline_idx ON maintenance_updates (maintenance_id, created_at DESC, id DESC);

-- Operational audit log. source identifies the channel (e.g. admin_api); no
-- human identity exists until authentication is introduced.
CREATE TABLE audit_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    resource_type text        NOT NULL CHECK (resource_type IN ('component', 'incident', 'maintenance', 'incident_template', 'maintenance_template')),
    resource_id   uuid        NOT NULL,
    action        text        NOT NULL CHECK (char_length(action) BETWEEN 1 AND 64),
    source        text        NOT NULL CHECK (char_length(source) BETWEEN 1 AND 64),
    request_id    text        CHECK (request_id IS NULL OR char_length(request_id) <= 128),
    changes       jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX audit_events_recent_idx ON audit_events (occurred_at DESC, id DESC);
CREATE INDEX audit_events_resource_idx ON audit_events (resource_type, resource_id, occurred_at DESC, id DESC);
