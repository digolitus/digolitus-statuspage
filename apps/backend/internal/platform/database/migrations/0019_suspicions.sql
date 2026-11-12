-- "Reportar suspeita" (ADR 0019): visitors with a light account report a
-- suspected failure of a public component; the owning team triages it.

-- Runtime feature switches managed in the admin area. A missing row means
-- "off". An environment variable may still lock a feature off (see config).
CREATE TABLE feature_flags (
    key        text        PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]{1,39}$'),
    enabled    boolean     NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid        REFERENCES users (id) ON DELETE SET NULL
);

-- Reporters are public accounts, separate from admin users: they can never
-- reach the admin API. Sign-in is passwordless (e-mail link or Google).
CREATE TABLE reporters (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email          text        NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254 AND email ~ '^[^@\s]+@[^@\s]+$'),
    name           text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    google_subject text        CHECK (google_subject IS NULL OR char_length(google_subject) BETWEEN 1 AND 255),
    is_blocked     boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL,
    last_login_at  timestamptz
);

CREATE UNIQUE INDEX reporters_email_key ON reporters (lower(email));
CREATE UNIQUE INDEX reporters_google_key ON reporters (google_subject) WHERE google_subject IS NOT NULL;

-- Single-use sign-in links sent by e-mail; only the SHA-256 of the token is
-- stored. Rows are purged a day after they expire.
CREATE TABLE reporter_login_tokens (
    token_hash bytea       PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    email      text        NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    name       text        NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE INDEX reporter_login_tokens_email_idx ON reporter_login_tokens (lower(email), created_at);

CREATE TABLE reporter_sessions (
    token_hash  bytea       PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    reporter_id uuid        NOT NULL REFERENCES reporters (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL,
    expires_at  timestamptz NOT NULL
);

CREATE INDEX reporter_sessions_reporter_idx ON reporter_sessions (reporter_id);
CREATE INDEX reporter_sessions_expires_idx ON reporter_sessions (expires_at);

-- A suspicion groups every report about one component while it is open
-- (pending or published): at most one open suspicion per component.
CREATE TABLE suspicions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number         bigint      GENERATED ALWAYS AS IDENTITY UNIQUE,
    component_id   uuid        NOT NULL REFERENCES components (id) ON DELETE RESTRICT,
    status         text        NOT NULL CHECK (status IN ('pending', 'published', 'incident', 'closed')),
    opened_at      timestamptz NOT NULL,
    last_report_at timestamptz NOT NULL,
    published_at   timestamptz,
    published_by   uuid        REFERENCES users (id) ON DELETE SET NULL,
    closed_at      timestamptz,
    closed_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    close_reason   text        CHECK (close_reason IN ('no_evidence', 'duplicate', 'out_of_scope')),
    close_message  text        CHECK (close_message IS NULL OR char_length(close_message) <= 2000),
    incident_id    uuid        REFERENCES incidents (id) ON DELETE SET NULL,
    assignee_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    -- Set once reporters were told that the linked incident was resolved.
    resolved_notified_at timestamptz,
    updated_at     timestamptz NOT NULL,
    CHECK ((status = 'closed') = (closed_at IS NOT NULL AND close_reason IS NOT NULL)),
    CHECK (status <> 'published' OR published_at IS NOT NULL)
);

CREATE UNIQUE INDEX suspicions_one_open_per_component ON suspicions (component_id) WHERE status IN ('pending', 'published');
CREATE INDEX suspicions_status_idx ON suspicions (status, last_report_at DESC);
CREATE INDEX suspicions_incident_idx ON suspicions (incident_id) WHERE incident_id IS NOT NULL;

CREATE TABLE suspicion_reports (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    suspicion_id uuid        NOT NULL REFERENCES suspicions (id) ON DELETE CASCADE,
    reporter_id  uuid        NOT NULL REFERENCES reporters (id) ON DELETE CASCADE,
    symptom      text        NOT NULL CHECK (symptom IN ('down', 'slow', 'error', 'partial', 'wrong_data', 'other')),
    frequency    text        NOT NULL CHECK (frequency IN ('continuous', 'intermittent', 'once')),
    reach        text        NOT NULL CHECK (reach IN ('only_me', 'my_team', 'many_places', 'unknown')),
    noticed      text        NOT NULL CHECK (noticed IN ('now', 'last_30m', 'last_2h', 'other')),
    noticed_at   timestamptz NOT NULL,
    location     text        NOT NULL DEFAULT '' CHECK (char_length(location) <= 120),
    error_ref    text        NOT NULL DEFAULT '' CHECK (char_length(error_ref) <= 300),
    description  text        NOT NULL CHECK (char_length(description) BETWEEN 1 AND 1000),
    created_at   timestamptz NOT NULL,
    UNIQUE (suspicion_id, reporter_id)
);

CREATE INDEX suspicion_reports_suspicion_idx ON suspicion_reports (suspicion_id, created_at);
CREATE INDEX suspicion_reports_reporter_idx ON suspicion_reports (reporter_id, created_at);

-- Internal notes (team only) and the conversation with reporters.
CREATE TABLE suspicion_messages (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    suspicion_id   uuid        NOT NULL REFERENCES suspicions (id) ON DELETE CASCADE,
    kind           text        NOT NULL CHECK (kind IN ('internal', 'to_reporter', 'from_reporter')),
    body           text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    author_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    author_name    text        NOT NULL CHECK (char_length(author_name) BETWEEN 1 AND 120),
    reporter_id    uuid        REFERENCES reporters (id) ON DELETE CASCADE,
    created_at     timestamptz NOT NULL,
    CHECK ((kind = 'internal') = (reporter_id IS NULL))
);

CREATE INDEX suspicion_messages_suspicion_idx ON suspicion_messages (suspicion_id, created_at);

-- E-mails to reporters (outbox written in the transaction of the change,
-- delivered by the notification worker).
CREATE TABLE reporter_mails (
    id              bigserial PRIMARY KEY,
    reporter_id     uuid        NOT NULL REFERENCES reporters (id) ON DELETE CASCADE,
    subject         text        NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 200),
    body            text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 6000),
    created_at      timestamptz NOT NULL,
    next_attempt_at timestamptz NOT NULL,
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    sent_at         timestamptz,
    last_error      text        CHECK (last_error IS NULL OR char_length(last_error) <= 300)
);

CREATE INDEX reporter_mails_pending_idx ON reporter_mails (next_attempt_at) WHERE sent_at IS NULL;

-- Triage is part of handling unavailabilities: roles that manage incidents
-- also triage suspicions (team roles only for their teams' components).
UPDATE roles SET permissions = array_append(permissions, 'suspicions.manage')
WHERE 'incidents.manage' = ANY (permissions) AND NOT ('suspicions.manage' = ANY (permissions));

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription', 'announcement_template', 'monitoring_integration',
                             'branding', 'department', 'suspicion', 'reporter', 'feature_flag'));
