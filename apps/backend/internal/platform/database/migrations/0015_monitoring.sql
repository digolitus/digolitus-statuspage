-- Monitoring integrations: external tools (UptimeRobot, Healthchecks.io...)
-- push alerts to a secret webhook URL. Each tool-side monitor may be linked to
-- a component; while it reports "down" the component's effective status is at
-- least the monitor's down_status (see statuspolicy and ADR 0015).

CREATE TABLE monitoring_integrations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    provider         text        NOT NULL CHECK (provider IN ('uptimerobot', 'healthchecks')),
    -- SHA-256 of the webhook token; the token itself is shown once.
    token_hash       bytea       NOT NULL CHECK (octet_length(token_hash) = 32),
    -- Last characters of the token, so operators can recognise it.
    token_hint       text        NOT NULL CHECK (char_length(token_hint) BETWEEN 1 AND 8),
    is_active        boolean     NOT NULL DEFAULT true,
    last_received_at timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT monitoring_integrations_token_hash_key UNIQUE (token_hash)
);

CREATE TABLE monitoring_monitors (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id   uuid        NOT NULL REFERENCES monitoring_integrations (id) ON DELETE CASCADE,
    -- The tool's identifier (UptimeRobot monitorID, Healthchecks check UUID).
    external_id      text        NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 128),
    name             text        NOT NULL DEFAULT '' CHECK (char_length(name) <= 255),
    component_id     uuid        REFERENCES components (id) ON DELETE SET NULL,
    down_status      text        NOT NULL DEFAULT 'major_outage'
                     CHECK (down_status IN ('degraded_performance', 'partial_outage', 'major_outage')),
    -- unknown until the first signal is received.
    state            text        NOT NULL DEFAULT 'unknown' CHECK (state IN ('unknown', 'up', 'down')),
    state_changed_at timestamptz,
    last_signal_at   timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT monitoring_monitors_integration_external_key UNIQUE (integration_id, external_id)
);

CREATE INDEX monitoring_monitors_component_idx ON monitoring_monitors (component_id) WHERE component_id IS NOT NULL;

-- Signals received, newest kept for troubleshooting (pruned per integration).
CREATE TABLE monitoring_events (
    id             bigserial PRIMARY KEY,
    integration_id uuid        NOT NULL REFERENCES monitoring_integrations (id) ON DELETE CASCADE,
    monitor_id     uuid        REFERENCES monitoring_monitors (id) ON DELETE SET NULL,
    received_at    timestamptz NOT NULL,
    -- down/up: a state signal; ignored: understood but not applicable;
    -- invalid: the payload could not be read.
    kind           text        NOT NULL CHECK (kind IN ('down', 'up', 'ignored', 'invalid')),
    monitor_name   text        NOT NULL DEFAULT '' CHECK (char_length(monitor_name) <= 255),
    detail         text        NOT NULL DEFAULT '' CHECK (char_length(detail) <= 500),
    -- Whether the signal changed a linked component's status input.
    applied        boolean     NOT NULL DEFAULT false
);

CREATE INDEX monitoring_events_integration_idx ON monitoring_events (integration_id, received_at DESC, id DESC);

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription', 'announcement_template', 'monitoring_integration', 'department'));

-- Status history: changes caused by a monitor (cause_id = monitor id).
ALTER TABLE component_status_changes DROP CONSTRAINT component_status_changes_cause_type_check;
ALTER TABLE component_status_changes ADD CONSTRAINT component_status_changes_cause_type_check
    CHECK (cause_type IN ('baseline', 'component', 'incident', 'maintenance', 'monitoring'));
