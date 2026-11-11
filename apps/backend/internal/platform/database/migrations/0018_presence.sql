-- Who is looking at the status page and the admin area right now (ADR 0018).
--
-- presence_heartbeats holds one row per open browser tab, refreshed about once
-- a minute while the tab is visible. tab_hash is a SHA-256 of a random id the
-- tab generates and keeps in sessionStorage: no IP address, user agent or
-- cookie is stored. Rows are deleted ten minutes after the last heartbeat.
-- UNLOGGED: this is transient data, so it skips the WAL (cheaper writes) and is
-- emptied after a crash, which only resets the live count.
CREATE UNLOGGED TABLE presence_heartbeats (
    tab_hash     bytea       PRIMARY KEY CHECK (octet_length(tab_hash) = 32),
    kind         text        NOT NULL CHECK (kind IN ('public', 'admin')),
    -- Public tabs: a fixed area code. Admin tabs: the screen label shown to
    -- other administrators.
    area         text        NOT NULL CHECK (char_length(area) BETWEEN 1 AND 120),
    user_id      uuid        REFERENCES users (id) ON DELETE CASCADE,
    hidden       boolean     NOT NULL DEFAULT false,
    last_seen_at timestamptz NOT NULL,
    CHECK ((kind = 'admin') = (user_id IS NOT NULL))
);

CREATE INDEX presence_heartbeats_seen_idx ON presence_heartbeats (last_seen_at);

-- Counts per minute for the last-hour chart and the peak of the day. Only
-- aggregates, no identifiers; kept for two days.
CREATE TABLE presence_samples (
    minute       timestamptz PRIMARY KEY,
    public_count integer     NOT NULL CHECK (public_count >= 0),
    admin_count  integer     NOT NULL CHECK (admin_count >= 0)
);
