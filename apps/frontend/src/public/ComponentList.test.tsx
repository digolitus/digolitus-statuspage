import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { PublicComponent, PublicComponentGroup } from "../api/types";
import { buildRows, ComponentList } from "./ComponentList";

const comp = (id: string, order: number, group: string | null, sub: string | null = null): PublicComponent => ({
  id, name: id, slug: id, description: "", status: "operational", monitored: false, group_id: group, subgroup_id: sub, url: null, display_order: order, updated_at: "",
});
const grp = (id: string, order: number, parent: string | null = null, status: PublicComponentGroup["status"] = "operational"): PublicComponentGroup => ({
  id, name: id, description: "", display_order: order, parent_id: parent, status, component_count: 1, affected_components: 0, components_under_maintenance: 0,
});

describe("buildRows", () => {
  it("nests subgroups inside their group, interleaved with direct components", () => {
    const rows = buildRows(
      [comp("Checkout", 1, "Pagamentos"), comp("API Cartões", 0, "Pagamentos", "Cartões"), comp("Site", 5, null)],
      [grp("Pagamentos", 1), grp("Cartões", 2, "Pagamentos")],
    );
    expect(rows.map((r) => r.name)).toEqual(["Pagamentos", "Site"]);
    const pay = rows[0];
    if (pay.kind !== "group") throw new Error("group expected");
    expect(pay.items.map((i) => i.name)).toEqual(["Checkout", "Cartões"]);
    const cards = pay.items[1];
    expect(cards.kind === "group" && cards.items.map((i) => i.name)).toEqual(["API Cartões"]);
  });

  it("falls back to the group when the subgroup is not published", () => {
    const rows = buildRows([comp("API", 0, "Pagamentos", "Oculto")], [grp("Pagamentos", 0)]);
    expect(rows[0].kind === "group" && rows[0].items.map((i) => i.name)).toEqual(["API"]);
  });
});

describe("ComponentList", () => {
  it("shows a collapsible subgroup with its own status", async () => {
    render(
      <ComponentList
        components={[comp("API Cartões", 0, "Pagamentos", "Cartões")]}
        groups={[grp("Pagamentos", 0, null, "degraded_performance"), grp("Cartões", 0, "Pagamentos", "degraded_performance")]}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /^Pagamentos/ }));
    const sub = screen.getByRole("button", { name: /Cartões\s*subgrupo/ });
    expect(sub).toHaveAttribute("aria-expanded", "false");
    await user.click(sub);
    expect(within(screen.getByRole("list", { name: "Componentes de Cartões" })).getByText("API Cartões")).toBeVisible();
  });
});
