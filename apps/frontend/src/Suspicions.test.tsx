import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import App from "./App";
import type { PublicSuspicion, StatusSummary } from "./api/types";
import { resetFeaturesForTests } from "./lib/features";
import { emptySummary } from "./test/fixtures";
import { mockApi } from "./test/mockApi";

afterEach(() => resetFeaturesForTests());

const summary: StatusSummary = {
  ...emptySummary,
  components: [
    { id: "c1", name: "API", slug: "api", description: "", status: "operational", monitored: false, group_id: "g1", subgroup_id: null, url: null, display_order: 0, updated_at: "2026-03-01T11:00:00Z" },
  ],
  groups: [
    { id: "g1", name: "Pagamentos", description: "", display_order: 0, parent_id: null, status: "operational", component_count: 1, affected_components: 0, components_under_maintenance: 0 },
  ],
};

const open: PublicSuspicion = {
  id: "s1",
  number: 7,
  status: "published",
  component: { id: "c1", name: "API", group: "Pagamentos" },
  opened_at: "2026-03-01T11:00:00Z",
  published_at: "2026-03-01T11:05:00Z",
  last_report_at: "2026-03-01T11:20:00Z",
  report_count: 4,
  recent_count: 3,
  recent: [0, 0, 1, 0, 1, 1],
  summary: { symptom: "slow", frequency: "intermittent", reach: "many_places" },
};

const featureOff = { "GET /api/v1/features": { body: { suspicions: false } } };
const featureOn = { "GET /api/v1/features": { body: { suspicions: true } } };
const providers = { "GET /api/v1/reporter/providers": { body: { email: true, google: false } } };
const signedOut = { "GET /api/v1/reporter/me": { status: 401, body: { error: { code: "unauthenticated", message: "Faça login." } } } };

describe("Reportar suspeita", () => {
  it("hides the link on the status page while the feature is off", async () => {
    mockApi({ "GET /api/v1/status": { body: summary }, ...featureOff });
    render(<App />);
    expect(await screen.findByText("Atualizar agora")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Reportar suspeita" })).not.toBeInTheDocument();
  });

  it("shows the link when the feature is on and lists open suspicions without login", async () => {
    mockApi({ "GET /api/v1/status": { body: summary }, ...featureOn, ...providers, ...signedOut, "GET /api/v1/suspicions": { body: { data: [open] } } });
    render(<App />);
    await userEvent.click(await screen.findByRole("link", { name: "Reportar suspeita" }));
    const card = (await screen.findByRole("heading", { name: /API/ })).closest("li");
    if (!card) throw new Error("card expected");
    expect(within(card).getByText("Lentidão")).toBeInTheDocument();
    expect(within(card).getByText(/4 no total/)).toBeInTheDocument();
  });

  it("asks for login before opening a suspicion", async () => {
    window.history.pushState(null, "", "/suspeitas/nova");
    mockApi({ ...featureOn, ...providers, ...signedOut });
    render(<App />);
    expect(await screen.findByRole("button", { name: "Enviar link de acesso" })).toBeInTheDocument();
    expect(screen.queryByRole("form", { name: "Abrir suspeita" })).not.toBeInTheDocument();
  });

  it("sends the report with the closed choices", async () => {
    window.history.pushState(null, "", "/suspeitas/nova?component=c1");
    const calls = mockApi({
      ...featureOn,
      ...providers,
      "GET /api/v1/reporter/me": { body: { id: "r1", name: "Ana", email: "ana@example.com" } },
      "GET /api/v1/status": { body: summary },
      "GET /api/v1/suspicions": { body: { data: [open] } },
      "POST /api/v1/reporter/suspicions": { status: 201, body: { ...open, id: "s1", number: 7, report_count: 5, messages: [], close_reason: null, close_message: null, incident: null } },
    });
    render(<App />);
    const form = await screen.findByRole("form", { name: "Abrir suspeita" });
    expect(within(form).getByRole("note")).toHaveTextContent("4 pessoas já reportaram");
    await userEvent.click(within(form).getByLabelText("Fora do ar"));
    await userEvent.click(within(form).getByLabelText("Contínuo"));
    await userEvent.click(within(form).getByLabelText("Vários locais"));
    await userEvent.click(within(form).getByLabelText("Agora"));
    await userEvent.type(within(form).getByLabelText(/Descrição/), "Tela de pagamento não carrega");
    await userEvent.click(within(form).getByRole("button", { name: "Enviar suspeita" }));
    expect(await screen.findByText(/Suspeita enviada/)).toBeInTheDocument();
    const post = calls.find((c) => c.method === "POST" && c.path === "/api/v1/reporter/suspicions");
    expect(post?.body).toMatchObject({
      component_id: "c1",
      symptom: "down",
      frequency: "continuous",
      reach: "many_places",
      noticed: "now",
      description: "Tela de pagamento não carrega",
    });
  });
});
