-- Announcements: general notices for customers that are neither incidents
-- nor maintenances (new features, policy changes, planned deprecations...).
--
-- Lifecycle: draft → published (published_at may be in the future, which
-- the API presents as "scheduled") → archived. Published announcements can
-- be retracted to draft; archived ones can be restored.

CREATE TABLE announcement_categories (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    description   text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    color         text        NOT NULL DEFAULT 'gray'
                  CHECK (color IN ('gray', 'blue', 'green', 'yellow', 'orange', 'red', 'purple')),
    display_order integer     NOT NULL DEFAULT 0 CHECK (display_order BETWEEN 0 AND 100000),
    is_active     boolean     NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX announcement_categories_name_key ON announcement_categories (lower(name));

CREATE TABLE teams (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX teams_name_key ON teams (lower(name));

CREATE TABLE announcements (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title        text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body         text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000),
    importance   text        NOT NULL DEFAULT 'info' CHECK (importance IN ('info', 'important')),
    is_pinned    boolean     NOT NULL DEFAULT false,
    status       text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    -- RESTRICT: categories and teams in use are deactivated, not deleted.
    category_id  uuid        CONSTRAINT announcements_category_id_fkey REFERENCES announcement_categories (id) ON DELETE RESTRICT,
    team_id      uuid        CONSTRAINT announcements_team_id_fkey REFERENCES teams (id) ON DELETE RESTRICT,
    published_at timestamptz,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT announcements_publication_consistent CHECK ((status = 'draft') = (published_at IS NULL)),
    CONSTRAINT announcements_archive_consistent CHECK ((status = 'archived') = (archived_at IS NOT NULL))
);

CREATE INDEX announcements_public_idx ON announcements (published_at DESC, id) WHERE status = 'published';
CREATE INDEX announcements_category_idx ON announcements (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX announcements_team_idx ON announcements (team_id) WHERE team_id IS NOT NULL;

CREATE TABLE announcement_components (
    announcement_id uuid NOT NULL REFERENCES announcements (id) ON DELETE CASCADE,
    component_id    uuid NOT NULL REFERENCES components (id) ON DELETE RESTRICT,
    PRIMARY KEY (announcement_id, component_id)
);

CREATE INDEX announcement_components_component_idx ON announcement_components (component_id);

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team'));
