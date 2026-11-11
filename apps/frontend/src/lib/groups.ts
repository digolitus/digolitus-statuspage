/**
 * Helpers for the two-level component hierarchy: Group -> Subgroup (optional)
 * -> Component. A subgroup is a group with parent_id set.
 */

type Orderable = { id: string; name: string; display_order: number; parent_id: string | null };

export function byDisplayOrder<T extends { name: string; display_order: number }>(a: T, b: T): number {
  return a.display_order - b.display_order || a.name.localeCompare(b.name, "pt-BR");
}

export type GroupNode<G> = { group: G; subgroups: G[] };

/**
 * Top-level groups in display order, each with its subgroups. Subgroups whose
 * group is not in the list are dropped (they cannot be shown in place).
 */
export function groupTree<G extends Orderable>(groups: G[]): GroupNode<G>[] {
  const subs = new Map<string, G[]>();
  for (const g of groups) {
    if (g.parent_id) subs.set(g.parent_id, [...(subs.get(g.parent_id) ?? []), g]);
  }
  return groups
    .filter((g) => !g.parent_id)
    .sort(byDisplayOrder)
    .map((group) => ({ group, subgroups: (subs.get(group.id) ?? []).sort(byDisplayOrder) }));
}

/** "Pagamentos › Cartões", or just the group name. */
export function groupPath<G extends { id: string; name: string }>(groups: G[], groupId: string | null, subgroupId: string | null): string | undefined {
  const name = (id: string | null) => (id ? groups.find((g) => g.id === id)?.name : undefined);
  const group = name(groupId);
  if (!group) return undefined;
  const sub = name(subgroupId);
  return sub ? `${group} › ${sub}` : group;
}
