-- Component groups: an optional, single-level grouping used to organise the
-- public page (e.g. "Pagamentos" containing several components). A group has
-- no status of its own; it is derived from its components.

CREATE TABLE component_groups (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description   text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    display_order integer     NOT NULL DEFAULT 0 CHECK (display_order BETWEEN 0 AND 100000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX component_groups_name_key ON component_groups (lower(name));
CREATE INDEX component_groups_listing_idx ON component_groups (display_order, name, id);

-- RESTRICT: a group can only be deleted once no component references it.
ALTER TABLE components
    ADD COLUMN group_id uuid CONSTRAINT components_group_id_fkey REFERENCES component_groups (id) ON DELETE RESTRICT;

CREATE INDEX components_group_idx ON components (group_id) WHERE group_id IS NOT NULL;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template', 'maintenance_template'));
