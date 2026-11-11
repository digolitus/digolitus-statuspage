-- Status history (ADR 0014).
--
-- component_status_changes records every change of a component's EFFECTIVE
-- status (declared status combined with active incidents and in-progress
-- maintenances, see statuspolicy), written in the transaction of the change.
-- Rows are append-only. The latest row per component is the status in force
-- since changed_at; status NULL means the component was deactivated (no data
-- for that period). Daily uptime is derived from these intervals.
CREATE TABLE component_status_changes (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    component_id    uuid        NOT NULL REFERENCES components (id),
    status          text        CHECK (status IS NULL OR status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance')),
    previous_status text        CHECK (previous_status IS NULL OR previous_status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance')),
    -- What triggered the recalculation. baseline = first record of a component.
    cause_type      text        NOT NULL CHECK (cause_type IN ('baseline', 'component', 'incident', 'maintenance')),
    cause_id        uuid,
    -- Who made the change (null for the system or the baseline).
    actor_id        uuid,
    actor_name      text CHECK (actor_name IS NULL OR char_length(actor_name) <= 200),
    changed_at      timestamptz NOT NULL
);

CREATE INDEX component_status_changes_component_idx ON component_status_changes (component_id, changed_at DESC, id DESC);
CREATE INDEX component_status_changes_time_idx ON component_status_changes (changed_at DESC, id DESC);

-- Status changes reported by third-party providers, detected by the poller.
-- Names are copied so the record survives the source being deleted.
CREATE TABLE external_component_status_changes (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id             uuid        REFERENCES external_sources (id) ON DELETE SET NULL,
    external_component_id uuid        REFERENCES external_components (id) ON DELETE SET NULL,
    source_name           text        NOT NULL CHECK (char_length(source_name) BETWEEN 1 AND 120),
    component_name        text        NOT NULL CHECK (char_length(component_name) BETWEEN 1 AND 255),
    status                text        NOT NULL
                          CHECK (status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance', 'unknown')),
    previous_status       text
                          CHECK (previous_status IS NULL OR previous_status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance', 'unknown')),
    changed_at            timestamptz NOT NULL
);

CREATE INDEX external_component_status_changes_source_idx ON external_component_status_changes (source_id, changed_at DESC, id DESC);
CREATE INDEX external_component_status_changes_component_idx ON external_component_status_changes (external_component_id, changed_at DESC, id DESC);
CREATE INDEX external_component_status_changes_time_idx ON external_component_status_changes (changed_at DESC, id DESC);

-- Incidents and maintenances seen on a provider's page: first and last time
-- they were seen open, and when they stopped being reported (closed_at).
CREATE TABLE external_events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id           uuid        REFERENCES external_sources (id) ON DELETE SET NULL,
    source_name         text        NOT NULL CHECK (char_length(source_name) BETWEEN 1 AND 120),
    kind                text        NOT NULL CHECK (kind IN ('incident', 'maintenance')),
    -- Provider id of the event (or a name-based key when the provider has none).
    external_id         text        NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 300),
    name                text        NOT NULL CHECK (char_length(name) <= 255),
    status              text        NOT NULL CHECK (char_length(status) <= 32),
    impact              text        NOT NULL CHECK (char_length(impact) <= 32),
    url                 text        CHECK (url IS NULL OR (url ~ '^https://' AND char_length(url) <= 500)),
    component_names     text[]      NOT NULL DEFAULT '{}',
    first_seen_at       timestamptz NOT NULL,
    last_seen_at        timestamptz NOT NULL,
    closed_at           timestamptz,
    CONSTRAINT external_events_seen_order CHECK (last_seen_at >= first_seen_at AND (closed_at IS NULL OR closed_at >= last_seen_at))
);

CREATE UNIQUE INDEX external_events_open_key ON external_events (source_id, kind, external_id) WHERE closed_at IS NULL;
CREATE INDEX external_events_time_idx ON external_events (first_seen_at DESC, id);
CREATE INDEX external_events_source_idx ON external_events (source_id, first_seen_at DESC);
