-- Third-party status sources (e.g. GitHub, Veracode) read from their public
-- status page APIs. Their data is a cached snapshot reported by the provider:
-- it is shown separately and never changes the platform's declared status.

CREATE TABLE external_sources (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    provider              text        NOT NULL CHECK (provider IN ('statuspage')),
    base_url              text        NOT NULL CHECK (base_url ~ '^https://[a-z0-9.-]+(:[0-9]+)?$'),
    page_name             text        NOT NULL DEFAULT '' CHECK (char_length(page_name) <= 255),
    is_active             boolean     NOT NULL DEFAULT true,
    display_order         integer     NOT NULL DEFAULT 0 CHECK (display_order BETWEEN 0 AND 100000),
    indicator             text        CHECK (indicator IS NULL OR indicator IN ('none', 'minor', 'major', 'critical', 'maintenance')),
    indicator_description text        NOT NULL DEFAULT '' CHECK (char_length(indicator_description) <= 255),
    -- Unresolved incidents and in-progress maintenance reported by the provider.
    events                jsonb       NOT NULL DEFAULT '[]'::jsonb,
    last_checked_at       timestamptz,
    last_success_at       timestamptz,
    last_error            text        CHECK (last_error IS NULL OR char_length(last_error) <= 500),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT external_sources_base_url_key UNIQUE (base_url)
);

CREATE INDEX external_sources_due_idx ON external_sources (last_checked_at NULLS FIRST) WHERE is_active;

CREATE TABLE external_components (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id           uuid        NOT NULL REFERENCES external_sources (id) ON DELETE CASCADE,
    external_id         text        NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 64),
    name                text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    description         text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    group_name          text        CHECK (group_name IS NULL OR char_length(group_name) <= 255),
    status              text        NOT NULL
                        CHECK (status IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage', 'maintenance', 'unknown')),
    position            integer     NOT NULL DEFAULT 0,
    -- Chosen by an operator to appear on the public page.
    is_selected         boolean     NOT NULL DEFAULT false,
    -- False when the provider no longer lists the component.
    is_present          boolean     NOT NULL DEFAULT true,
    provider_updated_at timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT external_components_source_external_key UNIQUE (source_id, external_id)
);

CREATE INDEX external_components_selected_idx ON external_components (source_id) WHERE is_selected AND is_present;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template', 'maintenance_template', 'external_source'));
