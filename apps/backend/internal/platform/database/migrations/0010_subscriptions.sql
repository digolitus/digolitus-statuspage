-- Subscriptions to status updates and the notification outbox.
--
-- Flow: a change (incident, maintenance, announcement) records a
-- notification_event in the same transaction; a worker fans it out into
-- outbox_messages for matching active subscriptions and delivers them with
-- retries. Confirmation and "manage your subscription" e-mails use the same
-- outbox, so nothing is sent from inside an HTTP request.

CREATE TABLE subscriptions (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    channel              text        NOT NULL CHECK (channel IN ('email', 'webhook', 'slack')),
    -- E-mail channel: the recipient. Webhook/Slack: contact for
    -- confirmation and failures (optional when created by an admin).
    email                text        CHECK (email IS NULL OR (char_length(email) BETWEEN 3 AND 254 AND email ~ '^[^@\s]+@[^@\s]+$')),
    target_url           text        CHECK (target_url IS NULL OR (char_length(target_url) <= 500 AND target_url ~ '^https://')),
    -- HMAC-SHA256 key for webhook signatures (X-Digolitus-Signature).
    signing_secret       text,
    status               text        NOT NULL CHECK (status IN ('pending', 'active', 'disabled')),
    all_components       boolean     NOT NULL DEFAULT true,
    created_by           text        NOT NULL CHECK (created_by IN ('public', 'admin')),
    failure_count        integer     NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    disabled_reason      text        CHECK (disabled_reason IS NULL OR char_length(disabled_reason) <= 300),
    last_delivery_at     timestamptz,
    last_error           text        CHECK (last_error IS NULL OR char_length(last_error) <= 300),
    confirmation_sent_at timestamptz,
    confirmed_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subscriptions_channel_fields CHECK (
        (channel = 'email' AND email IS NOT NULL AND target_url IS NULL) OR
        (channel IN ('webhook', 'slack') AND target_url IS NOT NULL)),
    CONSTRAINT subscriptions_webhook_secret CHECK ((channel = 'webhook') = (signing_secret IS NOT NULL))
);

-- One e-mail subscription per address; one subscription per endpoint.
CREATE UNIQUE INDEX subscriptions_email_key ON subscriptions (lower(email)) WHERE channel = 'email';
CREATE UNIQUE INDEX subscriptions_target_key ON subscriptions (channel, target_url) WHERE target_url IS NOT NULL;
CREATE INDEX subscriptions_active_idx ON subscriptions (status) WHERE status = 'active';

CREATE TABLE subscription_components (
    subscription_id uuid NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    component_id    uuid NOT NULL REFERENCES components (id) ON DELETE CASCADE,
    PRIMARY KEY (subscription_id, component_id)
);

CREATE INDEX subscription_components_component_idx ON subscription_components (component_id);

-- A snapshot of what happened, as shown publicly (public component names
-- only), so later edits do not change messages already queued.
CREATE TABLE notification_events (
    id            bigserial PRIMARY KEY,
    kind          text        NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 64),
    resource_type text        NOT NULL,
    resource_id   uuid        NOT NULL,
    payload       jsonb       NOT NULL,
    -- Public components affected; empty = the whole platform.
    component_ids uuid[]      NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL,
    dispatched_at timestamptz
);

CREATE INDEX notification_events_pending_idx ON notification_events (id) WHERE dispatched_at IS NULL;
CREATE INDEX notification_events_feed_idx ON notification_events (created_at DESC);

CREATE TABLE outbox_messages (
    id              bigserial PRIMARY KEY,
    subscription_id uuid        NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    kind            text        NOT NULL CHECK (kind IN ('event', 'confirm', 'manage_link', 'test')),
    event_id        bigint      REFERENCES notification_events (id) ON DELETE CASCADE,
    status          text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts        integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL,
    last_error      text        CHECK (last_error IS NULL OR char_length(last_error) <= 300),
    created_at      timestamptz NOT NULL,
    sent_at         timestamptz,
    CONSTRAINT outbox_event_kind CHECK ((kind = 'event') = (event_id IS NOT NULL)),
    CONSTRAINT outbox_event_once UNIQUE (event_id, subscription_id)
);

CREATE INDEX outbox_messages_due_idx ON outbox_messages (next_attempt_at) WHERE status = 'pending';

-- Server-generated secrets (e.g. the key that signs subscription links).
CREATE TABLE app_secrets (
    name       text PRIMARY KEY,
    value      bytea       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Announcements are notified once, when their publication time arrives
-- (immediate or scheduled). Existing ones are not re-sent.
ALTER TABLE announcements ADD COLUMN notified_at timestamptz;
UPDATE announcements SET notified_at = now() WHERE status <> 'draft';

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription'));
