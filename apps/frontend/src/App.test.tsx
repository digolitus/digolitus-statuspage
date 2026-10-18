import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import App from "./App";
import type { StatusSummary } from "./api/types";
import { emptySummary, incident, maintenance } from "./test/fixtures";
import { mockApi, page } from "./test/mockApi";

const history = {
  "GET /api/v1/incidents": page([]),
  "GET /api/v1/maintenances": page([]),
};

describe("public status page", () => {
  it("shows a loading state first", () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() => new Promise<Response>(() => undefined));
    render(<App />);
    expect(screen.getByText("Carregando o status da plataforma…")).toBeInTheDocument();
  });

  it("states that nothing is declared when there is no data, without claiming health", async () => {
    mockApi({ "GET /api/v1/status": { body: emptySummary }, ...history });
    render(<App />);
    expect(await screen.findByText("Status não informado")).toBeInTheDocument();
    expect(screen.getByText(/não realiza verificações automáticas/)).toBeInTheDocument();
    expect(screen.getByText("Nenhum incidente ativo")).toBeInTheDocument();
    expect(screen.getByText("Nenhum componente publicado")).toBeInTheDocument();
    expect(screen.queryByText(/Todos os sistemas operacionais/)).not.toBeInTheDocument();
  });

  it("renders components, active incidents with timeline and maintenances", async () => {
    const summary: StatusSummary = {
      ...emptySummary,
      overall: { status: "partial_outage", basis: "declared", affected_components: 1, components_under_maintenance: 0 },
      components: [{ id: "c1", name: "API", slug: "api", description: "REST", status: "partial_outage", updated_at: "2026-03-01T11:00:00Z" }],
      active_incidents: [incident],
      maintenances: { in_progress: [], upcoming: [maintenance] },
    };
    mockApi({ "GET /api/v1/status": { body: summary }, ...history });
    render(<App />);
    expect(await screen.findByText("1 componente afetado.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Lentidão na API" })).toBeInTheDocument();
    expect(screen.getByText("Investigando lentidão.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Atualização do banco" })).toBeInTheDocument();
    // Planned window shown in the maintenance's own zone.
    expect(screen.getByText(/10\/01\/2030.*02:00/)).toBeInTheDocument();
    const components = screen.getByRole("region", { name: "Componentes" });
    expect(within(components).getByText("Indisponibilidade parcial")).toBeInTheDocument();
  });

  it("shows an error with retry when the first load fails", async () => {
    let fail = true;
    mockApi({
      "GET /api/v1/status": () => (fail ? { status: 503, body: { error: { code: "database_unavailable", message: "Banco de dados indisponível." } } } : { body: emptySummary }),
      ...history,
    });
    render(<App />);
    expect(await screen.findByText("Banco de dados indisponível.")).toBeInTheDocument();
    fail = false;
    await userEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(await screen.findByText("Status não informado")).toBeInTheDocument();
  });

  it("keeps the last data and flags it as stale when a refresh fails", async () => {
    let fail = false;
    mockApi({
      "GET /api/v1/status": () => (fail ? { status: 503, body: { error: { code: "x", message: "x" } } } : { body: emptySummary }),
      ...history,
    });
    render(<App />);
    await screen.findByText("Status não informado");
    fail = true;
    await userEvent.click(screen.getByRole("button", { name: "Atualizar agora" }));
    expect(await screen.findByText(/Não foi possível atualizar. Exibindo dados de/)).toBeInTheDocument();
    expect(screen.getByText("Status não informado")).toBeInTheDocument();
  });
});

describe("admin area", () => {
  it("warns that there is no authentication", async () => {
    window.history.pushState(null, "", "/admin/components");
    mockApi({ "GET /api/v1/admin/components": page([]) });
    render(<App />);
    expect(screen.getByText("Área administrativa sem autenticação.")).toBeInTheDocument();
    expect(await screen.findByText("Nenhum componente encontrado")).toBeInTheDocument();
  });

  it("creates an incident from a template after review", async () => {
    window.history.pushState(null, "", "/admin/incidents/new");
    const calls = mockApi({
      "GET /api/v1/admin/incident-templates": page([
        { id: "t1", name: "Indisponibilidade parcial", description: "", title: "Falha em [componente]", message: "Investigando.", impact: "major", status: "investigating", guidance: "Substitua o componente.", is_active: true, created_at: "", updated_at: "" },
      ]),
      "GET /api/v1/admin/components": page([{ id: "c1", name: "API", slug: "api", description: "", status: "operational", is_public: true, is_active: true, display_order: 0, created_at: "", updated_at: "" }]),
      "POST /api/v1/admin/incidents": { status: 201, body: incident },
      "GET /api/v1/admin/incidents/11111111-1111-4111-8111-111111111111": { body: incident },
    });
    render(<App />);
    const user = userEvent.setup();

    // Client-side validation before anything is sent.
    await user.click(await screen.findByRole("button", { name: "Publicar incidente" }));
    expect(screen.getByText("Título é obrigatório.")).toBeInTheDocument();

    await user.selectOptions(await screen.findByLabelText("Usar template"), "t1");
    expect(screen.getByLabelText("Título")).toHaveValue("Falha em [componente]");
    expect(screen.getByLabelText("Impacto")).toHaveValue("major");
    expect(screen.getByText(/Substitua o componente/)).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Título"));
    await user.type(screen.getByLabelText("Título"), "Falha na API");
    await user.click(await screen.findByRole("checkbox", { name: "API" }));
    await user.click(screen.getByRole("button", { name: "Publicar incidente" }));

    expect(await screen.findByRole("heading", { name: "Lentidão na API" })).toBeInTheDocument();
    const post = calls.find((c) => c.method === "POST");
    expect(post?.body).toEqual({ title: "Falha na API", message: "Investigando.", impact: "major", status: "investigating", component_ids: ["c1"], template_id: "t1" });
  });

  it("shows field errors returned by the API", async () => {
    window.history.pushState(null, "", "/admin/incidents/new");
    mockApi({
      "GET /api/v1/admin/incident-templates": page([]),
      "GET /api/v1/admin/components": page([]),
      "POST /api/v1/admin/incidents": {
        status: 422,
        body: { error: { code: "validation_failed", message: "Os dados enviados são inválidos.", details: [{ field: "started_at", message: "O início não pode estar no futuro." }] } },
      },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Título"), "x");
    await user.type(screen.getByLabelText("Mensagem pública inicial"), "y");
    await user.click(screen.getByRole("button", { name: "Publicar incidente" }));
    expect(await screen.findByText("O início não pode estar no futuro.")).toBeInTheDocument();
    expect(screen.getByText("Os dados enviados são inválidos.")).toBeInTheDocument();
  });

  it("resolves an incident only after confirmation", async () => {
    window.history.pushState(null, "", `/admin/incidents/${incident.id}`);
    const resolved = { ...incident, status: "resolved" as const, resolved_at: "2026-03-01T12:00:00Z" };
    const calls = mockApi({
      [`GET /api/v1/admin/incidents/${incident.id}`]: { body: incident },
      [`POST /api/v1/admin/incidents/${incident.id}/resolve`]: { body: resolved },
      "GET /api/v1/admin/incident-templates": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Resolver incidente" }));
    expect(calls.some((c) => c.path.endsWith("/resolve"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "Confirmar resolução" }));
    expect(await screen.findByText(/Incidente resolvido. A linha do tempo é preservada/)).toBeInTheDocument();
    expect(calls.find((c) => c.path.endsWith("/resolve"))?.body).toEqual({ message: expect.stringContaining("resolvido") });
  });

  it("schedules a maintenance sending local time plus an explicit timezone", async () => {
    window.history.pushState(null, "", "/admin/maintenances/new");
    const calls = mockApi({
      "GET /api/v1/admin/maintenance-templates": page([]),
      "GET /api/v1/admin/components": page([]),
      "POST /api/v1/admin/maintenances": { status: 201, body: maintenance },
      [`GET /api/v1/admin/maintenances/${maintenance.id}`]: { body: maintenance },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Título"), "Atualização do banco");
    await user.selectOptions(screen.getByLabelText("Timezone da janela"), "America/Sao_Paulo");
    fireEvent.change(screen.getByLabelText("Início previsto"), { target: { value: "2030-01-10T02:00" } });
    fireEvent.change(screen.getByLabelText("Término previsto"), { target: { value: "2030-01-10T04:00" } });
    await user.click(screen.getByRole("button", { name: "Programar manutenção" }));
    expect(await screen.findByRole("heading", { name: "Atualização do banco" })).toBeInTheDocument();
    const post = calls.find((c) => c.method === "POST")?.body as Record<string, unknown>;
    expect(post.timezone).toBe("America/Sao_Paulo");
    expect(post.scheduled_start).toBe("2030-01-10T02:00");
    expect(post.scheduled_end).toBe("2030-01-10T04:00");
  });

  it("offers only valid maintenance transitions and applies them", async () => {
    window.history.pushState(null, "", `/admin/maintenances/${maintenance.id}`);
    const started = { ...maintenance, status: "in_progress" as const, started_at: "2026-03-01T12:00:00Z" };
    mockApi({
      [`GET /api/v1/admin/maintenances/${maintenance.id}`]: { body: maintenance },
      [`POST /api/v1/admin/maintenances/${maintenance.id}/start`]: { body: started },
      "GET /api/v1/admin/components": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Iniciar manutenção" }));
    expect(screen.queryByRole("button", { name: "Concluir manutenção" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirmar" }));
    expect(await screen.findByRole("button", { name: "Concluir manutenção" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancelar manutenção" })).not.toBeInTheDocument();
  });
});
