import { useId, useMemo, useState } from "react";
import type { PublicComponent, PublicComponentGroup } from "../api/types";
import { InfoTip } from "../components/InfoTip";
import { Badge } from "../components/ui";
import { componentStatusLabel, plural, toneForStatus } from "../lib/format";
import { byDisplayOrder } from "../lib/groups";
import { ComponentUptime, GroupUptime } from "./uptime";

type Item =
  | { kind: "component"; display_order: number; name: string; component: PublicComponent }
  | { kind: "group"; display_order: number; name: string; group: PublicComponentGroup; items: Item[] };

const componentItem = (c: PublicComponent): Item => ({ kind: "component", display_order: c.display_order, name: c.name, component: c });

/**
 * Builds the top-level rows: groups and ungrouped components, interleaved by
 * display order and then name. Inside a group, its direct components and its
 * subgroups (each with its components) are interleaved the same way.
 * Components whose group or subgroup is not published fall back one level.
 */
export function buildRows(components: PublicComponent[], groups: PublicComponentGroup[]): Item[] {
  const known = new Map(groups.map((g) => [g.id, g]));
  const children = new Map<string, Item[]>();
  const add = (id: string, item: Item) => children.set(id, [...(children.get(id) ?? []), item]);
  const rows: Item[] = [];
  for (const c of components) {
    if (c.subgroup_id && known.get(c.subgroup_id)?.parent_id === c.group_id && c.group_id && known.has(c.group_id)) add(c.subgroup_id, componentItem(c));
    else if (c.group_id && known.has(c.group_id)) add(c.group_id, componentItem(c));
    else rows.push(componentItem(c));
  }
  for (const g of groups) {
    if (!g.parent_id) continue;
    if (known.has(g.parent_id)) add(g.parent_id, { kind: "group", display_order: g.display_order, name: g.name, group: g, items: (children.get(g.id) ?? []).sort(byDisplayOrder) });
  }
  for (const g of groups) {
    if (!g.parent_id) rows.push({ kind: "group", display_order: g.display_order, name: g.name, group: g, items: (children.get(g.id) ?? []).sort(byDisplayOrder) });
  }
  return rows.sort(byDisplayOrder);
}

function ComponentRow({ component: c }: { component: PublicComponent }) {
  return (
    <li className="component-row">
      <div>
        <span className="component-name-line">
          <span className="component-name">{c.name}</span>
          {c.description && <InfoTip label={`Sobre ${c.name}`} text={c.description} />}
          {c.monitored && (
            <span className="monitored-chip" title="Status verificado automaticamente por uma ferramenta de monitoramento.">
              <svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true">
                <path d="M1 6h2l1.5-3 3 6L9 6h2" fill="none" stroke="currentColor" strokeWidth="1.5" />
              </svg>
              monitorado
            </span>
          )}
          {c.url && /^https?:\/\//i.test(c.url) && (
          <a
            className="component-link"
            href={c.url}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={`Abrir a página de ${c.name} (nova aba)`}
            title={c.url}
          >
              <span aria-hidden="true">↗</span>
            </a>
          )}
        </span>
      </div>
      <Badge tone={toneForStatus[c.status]}>{componentStatusLabel[c.status]}</Badge>
      <div className="component-uptime">
        <ComponentUptime id={c.id} name={c.name} />
      </div>
    </li>
  );
}

/** Ids of every component inside a group, subgroups included. */
function componentIds(items: Item[]): string[] {
  return items.flatMap((i) => (i.kind === "component" ? [i.component.id] : componentIds(i.items)));
}

function groupSummary(g: PublicComponentGroup): string {
  const parts = [plural(g.component_count, "componente", "componentes")];
  if (g.affected_components > 0) parts.push(plural(g.affected_components, "afetado", "afetados"));
  if (g.components_under_maintenance > 0) parts.push(`${g.components_under_maintenance} em manutenção`);
  return parts.join(" · ");
}

/** A group or subgroup, collapsed by default; its status is always visible. */
function GroupRow({ group, items }: { group: PublicComponentGroup; items: Item[] }) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const sub = group.parent_id !== null;
  const ids = useMemo(() => componentIds(items), [items]);
  return (
    <li className={`group-row${open ? " group-row-open" : ""}${sub ? " subgroup-row" : ""}`}>
      <button type="button" className="group-toggle" aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((o) => !o)}>
        <span className="group-chevron" aria-hidden="true">
          ▸
        </span>
        <span className="group-title">
          <span className="component-name">
            {group.name}
            {sub && <span className="subgroup-label">subgrupo</span>}
          </span>
          <span className="component-description">{groupSummary(group)}</span>
        </span>
        <Badge tone={toneForStatus[group.status]}>{componentStatusLabel[group.status]}</Badge>
      </button>
      <div className="group-uptime">
        <GroupUptime ids={ids} name={group.name} size={sub ? "sm" : "md"} />
      </div>
      <div id={panelId} hidden={!open}>
        {group.description && <p className="group-description">{group.description}</p>}
        <ul className="component-rows nested" aria-label={`Componentes de ${group.name}`}>
          <Rows items={items} />
        </ul>
      </div>
    </li>
  );
}

function Rows({ items }: { items: Item[] }) {
  return items.map((r) =>
    r.kind === "group" ? <GroupRow key={`g-${r.group.id}`} group={r.group} items={r.items} /> : <ComponentRow key={r.component.id} component={r.component} />,
  );
}

export function ComponentList({ components, groups }: { components: PublicComponent[]; groups: PublicComponentGroup[] }) {
  return (
    <ul className="component-rows">
      <Rows items={buildRows(components, groups)} />
    </ul>
  );
}
