import { useId, useState } from "react";
import type { PublicComponent, PublicComponentGroup } from "../api/types";
import { Badge } from "../components/ui";
import { componentStatusLabel, plural, toneForStatus } from "../lib/format";

type Row =
  | { kind: "component"; order: number; name: string; component: PublicComponent }
  | { kind: "group"; order: number; name: string; group: PublicComponentGroup; components: PublicComponent[] };

/**
 * Builds the top-level rows: groups (with their components) and ungrouped
 * components, interleaved by display order and then name.
 */
export function buildRows(components: PublicComponent[], groups: PublicComponentGroup[]): Row[] {
  const byGroup = new Map<string, PublicComponent[]>();
  const rows: Row[] = [];
  for (const c of components) {
    if (c.group_id && groups.some((g) => g.id === c.group_id)) {
      byGroup.set(c.group_id, [...(byGroup.get(c.group_id) ?? []), c]);
    } else {
      rows.push({ kind: "component", order: c.display_order, name: c.name, component: c });
    }
  }
  for (const g of groups) {
    rows.push({ kind: "group", order: g.display_order, name: g.name, group: g, components: byGroup.get(g.id) ?? [] });
  }
  return rows.sort((a, b) => a.order - b.order || a.name.localeCompare(b.name, "pt-BR"));
}

function ComponentRow({ component: c }: { component: PublicComponent }) {
  return (
    <li className="component-row">
      <div>
        <span className="component-name">{c.name}</span>
        {c.description && <span className="component-description">{c.description}</span>}
      </div>
      <Badge tone={toneForStatus[c.status]}>{componentStatusLabel[c.status]}</Badge>
    </li>
  );
}

function groupSummary(g: PublicComponentGroup): string {
  const parts = [plural(g.component_count, "componente", "componentes")];
  if (g.affected_components > 0) parts.push(plural(g.affected_components, "afetado", "afetados"));
  if (g.components_under_maintenance > 0) parts.push(`${g.components_under_maintenance} em manutenção`);
  return parts.join(" · ");
}

/** A group, collapsed by default; its status is always visible. */
function GroupRow({ group, components }: { group: PublicComponentGroup; components: PublicComponent[] }) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  return (
    <li className={`group-row${open ? " group-row-open" : ""}`}>
      <button type="button" className="group-toggle" aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((o) => !o)}>
        <span className="group-chevron" aria-hidden="true">
          ▸
        </span>
        <span className="group-title">
          <span className="component-name">{group.name}</span>
          <span className="component-description">{groupSummary(group)}</span>
        </span>
        <Badge tone={toneForStatus[group.status]}>{componentStatusLabel[group.status]}</Badge>
      </button>
      <div id={panelId} hidden={!open}>
        {group.description && <p className="group-description">{group.description}</p>}
        <ul className="component-rows nested" aria-label={`Componentes de ${group.name}`}>
          {components.map((c) => (
            <ComponentRow key={c.id} component={c} />
          ))}
        </ul>
      </div>
    </li>
  );
}

export function ComponentList({ components, groups }: { components: PublicComponent[]; groups: PublicComponentGroup[] }) {
  const rows = buildRows(components, groups);
  return (
    <ul className="component-rows">
      {rows.map((r) =>
        r.kind === "group" ? (
          <GroupRow key={`g-${r.group.id}`} group={r.group} components={r.components} />
        ) : (
          <ComponentRow key={r.component.id} component={r.component} />
        ),
      )}
    </ul>
  );
}
