-- Optional subgroups: Group -> Subgroup -> Component (ADR 0013).
-- A subgroup is a component_groups row with parent_id set. Only two levels
-- exist: the service keeps parent_id pointing at a top-level group and lets
-- a group change level only while it has no components or subgroups.

ALTER TABLE component_groups
    ADD COLUMN parent_id uuid CONSTRAINT component_groups_parent_id_fkey REFERENCES component_groups (id) ON DELETE RESTRICT,
    ADD CONSTRAINT component_groups_parent_not_self CHECK (parent_id IS NULL OR parent_id <> id),
    -- Target of the composite key below.
    ADD CONSTRAINT component_groups_id_parent_key UNIQUE (id, parent_id);

CREATE INDEX component_groups_parent_idx ON component_groups (parent_id) WHERE parent_id IS NOT NULL;

-- Names are unique among top-level groups and among the subgroups of one
-- group (case-insensitive), so "API" can exist under several groups.
DROP INDEX component_groups_name_key;
CREATE UNIQUE INDEX component_groups_name_key ON component_groups (parent_id, lower(name)) NULLS NOT DISTINCT;

-- components.group_id stays the top-level group; subgroup_id is optional and
-- must be a subgroup of that group. The composite key guarantees it, and
-- ON UPDATE CASCADE moves the components when a subgroup changes group.
ALTER TABLE components
    ADD COLUMN subgroup_id uuid,
    ADD CONSTRAINT components_subgroup_requires_group CHECK (subgroup_id IS NULL OR group_id IS NOT NULL),
    ADD CONSTRAINT components_subgroup_id_fkey FOREIGN KEY (subgroup_id, group_id)
        REFERENCES component_groups (id, parent_id) ON UPDATE CASCADE ON DELETE RESTRICT;

CREATE INDEX components_subgroup_idx ON components (subgroup_id) WHERE subgroup_id IS NOT NULL;
