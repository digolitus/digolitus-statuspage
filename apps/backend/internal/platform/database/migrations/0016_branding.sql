-- Visual identity of the public page and the login screen (ADR 0016).
-- One settings document (validated by the branding module) and the images
-- uploaded for it. Settings absent = defaults; images are served by the API
-- so the page keeps working without third-party hosts.

CREATE TABLE branding_settings (
    -- Single row.
    id         boolean     PRIMARY KEY DEFAULT true CHECK (id),
    settings   jsonb       NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE branding_images (
    kind         text        PRIMARY KEY CHECK (kind IN ('logo', 'favicon')),
    content_type text        NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/webp', 'image/x-icon')),
    data         bytea       NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 524288),
    sha256       text        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    updated_at   timestamptz NOT NULL
);

-- Includes 'monitoring_integration' (migration 0015) so the list is the same
-- whichever order the two features land in.
ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription', 'announcement_template', 'monitoring_integration', 'branding'));
