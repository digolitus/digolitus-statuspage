import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import App from "./App";
import type { StatusChange, StatusSummary, UptimeDay } from "./api/types";
import { emptySummary, session } from "./test/fixtures";
import { mockApi, page } from "./test/mockApi";

const history = {
  "GET /api/v1/incidents": page([]),
  "GET /api/v1/maintenances": page([]),
};

const zero = { major_outage: 0, partial_outage: 0, degraded_performance: 0, maintenance: 0 };
const okDay = (date: string): UptimeDay => ({ date, status: "operational", uptime_percent: 100, durations: zero, incidents: [] });

function bar(id: string, last: UptimeDay, n = 30) {
  const days: UptimeDay[] = [];
  for (let i = 1; i < n; i++) days.push(okDay(`2026-10-${String(i).padStart(2, "0")}`));
  days.push(last);
  return { id, days, uptime_percent: last.uptime_percent ?? 100 };
}

describe("uptime bars", () => {
  const summary: StatusSummary = {
    ...emptySummary,
    overall: { status: "operational", basis: "declared", monitored_components: 0, affected_components: 0, components_under_maintenance: 0 },
    components: [
      { id: "c1", name: "API", slug: "api", description: "", status: "operational", monitored: false, group_id: "g1", subgroup_id: null, url: null, display_order: 0, updated_at: "2026-03-01T11:00:00Z" },
      { id: "c2", name: "Checkout", slug: "checkout", description: "", status: "operational", monitored: false, group_id: "g1", subgroup_id: null, url: null, display_order: 1, updated_at: "2026-03-01T11:00:00Z" },
      { id: "c3", name: "Portal", slug: "portal", description: "", status: "operational", monitored: false, group_id: null, subgroup_id: null, url: null, display_order: 5, updated_at: "2026-03-01T11:00:00Z" },
    ],
    groups: [
      { id: "g1", name: "Pagamentos", description: "", display_order: 1, parent_id: null, status: "operational", component_count: 2, affected_components: 0, components_under_maintenance: 0 },
    ],
  };
  const outage: UptimeDay = {
    date: "2026-10-30",
    status: "major_outage",
    uptime_percent: 95.83,
    durations: { ...zero, major_outage: 3600 },
    incidents: [{ id: "i1", title: "API fora do ar", impact: "major_outage" }],
  };

  it("shows daily bars per component and consolidated per group, with day details and a range picker", async () => {
    const calls = mockApi({
      "GET /api/v1/status": { body: summary },
      ...history,
      "GET /api/v1/status/uptime": (call) => ({
        body: {
          days: Number(call.query.get("days")),
          timezone: call.query.get("timezone"),
          from: "2026-10-01T00:00:00Z",
          to: "2026-10-30T12:00:00Z",
          generated_at: "2026-10-30T12:00:00Z",
          components: [bar("c1", outage), bar("c2", okDay("2026-10-30")), bar("c3", okDay("2026-10-30"))],
        },
      }),
    });
    localStorage.removeItem("dsp.uptime.days");
    render(<App />);
    const user = userEvent.setup();

    // Ungrouped component bar and the group's consolidated bar (worst day, mean uptime).
    expect(await screen.findByRole("group", { name: "Portal: 100,00% de uptime nos últimos 30 dias" })).toBeInTheDocument();
    const groupBar = screen.getByRole("group", { name: "Pagamentos: 97,91% de uptime nos últimos 30 dias" });
    const day = within(groupBar).getByRole("button", { name: /30 de out\. de 2026: Interrupção grave/ });
    await user.click(day);
    expect(day).toHaveAttribute("aria-pressed", "true");
    const detail = screen.getByRole("region", { name: /Pagamentos em 30 de out\. de 2026/ });
    expect(within(detail).getByText("1 h")).toBeInTheDocument();
    expect(within(detail).getByRole("link", { name: "API fora do ar" })).toHaveAttribute("href", "/incidents/i1");

    // Range picker: 90 days by default, remembered after a change.
    const first = calls.find((c) => c.path === "/api/v1/status/uptime");
    expect(first?.query.get("days")).toBe("90");
    expect(first?.query.get("timezone")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "30 dias" }));
    expect(screen.getByRole("button", { name: "30 dias" })).toHaveAttribute("aria-pressed", "true");
    expect(calls.filter((c) => c.path === "/api/v1/status/uptime").at(-1)?.query.get("days")).toBe("30");
    expect(localStorage.getItem("dsp.uptime.days")).toBe("30");
  });

  it("still shows components when the bars cannot be loaded", async () => {
    mockApi({ "GET /api/v1/status": { body: summary }, ...history });
    render(<App />);
    expect(await screen.findByText("Portal")).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Período do histórico" })).not.toBeInTheDocument();
  });
});

describe("admin status history", () => {
  it("lists effective status changes with their origin and filters them", async () => {
    window.history.pushState(null, "", "/admin/status-history");
    const changes: StatusChange[] = [
      {
        id: 2, component_id: "c1", component_name: "API", previous_status: "operational", status: "major_outage",
        cause_type: "incident", cause_id: "i1", cause_name: "API fora do ar", actor: { id: "u1", name: "Ana" }, changed_at: "2026-10-30T10:00:00Z",
      },
      {
        id: 1, component_id: "c1", component_name: "API", previous_status: null, status: "operational",
        cause_type: "baseline", cause_id: null, cause_name: null, actor: null, changed_at: "2026-10-01T00:00:00Z",
      },
    ];
    const calls = mockApi({
      ...session(),
      "GET /api/v1/admin/components": page([{ id: "c1", name: "API" }]),
      "GET /api/v1/admin/status-history": page(changes),
    });
    render(<App />);
    const user = userEvent.setup();
    expect(await screen.findByRole("heading", { name: "Histórico de status" })).toBeInTheDocument();
    const table = await screen.findByRole("table");
    expect(within(table).getByRole("link", { name: "Incidente: API fora do ar" })).toHaveAttribute("href", "/admin/incidents/i1");
    expect(within(table).getByText("Início do registro")).toBeInTheDocument();
    expect(within(table).getByText("Interrupção grave")).toBeInTheDocument();

    await user.selectOptions(screen.getByRole("combobox", { name: /Origem/ }), "incident");
    expect(calls.filter((c) => c.path === "/api/v1/admin/status-history").at(-1)?.query.get("cause_type")).toBe("incident");
  });
});
