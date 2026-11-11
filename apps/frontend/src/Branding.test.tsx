import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import App from "./App";
import type { AdminBranding, PublicBranding } from "./api/types";
import { DEFAULT_BRANDING, contrast, contrastProblems } from "./lib/branding";
import { emptySummary, session, unauthenticated } from "./test/fixtures";
import { mockApi } from "./test/mockApi";

const acme: PublicBranding = {
  ...DEFAULT_BRANDING,
  title: "Acme Status",
  subtitle: "Serviços da Acme",
  description: "Status da Acme",
  login_title: "Entrar no painel Acme",
  login_message: "Acesso restrito à equipe.",
  footer_text: "Equipe de operações Acme.",
  footer_link: { label: "Suporte", url: "https://acme.example.com/suporte" },
  colors: { primary: "#1f5fa8", header_background: "#0f2a47", header_text: "#ffffff", page_background: "#f4f7fb" },
  logo_url: "/api/v1/branding/images/logo?v=0123456789abcdef",
  show_title_with_logo: false,
  updated_at: "2026-11-11T10:00:00Z",
};

describe("visual identity on the public page", () => {
  it("shows the configured name, logo, footer and colors", async () => {
    mockApi({ "GET /api/v1/status": { body: emptySummary }, "GET /api/v1/branding": { body: acme } });
    render(<App />);
    const logo = await screen.findByRole("img", { name: "Acme Status" });
    expect(logo).toHaveAttribute("src", acme.logo_url);
    expect(screen.getByRole("link", { name: "Acme Status — início" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Suporte" })).toHaveAttribute("href", "https://acme.example.com/suporte");
    expect(screen.getByText("Equipe de operações Acme.", { exact: false })).toBeInTheDocument();
    expect(document.title).toBe("Acme Status");
    expect(document.querySelector('meta[name="description"]')).toHaveAttribute("content", "Status da Acme");
    // No favicon configured: the title's initial on the primary color.
    expect(document.querySelector('link[rel="icon"]')?.getAttribute("href")).toMatch(/^data:image\/svg\+xml,/);
    const app = document.querySelector<HTMLElement>(".app.branded");
    expect(app?.style.getPropertyValue("--brand")).toBe("#1f5fa8");
    expect(app?.style.getPropertyValue("--header-bg")).toBe("#0f2a47");
  });

  it("keeps the default look when the branding cannot be loaded", async () => {
    mockApi({ "GET /api/v1/status": { body: emptySummary }, "GET /api/v1/branding": { status: 500, body: { error: { code: "internal_error", message: "x" } } } });
    render(<App />);
    await screen.findByText("Status não informado");
    expect(screen.getByRole("link", { name: "Digolitus Status Page — início" })).toBeInTheDocument();
    expect(screen.getByText("Status informado pela equipe de operações.")).toBeInTheDocument();
  });

  it("uses the login title and message on the sign-in screen", async () => {
    window.history.pushState(null, "", "/admin");
    mockApi({
      "GET /api/v1/auth/me": unauthenticated,
      "GET /api/v1/auth/setup": { body: { setup_required: false } },
      "GET /api/v1/branding": { body: { ...acme, logo_url: null } },
    });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Entrar no painel Acme" })).toBeInTheDocument();
    expect(screen.getByText("Acesso restrito à equipe.")).toBeInTheDocument();
    expect(screen.getByText("Acme Status")).toBeInTheDocument();
  });
});

describe("contrast rules", () => {
  it("matches the API checks", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 1);
    expect(contrastProblems(DEFAULT_BRANDING.colors)).toEqual({});
    expect(Object.keys(contrastProblems({ primary: "#9fd3b8", header_background: "#ffffff", header_text: "#dddddd", page_background: "#202020" })).sort()).toEqual([
      "colors.header_text",
      "colors.page_background",
      "colors.primary",
    ]);
  });
});

describe("visual identity admin page", () => {
  const admin: AdminBranding = {
    ...DEFAULT_BRANDING,
    footer_link_label: "",
    footer_link_url: "",
    logo: { source: "none", url: null },
    favicon: { source: "none", url: null },
    logo_upload: null,
    favicon_upload: null,
    allowed_image_hosts: ["cdn.acme.example.com"],
    defaults: {
      ...DEFAULT_BRANDING,
      footer_link_label: "",
      footer_link_url: "",
      logo: { source: "none", url: null },
      favicon: { source: "none", url: null },
    },
    updated_at: null,
  };
  // AdminBranding has no public-only fields.
  delete (admin as Partial<PublicBranding>).footer_link;
  delete (admin as Partial<PublicBranding>).logo_url;
  delete (admin as Partial<PublicBranding>).favicon_url;

  it("previews changes, blocks low contrast and saves the whole settings", async () => {
    window.history.pushState(null, "", "/admin/branding");
    const calls = mockApi({
      ...session(),
      "GET /api/v1/admin/branding": { body: admin },
      "PUT /api/v1/admin/branding": (c) => ({ body: { ...admin, ...(c.body as object), updated_at: "2026-11-11T12:00:00Z" } }),
    });
    const user = userEvent.setup();
    render(<App />);
    const title = await screen.findByLabelText("Título da aplicação");
    await user.clear(title);
    await user.type(title, "Acme Status");
    const preview = screen.getByRole("complementary", { name: "Prévia" });
    expect(within(preview).getAllByText("Acme Status").length).toBeGreaterThan(0);

    const primary = screen.getByLabelText("Cor principal");
    fireEvent.change(primary, { target: { value: "#9fd3b8" } });
    expect(await screen.findByText(/^Contraste de .* texto branco dos botões/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Salvar e publicar" })).toBeDisabled();

    fireEvent.change(primary, { target: { value: "#1f5fa8" } });
    await user.click(within(screen.getByRole("group", { name: "Logotipo" })).getByRole("button", { name: "Usar URL" }));
    await user.type(screen.getByLabelText("URL pública do logotipo"), "https://cdn.acme.example.com/logo.png");
    await user.click(screen.getByRole("button", { name: "Salvar e publicar" }));
    expect(await screen.findByText("Identidade visual salva e publicada.")).toBeInTheDocument();

    const put = calls.find((c) => c.method === "PUT" && c.path === "/api/v1/admin/branding");
    expect(put?.body).toMatchObject({
      title: "Acme Status",
      colors: { primary: "#1f5fa8" },
      logo: { source: "url", url: "https://cdn.acme.example.com/logo.png" },
      favicon: { source: "none", url: null },
    });
    expect(Object.keys(put?.body as object)).not.toContain("allowed_image_hosts");
  });
});
