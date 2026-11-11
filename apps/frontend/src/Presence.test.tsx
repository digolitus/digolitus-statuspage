import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import App from "./App";
import type { PermissionKey, PresenceSnapshot } from "./api/types";
import { publicArea } from "./lib/presence";
import { adminMe, emptySummary, session } from "./test/fixtures";
import { mockApi, page } from "./test/mockApi";

const now = new Date();
const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000).toISOString();

const snapshot: PresenceSnapshot = {
  window_seconds: 120,
  public: {
    total: 44,
    areas: [
      { area: "status", count: 32 },
      { area: "incident", count: 7 },
      { area: "maintenance", count: 0 },
      { area: "announcements", count: 4 },
      { area: "subscriptions", count: 1 },
      { area: "other", count: 0 },
    ],
  },
  admin: {
    total: 2,
    users: [
      { id: adminMe.user.id, name: "Admin", area: "Visão geral", hidden: false, last_seen_at: minutesAgo(0) },
      { id: "u2", name: "Marina Alves", area: "Incidentes › Lentidão no checkout", hidden: true, last_seen_at: minutesAgo(1) },
    ],
  },
  history: [
    { minute: minutesAgo(59), public: 12, admin: 1 },
    { minute: minutesAgo(30), public: 25, admin: 2 },
    { minute: minutesAgo(1), public: 44, admin: 2 },
  ],
  peak_today: { count: 61, at: "2026-11-11T12:14:00Z" },
  generated_at: now.toISOString(),
};

const overview = {
  "GET /api/v1/status": { body: emptySummary },
  "GET /api/v1/admin/components": page([]),
  "GET /api/v1/admin/audit-events": page([]),
};

describe("presence", () => {
  it("maps public paths to areas", () => {
    expect(publicArea("/")).toBe("status");
    expect(publicArea("/incidents/abc")).toBe("incident");
    expect(publicArea("/announcements")).toBe("announcements");
    expect(publicArea("/subscriptions/manage")).toBe("subscriptions");
    expect(publicArea("/nope")).toBe("other");
  });

  it("reports the public tab anonymously", async () => {
    const calls = mockApi({
      "GET /api/v1/status": { body: emptySummary },
      "POST /api/v1/presence": { status: 204 },
    });
    render(<App />);
    await waitFor(() => expect(calls.some((c) => c.path === "/api/v1/presence")).toBe(true));
    const beat = calls.find((c) => c.path === "/api/v1/presence")?.body as { tab_id: string; area: string };
    expect(beat.area).toBe("status");
    expect(beat.tab_id).toMatch(/^[A-Za-z0-9_-]{16,64}$/);
    expect(Object.keys(beat).sort()).toEqual(["area", "tab_id"]);
  });

  it("shows who is here now on the overview and in the menu counter", async () => {
    window.history.pushState(null, "", "/admin");
    const calls = mockApi({
      ...session(),
      ...overview,
      "GET /api/v1/admin/presence": { body: snapshot },
      "POST /api/v1/admin/presence": { status: 204 },
    });
    render(<App />);
    const panel = await screen.findByRole("region", { name: "Acessando agora" });
    expect(within(panel).getByText("44")).toBeInTheDocument();
    expect(within(panel).getByText("Detalhe de incidente")).toBeInTheDocument();
    expect(within(panel).getByText("pico de hoje: 61 às", { exact: false })).toBeInTheDocument();
    expect(within(panel).getByText("você")).toBeInTheDocument();
    expect(within(panel).getByText(/Lentidão no checkout · aba em segundo plano/)).toBeInTheDocument();
    expect(within(panel).getByRole("img", { name: /entre 12 e 44/ })).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "46 pessoas acessando agora" }));
    expect(screen.getByRole("link", { name: "Ver detalhes na Visão geral" })).toBeVisible();

    const beat = calls.find((c) => c.method === "POST" && c.path === "/api/v1/admin/presence")?.body;
    expect(beat).toMatchObject({ area: "Visão geral", hidden: false });
    expect(calls.find((c) => c.path === "/api/v1/admin/presence" && c.method === "GET")?.query.get("timezone")).toBeTruthy();
  });

  it("hides the panel and the counter without presence.read, but still reports the tab", async () => {
    window.history.pushState(null, "", "/admin");
    const operator = { ...adminMe, is_admin: false, permissions: ["incidents.manage"] as PermissionKey[] };
    const calls = mockApi({ ...session(operator), ...overview, "POST /api/v1/admin/presence": { status: 204 } });
    render(<App />);
    await screen.findByRole("heading", { name: "Visão geral" });
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path === "/api/v1/admin/presence")).toBe(true));
    expect(screen.queryByRole("region", { name: "Acessando agora" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /acessando agora/ })).not.toBeInTheDocument();
    expect(calls.some((c) => c.method === "GET" && c.path === "/api/v1/admin/presence")).toBe(false);
  });
});
