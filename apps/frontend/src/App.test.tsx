import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import App from "./App";
import type { PermissionKey, StatusSummary } from "./api/types";
import { dayLabel } from "./lib/format";
import { adminMe, emptySummary, incident, maintenance, session, unauthenticated } from "./test/fixtures";
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
    const calls = mockApi({ "GET /api/v1/status": { body: emptySummary }, ...history });
    render(<App />);
    expect(await screen.findByText("Status não informado")).toBeInTheDocument();
    expect(screen.getByText(/sem verificação automática/)).toBeInTheDocument();
    expect(screen.getByText("Nenhum componente publicado")).toBeInTheDocument();
    expect(screen.queryByText(/Todos os sistemas operacionais/)).not.toBeInTheDocument();
    // Nothing happening: no events section, and history is collapsed (not fetched).
    expect(screen.queryByRole("heading", { name: "Acontecendo agora" })).not.toBeInTheDocument();
    expect(calls.some((c) => c.path === "/api/v1/incidents")).toBe(false);
  });

  it("puts components first when nothing is happening, with groups collapsed", async () => {
    const summary: StatusSummary = {
      ...emptySummary,
      overall: { status: "degraded_performance", basis: "declared", affected_components: 1, components_under_maintenance: 0 },
      components: [
        { id: "c1", name: "API de Pagamentos", slug: "api-pag", description: "", status: "degraded_performance", group_id: "g1", url: null, display_order: 0, updated_at: "2026-03-01T11:00:00Z" },
        { id: "c2", name: "Checkout", slug: "checkout", description: "", status: "operational", group_id: "g1", url: null, display_order: 1, updated_at: "2026-03-01T11:00:00Z" },
        { id: "c3", name: "Portal", slug: "portal", description: "", status: "operational", group_id: null, url: null, display_order: 5, updated_at: "2026-03-01T11:00:00Z" },
      ],
      groups: [
        { id: "g1", name: "Pagamentos", description: "Cobrança", display_order: 1, status: "degraded_performance", component_count: 2, affected_components: 1, components_under_maintenance: 0 },
      ],
    };
    mockApi({ "GET /api/v1/status": { body: summary }, ...history });
    render(<App />);
    const toggle = await screen.findByRole("button", { name: /Pagamentos/ });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("2 componentes · 1 afetado")).toBeInTheDocument();
    expect(screen.queryByText("Checkout")).not.toBeVisible();
    expect(screen.getByText("Portal")).toBeVisible();

    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Checkout")).toBeVisible();
    expect(screen.getByText("Cobrança")).toBeVisible();
  });

  it("renders components, active incidents with timeline and maintenances", async () => {
    const summary: StatusSummary = {
      ...emptySummary,
      overall: { status: "partial_outage", basis: "declared", affected_components: 1, components_under_maintenance: 0 },
      components: [{ id: "c1", name: "API", slug: "api", description: "REST", status: "partial_outage", group_id: null, url: "https://docs.example.com/api", display_order: 0, updated_at: "2026-03-01T11:00:00Z" }],
      active_incidents: [incident],
      maintenances: { in_progress: [], upcoming: [maintenance] },
    };
    mockApi({ "GET /api/v1/status": { body: summary }, ...history });
    render(<App />);
    expect(await screen.findByText("1 componente afetado.")).toBeInTheDocument();
    // Active events take the top of the page, above components.
    const now = screen.getByRole("heading", { name: "Acontecendo agora" });
    const components = screen.getByRole("heading", { name: "Componentes" });
    expect(now.compareDocumentPosition(components) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Lentidão na API" })).toBeInTheDocument();
    expect(screen.getByText("Investigando lentidão.")).toBeInTheDocument();
    // Scheduled maintenance is a compact row below components.
    expect(screen.getByRole("link", { name: "Atualização do banco" })).toBeInTheDocument();
    // Planned window shown in the maintenance's own zone.
    expect(screen.getByText(/10\/01\/2030.*02:00/)).toBeInTheDocument();
    const componentsRegion = screen.getByRole("region", { name: "Componentes" });
    expect(within(componentsRegion).getByText("Interrupção parcial")).toBeInTheDocument();
    // Discreet link to the component's own page, opened safely in a new tab.
    const link = within(componentsRegion).getByRole("link", { name: "Abrir a página de API (nova aba)" });
    expect(link).toHaveAttribute("href", "https://docs.example.com/api");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    // The description is behind a discreet "i", not printed in the row.
    expect(within(componentsRegion).queryByText("REST")).not.toBeInTheDocument();
    await userEvent.click(within(componentsRegion).getByRole("button", { name: "Sobre API" }));
    expect(screen.getByRole("tooltip")).toHaveTextContent("REST");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
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

describe("third-party services", () => {
  const source = {
    id: "s1",
    name: "GitHub",
    page_url: "https://www.githubstatus.com",
    status: "partial_outage" as const,
    stale: false,
    last_success_at: "2026-03-01T11:59:00Z",
    components: [
      { id: "e1", name: "API Requests", group_name: "Core", status: "partial_outage" as const },
      { id: "e2", name: "Git Operations", group_name: "Core", status: "operational" as const },
    ],
    events: [{ kind: "incident" as const, name: "API degraded", status: "investigating", impact: "minor", url: "https://stspg.io/a" }],
  };

  it("shows providers collapsed, separate from the platform status", async () => {
    mockApi({ "GET /api/v1/status": { body: { ...emptySummary, third_party: [source] } }, ...history });
    render(<App />);
    const toggle = await screen.findByRole("button", { name: /GitHub/ });
    expect(screen.getByRole("heading", { name: "Serviços de terceiros" })).toBeInTheDocument();
    expect(screen.getByText(/não altera o status da plataforma/)).toBeInTheDocument();
    // Overall stays as declared by the platform itself.
    expect(screen.getByText("Status não informado")).toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("API Requests")).not.toBeVisible();

    await userEvent.click(toggle);
    expect(screen.getByText("API Requests")).toBeVisible();
    const event = screen.getByRole("link", { name: "API degraded" });
    expect(event).toHaveAttribute("href", "https://stspg.io/a");
    expect(event).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByRole("link", { name: /Ver status em www.githubstatus.com/ })).toBeInTheDocument();
  });

  it("flags stale providers instead of showing old statuses", async () => {
    const stale = {
      ...source,
      stale: true,
      status: "unknown" as const,
      events: [],
      components: source.components.map((c) => ({ ...c, status: "unknown" as const })),
    };
    mockApi({ "GET /api/v1/status": { body: { ...emptySummary, third_party: [stale] } }, ...history });
    render(<App />);
    expect(await screen.findByText(/dados desatualizados/)).toBeInTheDocument();
    expect(screen.getAllByText("Desconhecido").length).toBeGreaterThan(0);
  });

  it("lets operators choose which provider components to publish", async () => {
    window.history.pushState(null, "", "/admin/third-party");
    const adminSource = {
      id: "s1", name: "GitHub", provider: "statuspage" as const, base_url: "https://www.githubstatus.com", page_name: "GitHub",
      is_active: true, display_order: 0, indicator: "none" as const, indicator_description: "All Systems Operational",
      last_checked_at: "2026-03-01T12:00:00Z", last_success_at: "2026-03-01T12:00:00Z", last_error: null, stale: false,
      component_count: 2, selected_count: 0, created_at: "2026-03-01T12:00:00Z", updated_at: "2026-03-01T12:00:00Z",
    };
    const comps = [
      { id: "e1", external_id: "c1", name: "API Requests", description: "", group_name: "Core", status: "operational" as const, is_selected: false, is_present: true },
      { id: "e2", external_id: "c2", name: "Pages", description: "", group_name: null, status: "operational" as const, is_selected: false, is_present: true },
    ];
    const calls = mockApi({ ...session(),
      "GET /api/v1/admin/external-sources": { body: { data: [adminSource] } },
      "GET /api/v1/admin/external-sources/s1/components": { body: { data: comps } },
      "PUT /api/v1/admin/external-sources/s1/selection": { body: { data: comps.map((c) => ({ ...c, is_selected: c.id === "e1" })) } },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Escolher componentes" }));
    await user.click(await screen.findByRole("checkbox", { name: /API Requests/ }));
    await user.click(screen.getByRole("button", { name: "Salvar seleção (1)" }));
    expect(await screen.findByText("Seleção salva.")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ component_ids: ["e1"] });
  });
});

describe("admin area", () => {
  it("asks for login when there is no session, then opens the admin area", async () => {
    window.history.pushState(null, "", "/admin/components");
    let signedIn = false;
    const calls = mockApi({
      "GET /api/v1/auth/me": () => (signedIn ? { body: adminMe } : unauthenticated),
      "GET /api/v1/auth/setup": { body: { setup_required: false } },
      "POST /api/v1/auth/login": (c) =>
        (c.body as { password: string }).password === "senha correta e longa"
          ? ((signedIn = true), { body: adminMe })
          : { status: 401, body: { error: { code: "invalid_credentials", message: "E-mail ou senha inválidos." } } },
      "GET /api/v1/admin/components": page([]),
      "GET /api/v1/admin/component-groups": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("E-mail"), "admin@example.com");
    await user.type(screen.getByLabelText("Senha"), "errada");
    await user.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByText("E-mail ou senha inválidos.")).toBeInTheDocument();
    // The password field is cleared after a failure.
    expect(screen.getByLabelText("Senha")).toHaveValue("");
    await user.type(screen.getByLabelText("Senha"), "senha correta e longa");
    await user.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByText("Nenhum componente encontrado")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    // The last attempt is the successful one.
    expect(calls.filter((c) => c.path === "/api/v1/auth/login").pop()?.body).toEqual({ email: "admin@example.com", password: "senha correta e longa" });
  });

  it("creates the first administrator in the initial setup", async () => {
    window.history.pushState(null, "", "/admin/setup");
    const calls = mockApi({
      "GET /api/v1/auth/me": unauthenticated,
      "GET /api/v1/auth/setup": { body: { setup_required: true } },
      "POST /api/v1/auth/setup": { status: 201, body: adminMe },
      "GET /api/v1/status": { body: emptySummary },
      "GET /api/v1/admin/components": page([]),
      "GET /api/v1/admin/audit-events": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    expect(await screen.findByRole("heading", { name: "Configuração inicial" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Token de configuração"), " tok123 ");
    await user.type(screen.getByLabelText("Nome"), "Ana");
    await user.type(screen.getByLabelText("E-mail"), "ana@example.com");
    await user.type(screen.getByLabelText("Senha"), "uma frase longa e segura");
    await user.type(screen.getByLabelText("Confirmar senha"), "outra frase");
    await user.click(screen.getByRole("button", { name: "Criar administrador e entrar" }));
    expect(screen.getByText("As senhas não conferem.")).toBeInTheDocument();
    await user.clear(screen.getByLabelText("Confirmar senha"));
    await user.type(screen.getByLabelText("Confirmar senha"), "uma frase longa e segura");
    await user.click(screen.getByRole("button", { name: "Criar administrador e entrar" }));
    expect(await screen.findByRole("heading", { name: "Visão geral" })).toBeInTheDocument();
    expect(calls.find((c) => c.path === "/api/v1/auth/setup" && c.method === "POST")?.body).toEqual({
      setup_token: "tok123", name: "Ana", email: "ana@example.com", password: "uma frase longa e segura",
    });
  });

  it("requires a personal password after an administrator set it", async () => {
    window.history.pushState(null, "", "/admin");
    let changed = false;
    const temp = { ...adminMe, user: { ...adminMe.user, must_change_password: true } };
    const calls = mockApi({
      "GET /api/v1/auth/me": () => ({ body: changed ? adminMe : temp }),
      "POST /api/v1/auth/password": () => ((changed = true), { status: 204 }),
      "GET /api/v1/status": { body: emptySummary },
      "GET /api/v1/admin/components": page([]),
      "GET /api/v1/admin/audit-events": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    expect(await screen.findByRole("heading", { name: "Defina sua senha" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Senha recebida"), "senha provisoria 1");
    await user.type(screen.getByLabelText("Nova senha"), "minha senha pessoal");
    await user.type(screen.getByLabelText("Confirmar nova senha"), "minha senha pessoal");
    await user.click(screen.getByRole("button", { name: "Salvar e continuar" }));
    expect(await screen.findByRole("heading", { name: "Visão geral" })).toBeInTheDocument();
    expect(calls.find((c) => c.path === "/api/v1/auth/password")?.body).toEqual({ current_password: "senha provisoria 1", new_password: "minha senha pessoal" });
  });

  it("shows only what the user's role allows", async () => {
    window.history.pushState(null, "", "/admin/components");
    const operator = {
      ...adminMe,
      is_admin: false,
      permissions: ["incidents.manage", "components.status"] as PermissionKey[],
      user: { ...adminMe.user, name: "Operadora", roles: [{ id: "r2", name: "Operador de incidentes", is_system: false }] },
    };
    const comp = { id: "c1", name: "API", slug: "api", description: "", status: "operational" as const, is_public: true, is_active: true, display_order: 0, group_id: null, url: null, created_at: "", updated_at: "" };
    const calls = mockApi({
      ...session(operator),
      "GET /api/v1/admin/components": page([comp]),
      "GET /api/v1/admin/component-groups": page([]),
      "PATCH /api/v1/admin/components/c1": { body: { ...comp, status: "partial_outage" } },
    });
    render(<App />);
    const user = userEvent.setup();
    await screen.findByText("api");
    const nav = screen.getByRole("navigation", { name: "Administração" });
    expect(within(nav).queryByRole("link", { name: "Usuários" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Histórico operacional" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Novo componente" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
    expect(screen.getByText(/Somente leitura/)).toBeInTheDocument();
    // Status-only change is allowed.
    await user.click(screen.getByRole("combobox", { name: "Status de API" }));
    await user.click(screen.getByRole("option", { name: "Interrupção parcial" }));
    await user.click(screen.getByRole("button", { name: "Salvar status" }));
    expect(await screen.findByText("Status atualizado.")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "PATCH")?.body).toEqual({ status: "partial_outage" });
  });

  it("returns to the login form when the session ends", async () => {
    window.history.pushState(null, "", "/admin/components");
    let alive = true;
    mockApi({
      "GET /api/v1/auth/me": () => (alive ? { body: adminMe } : unauthenticated),
      "GET /api/v1/auth/setup": { body: { setup_required: false } },
      "GET /api/v1/admin/components": () => (alive ? page([]) : unauthenticated),
      "GET /api/v1/admin/component-groups": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    await screen.findByText("Nenhum componente encontrado");
    alive = false;
    await user.click(screen.getByRole("button", { name: "Desativados" }));
    expect(await screen.findByText(/Sua sessão terminou/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Entrar" })).toBeInTheDocument();
  });

  it("creates an incident from a template after review", async () => {
    window.history.pushState(null, "", "/admin/incidents/new");
    const calls = mockApi({ ...session(),
      "GET /api/v1/admin/incident-templates": page([
        { id: "t1", name: "Interrupção parcial", description: "", title: "Falha em [componente]", message: "Investigando.", impact: "partial_outage", status: "investigating", guidance: "Substitua o componente.", is_active: true, created_at: "", updated_at: "" },
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
    expect(screen.getByRole("combobox", { name: "Impacto" })).toHaveTextContent("Interrupção parcial");
    expect(screen.getByText(/Substitua o componente/)).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Título"));
    await user.type(screen.getByLabelText("Título"), "Falha na API");
    await user.click(await screen.findByRole("checkbox", { name: "API" }));
    await user.click(screen.getByRole("button", { name: "Publicar incidente" }));

    expect(await screen.findByRole("heading", { name: "Lentidão na API" })).toBeInTheDocument();
    const post = calls.find((c) => c.method === "POST");
    expect(post?.body).toEqual({ title: "Falha na API", message: "Investigando.", impact: "partial_outage", status: "investigating", component_ids: ["c1"], template_id: "t1" });
  });

  it("shows field errors returned by the API", async () => {
    window.history.pushState(null, "", "/admin/incidents/new");
    mockApi({ ...session(),
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
    const calls = mockApi({ ...session(),
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

  it("records when the impact really ended when resolving", async () => {
    window.history.pushState(null, "", `/admin/incidents/${incident.id}`);
    const resolved = { ...incident, status: "resolved" as const, resolved_at: "2026-03-02T10:00:00Z" };
    const calls = mockApi({ ...session(),
      [`GET /api/v1/admin/incidents/${incident.id}`]: { body: incident },
      [`POST /api/v1/admin/incidents/${incident.id}/resolve`]: { body: resolved },
      "GET /api/v1/admin/incident-templates": page([]),
    });
    render(<App />);
    const user = userEvent.setup();
    const end = await screen.findByLabelText("Fim do impacto (opcional)");
    fireEvent.change(end, { target: { value: "2026-03-02T10:00" } });
    await user.click(screen.getByRole("button", { name: "Resolver incidente" }));
    await user.click(screen.getByRole("button", { name: "Confirmar resolução" }));
    expect(await screen.findByText(/Incidente resolvido/)).toBeInTheDocument();
    expect(calls.find((c) => c.path.endsWith("/resolve"))?.body).toEqual({
      message: expect.any(String),
      resolved_at: new Date("2026-03-02T10:00").toISOString(),
    });
    // The page shows the real period and its duration.
    expect(screen.getByText("Fim do impacto")).toBeInTheDocument();
    expect(screen.getByText("Duração")).toBeInTheDocument();
  });

  it("groups the admin menu, collapsed except for the current page's group", async () => {
    window.history.pushState(null, "", "/admin/components");
    mockApi({ ...session(), "GET /api/v1/admin/components": page([]), "GET /api/v1/admin/component-groups": page([]) });
    render(<App />);
    const user = userEvent.setup();
    const nav = await screen.findByRole("navigation", { name: "Administração" });
    const catalog = within(nav).getByRole("button", { name: "Catálogo" });
    const operation = within(nav).getByRole("button", { name: "Operação" });
    expect(catalog).toHaveAttribute("aria-expanded", "true");
    expect(within(nav).getByRole("link", { name: "Componentes" })).toHaveAttribute("aria-current", "page");
    expect(operation).toHaveAttribute("aria-expanded", "false");
    expect(within(nav).queryByRole("link", { name: "Incidentes" })).not.toBeInTheDocument();
    await user.click(operation);
    expect(within(nav).getByRole("link", { name: "Incidentes" })).toBeInTheDocument();
  });

  it("schedules a maintenance sending local time plus an explicit timezone", async () => {
    window.history.pushState(null, "", "/admin/maintenances/new");
    const calls = mockApi({ ...session(),
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
    mockApi({ ...session(),
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

describe("impact selector", () => {
  it("lists the four impact levels with icons and supports the keyboard", async () => {
    window.history.pushState(null, "", "/admin/incidents/new");
    const calls = mockApi({ ...session(),
      "GET /api/v1/admin/incident-templates": page([]),
      "GET /api/v1/admin/components": page([]),
      "POST /api/v1/admin/incidents": { status: 201, body: incident },
      [`GET /api/v1/admin/incidents/${incident.id}`]: { body: incident },
    });
    render(<App />);
    const user = userEvent.setup();
    const combo = await screen.findByRole("combobox", { name: "Impacto" });
    await user.click(combo);
    // Only the impact list (the status <select> also has options).
    const options = within(screen.getByRole("listbox")).getAllByRole("option").map((o: HTMLElement) => o.textContent);
    expect(options).toEqual(["Operacional", "Performance degradada", "Interrupção parcial", "Interrupção grave"]);
    await user.click(screen.getByRole("option", { name: "Interrupção grave" }));
    expect(combo).toHaveTextContent("Interrupção grave");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();

    // Keyboard: open with ArrowUp from the current value, confirm with Enter.
    combo.focus();
    await user.keyboard("{ArrowDown}{ArrowUp}{Enter}");
    expect(combo).toHaveTextContent("Interrupção parcial");

    await user.type(screen.getByLabelText("Título"), "Falha");
    await user.type(screen.getByLabelText("Mensagem pública inicial"), "Investigando.");
    await user.click(screen.getByRole("button", { name: "Publicar incidente" }));
    await screen.findByRole("heading", { name: "Lentidão na API" });
    expect((calls.find((c) => c.method === "POST")?.body as Record<string, unknown>).impact).toBe("partial_outage");
  });
});

describe("announcements", () => {
  const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();
  const base = {
    body: "Texto com **destaque**.\n\n- item um\n- item dois",
    importance: "info" as const,
    is_pinned: false,
    category: { id: "cat1", name: "Novidades", color: "blue" as const },
    team: { id: "t1", name: "Plataforma" },
    components: [],
  };
  const items = [
    { ...base, id: "a1", title: "Nova região disponível", published_at: hoursAgo(1), updated_at: hoursAgo(1) },
    { ...base, id: "a2", title: "Mudança no faturamento", importance: "important" as const, category: null, team: null, published_at: hoursAgo(80), updated_at: hoursAgo(80) },
  ];
  const routes = {
    "GET /api/v1/status": { body: emptySummary },
    "GET /api/v1/announcement-categories": { body: { data: [{ id: "cat1", name: "Novidades", description: "", color: "blue" }] } },
    "GET /api/v1/announcements": (call: { query: URLSearchParams }) =>
      call.query.get("pinned") === "true" ? page([]) : page(call.query.get("category_id") ? [items[0]] : items),
  };

  it("signals new announcements in the menu and clears it after the visit", async () => {
    mockApi(routes);
    render(<App />);
    const user = userEvent.setup();
    const nav = screen.getByRole("navigation", { name: "Navegação principal" });
    expect(within(nav).getByRole("link", { name: "Componentes" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Histórico" })).toBeInTheDocument();
    const link = await within(nav).findByRole("link", { name: /Comunicados.*2 novos/ });

    await user.click(link);
    expect(await screen.findByRole("heading", { level: 1, name: "Comunicados" })).toBeInTheDocument();
    // Grouped by day, newest first; formatting rendered safely.
    expect(await screen.findByRole("heading", { name: dayLabel(items[0].published_at) })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Nova região disponível" })).toHaveAttribute("href", "/announcements/a1");
    expect(screen.getAllByText("destaque")[0].tagName).toBe("STRONG");
    expect(screen.getByText("Importante")).toBeInTheDocument();
    expect(screen.getByText("Por Plataforma")).toBeInTheDocument();
    // Visiting the page marks them as seen.
    expect(await within(nav).findByRole("link", { name: "Comunicados" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).queryByText(/novos?\)/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Novidades" }));
    expect(screen.getByRole("button", { name: "Novidades" })).toHaveAttribute("aria-pressed", "true");
    expect(await screen.findByText("Nova região disponível")).toBeInTheDocument();
    expect(screen.queryByText("Mudança no faturamento")).not.toBeInTheDocument();
  });

  it("does not flag announcements already seen", async () => {
    window.localStorage.setItem("digolitus.announcements.lastSeen", items[0].published_at);
    mockApi(routes);
    render(<App />);
    const nav = screen.getByRole("navigation", { name: "Navegação principal" });
    await screen.findByText("Status não informado");
    expect(within(nav).getByRole("link", { name: "Comunicados" })).toBeInTheDocument();
  });

  it("lets operators write, preview and publish an announcement", async () => {
    window.history.pushState(null, "", "/admin/announcements/new");
    const draft = {
      ...base,
      id: "a9",
      title: "Nova API",
      body: "Disponível **hoje**.",
      category: null,
      team: null,
      state: "draft" as const,
      published_at: null,
      archived_at: null,
      created_at: "2026-03-01T12:00:00Z",
      updated_at: "2026-03-01T12:00:00Z",
    };
    const published = { ...draft, state: "published" as const, published_at: "2026-03-01T12:05:00Z" };
    const calls = mockApi({ ...session(),
      "GET /api/v1/admin/announcement-categories": { body: { data: [{ id: "cat1", name: "Novidades", description: "Lançamentos e melhorias.", color: "blue", display_order: 0, is_active: true, announcement_count: 0, created_at: "", updated_at: "" }] } },
      "GET /api/v1/admin/teams": { body: { data: [] } },
      "GET /api/v1/admin/announcements": page([]),
      "GET /api/v1/admin/components": page([]),
      "POST /api/v1/admin/announcements": { status: 201, body: draft },
      "GET /api/v1/admin/announcements/a9": { body: draft },
      "POST /api/v1/admin/announcements/a9/publish": { body: published },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Título"), "Nova API");
    await user.selectOptions(screen.getByLabelText("Categoria"), "cat1");
    expect(screen.getByText("Lançamentos e melhorias.")).toBeInTheDocument();
    // Markdown mode: the toolbar inserts the syntax; the preview renders it.
    await user.click(screen.getByRole("tab", { name: "Markdown" }));
    await user.type(screen.getByRole("textbox", { name: "Texto" }), "Disponível ");
    await user.click(screen.getByRole("button", { name: /Negrito/ }));
    await user.click(screen.getByRole("tab", { name: "Pré-visualizar" }));
    expect(screen.getByText("texto").tagName).toBe("STRONG");
    await user.click(screen.getByRole("button", { name: "Salvar rascunho" }));

    expect(await screen.findByText("Visível apenas na administração.")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({
      title: "Nova API",
      body: "Disponível **texto**",
      importance: "info",
      is_pinned: false,
      category_id: "cat1",
      team_id: null,
      component_ids: [],
    });
    await user.click(screen.getByRole("button", { name: "Publicar agora" }));
    expect(calls.some((c) => c.path.endsWith("/publish"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "Confirmar publicação" }));
    expect(await screen.findByText("Comunicado publicado.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Ver na página pública" })).toHaveAttribute("href", "/announcements/a9");
  });
});

describe("team ownership", () => {
  const squad = { id: "t1", name: "Squad Pagamentos", kind: "Squad" };
  const other = { id: "t2", name: "Squad Busca", kind: "Squad" };
  const team = (t: typeof squad) => ({
    ...t,
    description: "",
    documentation_url: null,
    support_url: null,
    is_active: true,
    announcement_count: 0,
    component_count: 1,
    group_count: 0,
    member_count: 1,
    created_at: "",
    updated_at: "",
  });
  const comp = (id: string, name: string, teamId: string | null, owner: string | null) => ({
    id, name, slug: id, description: "", status: "operational" as const, is_public: true, is_active: true, display_order: 0,
    group_id: null, url: null, team_id: teamId, owner_team_id: owner, created_at: "", updated_at: "",
  });

  it("lets a team-scoped user manage only the components of their teams", async () => {
    window.history.pushState(null, "", "/admin/components");
    const member = {
      ...adminMe,
      is_admin: false,
      permissions: ["components.manage", "components.status"] as PermissionKey[],
      team_permissions: ["components.manage", "components.status"] as PermissionKey[],
      user: { ...adminMe.user, name: "Dev", roles: [{ id: "r3", name: "Operador do time", is_system: false }], teams: [{ id: squad.id, name: squad.name }] },
    };
    mockApi({
      ...session(member),
      "GET /api/v1/admin/components": page([comp("c1", "Checkout", "t1", "t1"), comp("c2", "Busca", null, "t2")]),
      "GET /api/v1/admin/component-groups": page([]),
      "GET /api/v1/admin/teams": { body: { data: [team(squad), team(other)] } },
    });
    render(<App />);
    const user = userEvent.setup();
    expect(await screen.findByText("Time: Squad Busca (grupo)")).toBeInTheDocument();
    expect(screen.getByText(/Seu papel vale só para os componentes dos seus times \(Squad Pagamentos\)/)).toBeInTheDocument();
    const edits = screen.getAllByRole("button", { name: "Editar" });
    expect(edits).toHaveLength(1);
    await user.click(edits[0]);
    const select = screen.getByLabelText("Time responsável");
    const labels = within(select).getAllByRole("option").map((o: HTMLElement) => o.textContent);
    expect(labels).toContain("Squad Pagamentos (Squad)");
    expect(labels).not.toContain("Squad Busca (Squad)");
  });

  it("restricts team-scoped roles to scopable permissions", async () => {
    window.history.pushState(null, "", "/admin/roles");
    const calls = mockApi({
      ...session(),
      "GET /api/v1/admin/roles": { body: { data: [] } },
      "GET /api/v1/admin/permissions": {
        body: {
          data: [
            { key: "incidents.manage", label: "Gerenciar incidentes", description: "Abrir e atualizar.", group: "Operação", sensitive: false, scopable: true },
            { key: "users.manage", label: "Gerenciar usuários", description: "Contas.", group: "Acesso", sensitive: true, scopable: false },
          ],
        },
      },
      "POST /api/v1/admin/roles": { status: 201, body: { id: "r9", name: "Squad", description: "", scope: "team", permissions: ["incidents.manage"], is_system: false, user_count: 0, created_at: "", updated_at: "" } },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Nome do papel"), "Squad");
    await user.click(screen.getByRole("checkbox", { name: /Gerenciar usuários/ }));
    await user.click(screen.getByRole("checkbox", { name: /Gerenciar incidentes/ }));
    await user.click(screen.getByRole("radio", { name: "Componentes dos times do usuário" }));
    expect(screen.getByRole("checkbox", { name: /Gerenciar usuários/ })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: /Gerenciar usuários/ })).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Criar papel" }));
    expect(await screen.findByText("Papel criado.")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ name: "Squad", description: "", scope: "team", permissions: ["incidents.manage"] });
  });
});

describe("subscriptions", () => {
  const options = { email: true, webhook: true, slack: true, sms: false, atom_url: "/api/v1/feed.atom", rss_url: "/api/v1/feed.rss" };

  it("subscribes by e-mail from the header, with double opt-in", async () => {
    const calls = mockApi({
      "GET /api/v1/status": { body: emptySummary },
      ...history,
      "GET /api/v1/subscriptions/options": { body: options },
      "GET /api/v1/components": page([]),
      "POST /api/v1/subscriptions": { status: 202, body: { message: "ok" } },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Inscrever-se" }));
    const dialog = await screen.findByRole("dialog", { name: /Inscrever-se/ });
    expect(within(dialog).queryByRole("tab", { name: "SMS" })).not.toBeInTheDocument();
    await user.type(await within(dialog).findByLabelText("E-mail"), "pessoa@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Inscrever-se" }));
    expect(await within(dialog).findByText("Confira seu e-mail.")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ channel: "email", email: "pessoa@example.com", component_ids: [] });

    await user.click(within(dialog).getByRole("tab", { name: "RSS / Atom" }));
    expect(within(dialog).getByRole("link", { name: "Feed Atom" })).toHaveAttribute("href", "/api/v1/feed.atom");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("confirms a subscription with a click and drops the token from the address bar", async () => {
    window.history.pushState(null, "", "/subscriptions/confirm?token=abc");
    const view = { id: "s1", channel: "email", email: "pessoa@example.com", target_url: null, status: "active", all_components: true, components: [], signing_secret: null, created_at: "" };
    const calls = mockApi({
      "GET /api/v1/components": page([]),
      "POST /api/v1/subscriptions/confirm": { body: view },
      "POST /api/v1/subscriptions/unsubscribe": { status: 204 },
    });
    render(<App />);
    const user = userEvent.setup();
    expect(window.location.search).toBe("");
    await user.click(screen.getByRole("button", { name: "Confirmar inscrição" }));
    expect(await screen.findByText(/Inscrição confirmada/)).toBeInTheDocument();
    expect(calls.find((c) => c.path.endsWith("/confirm"))?.body).toEqual({ token: "abc" });
    await user.click(screen.getByRole("button", { name: "Cancelar inscrição" }));
    expect(await screen.findByRole("heading", { name: "Inscrição cancelada" })).toBeInTheDocument();
    expect(calls.find((c) => c.path.endsWith("/unsubscribe"))?.body).toEqual({ token: "abc" });
  });
});

describe("announcement templates and team tags", () => {
  it("fills the announcement editor from a template", async () => {
    window.history.pushState(null, "", "/admin/announcements/new");
    mockApi({
      ...session(),
      "GET /api/v1/admin/announcement-categories": { body: { data: [{ id: "cat1", name: "Novidades", description: "", color: "blue", display_order: 0, is_active: true, announcement_count: 0, created_at: "", updated_at: "" }] } },
      "GET /api/v1/admin/teams": { body: { data: [] } },
      "GET /api/v1/admin/announcements": page([]),
      "GET /api/v1/admin/components": page([]),
      "GET /api/v1/admin/announcement-templates": page([
        { id: "t1", name: "Nova funcionalidade", description: "", title: "Novo: [recurso]", body: "Já está disponível **[recurso]**.", importance: "important", category_id: "cat1", guidance: "Comece pelo benefício.", is_active: true, created_at: "", updated_at: "" },
      ]),
    });
    render(<App />);
    const user = userEvent.setup();
    await user.selectOptions(await screen.findByLabelText("Usar template de comunicado"), "t1");
    expect(screen.getByLabelText("Título")).toHaveValue("Novo: [recurso]");
    expect(screen.getByLabelText("Categoria")).toHaveValue("cat1");
    expect(screen.getByText(/Comece pelo benefício/)).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Markdown" }));
    expect(screen.getByRole("textbox", { name: "Texto" })).toHaveValue("Já está disponível **[recurso]**.");
  });

  it("adds team tags as chips and sends them", async () => {
    window.history.pushState(null, "", "/admin/teams");
    const created = { id: "t9", name: "Squad X", kind: "", description: "", documentation_url: null, support_url: null, tags: ["pagamentos", "on-call"], is_active: true, announcement_count: 0, component_count: 0, group_count: 0, member_count: 0, created_at: "", updated_at: "" };
    const calls = mockApi({
      ...session(),
      "GET /api/v1/admin/teams": { body: { data: [] } },
      "POST /api/v1/admin/teams": { status: 201, body: created },
    });
    render(<App />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Novo time" }));
    await user.type(screen.getByLabelText("Nome do time"), "Squad X");
    await user.type(screen.getByLabelText("Tags"), "pagamentos{Enter}on-call,PAGAMENTOS{Enter}");
    expect(screen.getByRole("button", { name: "Remover pagamentos" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^Remover / })).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Criar time" }));
    expect(await screen.findByText("Time criado.")).toBeInTheDocument();
    expect((calls.find((c) => c.method === "POST")?.body as { tags: string[] }).tags).toEqual(["pagamentos", "on-call"]);
  });
});
