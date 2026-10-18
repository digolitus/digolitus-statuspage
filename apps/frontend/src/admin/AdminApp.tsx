import { useState } from "react";
import type { PermissionKey } from "../api/types";
import { Link, match, useLocation } from "../lib/router";
import { AccountPage, RolesPage, UsersPage } from "./AccessPages";
import { AnnouncementDetailPage, AnnouncementNewPage, AnnouncementsPage } from "./announcements";
import { AnnouncementSettingsPage } from "./AnnouncementSettingsPage";
import { AnnouncementTemplatesPage } from "./AnnouncementTemplatesPage";
import { AuditPage } from "./AuditPage";
import { AuthGate } from "./AuthGate";
import { useAuth } from "./authContext";
import { ComponentsPage } from "./ComponentsPage";
import { GroupsPage } from "./GroupsPage";
import { IncidentDetailPage, IncidentNewPage, IncidentsPage } from "./incidents";
import { MaintenanceDetailPage, MaintenanceNewPage, MaintenancesPage } from "./maintenances";
import { OverviewPage } from "./OverviewPage";
import { SubscriptionsPage } from "./SubscriptionsPage";
import { TeamsPage } from "./TeamsPage";
import { TemplatesPage } from "./TemplatesPage";
import { ThirdPartyPage } from "./ThirdPartyPage";

type NavItem = { to: string; label: string; need?: PermissionKey[] };

/** Navigation, grouped by purpose; `need` hides entries the user cannot use at all. */
const NAV_GROUPS: { id: string; label: string; items: NavItem[] }[] = [
  {
    id: "operation",
    label: "Operação",
    items: [
      { to: "/admin/incidents", label: "Incidentes" },
      { to: "/admin/maintenances", label: "Manutenções" },
      { to: "/admin/announcements", label: "Comunicados" },
    ],
  },
  {
    id: "catalog",
    label: "Catálogo",
    items: [
      { to: "/admin/components", label: "Componentes" },
      { to: "/admin/component-groups", label: "Grupos de componentes" },
      { to: "/admin/teams", label: "Times responsáveis" },
      { to: "/admin/third-party", label: "Serviços de terceiros" },
    ],
  },
  {
    id: "content",
    label: "Templates e categorias",
    items: [
      { to: "/admin/incident-templates", label: "Templates de incidente" },
      { to: "/admin/maintenance-templates", label: "Templates de manutenção" },
      { to: "/admin/announcement-templates", label: "Templates de comunicado" },
      { to: "/admin/announcement-settings", label: "Categorias de comunicado" },
    ],
  },
  {
    id: "notifications",
    label: "Notificações",
    items: [{ to: "/admin/subscriptions", label: "Inscrições", need: ["subscriptions.manage"] }],
  },
  {
    id: "access",
    label: "Acesso e auditoria",
    items: [
      { to: "/admin/users", label: "Usuários", need: ["users.manage"] },
      { to: "/admin/roles", label: "Papéis e permissões", need: ["roles.manage", "users.manage"] },
      { to: "/admin/audit", label: "Histórico operacional", need: ["audit.read"] },
    ],
  },
];

const NAV_KEY = "dsp.admin.nav.open";

function loadOpen(): string[] {
  try {
    const v = JSON.parse(window.localStorage.getItem(NAV_KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : [];
  } catch {
    return [];
  }
}

/** Collapsible menu groups: closed by default, the current page's group always open. */
function AdminNav({ isActive, can }: { isActive: (to: string) => boolean; can: (p: PermissionKey) => boolean }) {
  const [open, setOpen] = useState<string[]>(loadOpen);
  const toggle = (id: string) =>
    setOpen((cur) => {
      const next = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id];
      try {
        window.localStorage.setItem(NAV_KEY, JSON.stringify(next));
      } catch {
        // Not remembered; fine.
      }
      return next;
    });
  return (
    <nav aria-label="Administração" className="admin-nav">
      <ul>
        <li>
          <Link to="/admin" className="nav-top" aria-current={isActive("/admin") ? "page" : undefined}>
            Visão geral
          </Link>
        </li>
        {NAV_GROUPS.map((g) => {
          const items = g.items.filter((n) => !n.need || n.need.some((p) => can(p)));
          if (!items.length) return null;
          const current = items.some((n) => isActive(n.to));
          const expanded = current || open.includes(g.id);
          return (
            <li key={g.id} className="nav-group">
              <button
                type="button"
                className={current ? "nav-group-toggle nav-group-current" : "nav-group-toggle"}
                aria-expanded={expanded}
                aria-controls={`nav-${g.id}`}
                onClick={() => !current && toggle(g.id)}
                aria-disabled={current || undefined}
                title={current ? "Grupo da página atual" : undefined}
              >
                <span>{g.label}</span>
                <svg className="nav-chevron" width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
                  <path d="M4 2.5L7.5 6 4 9.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </button>
              <ul id={`nav-${g.id}`} hidden={!expanded}>
                {items.map((n) => (
                  <li key={n.to}>
                    <Link to={n.to} aria-current={isActive(n.to) ? "page" : undefined}>
                      {n.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function Forbidden() {
  return (
    <div className="admin-content">
      <h1>Sem permissão</h1>
      <p>Seu papel não dá acesso a esta área. Peça a um administrador, se precisar.</p>
      <Link to="/admin">Voltar à visão geral</Link>
    </div>
  );
}

function Route({ pathname }: { pathname: string }) {
  const { can } = useAuth();
  const allowed = (...perms: PermissionKey[]) => perms.some((p) => can(p));
  let p: Record<string, string> | null;
  if (match("/admin", pathname)) return <OverviewPage />;
  if (match("/admin/account", pathname)) return <AccountPage />;
  if (match("/admin/components", pathname)) return <ComponentsPage />;
  if (match("/admin/component-groups", pathname)) return <GroupsPage />;
  if (match("/admin/third-party", pathname)) return <ThirdPartyPage />;
  if (match("/admin/incidents", pathname)) return <IncidentsPage />;
  if (match("/admin/incidents/new", pathname)) return allowed("incidents.manage") ? <IncidentNewPage /> : <Forbidden />;
  if ((p = match("/admin/incidents/:id", pathname))) return <IncidentDetailPage key={p.id} id={p.id} />;
  if (match("/admin/maintenances", pathname)) return <MaintenancesPage />;
  if (match("/admin/maintenances/new", pathname)) return allowed("maintenances.manage") ? <MaintenanceNewPage /> : <Forbidden />;
  if ((p = match("/admin/maintenances/:id", pathname))) return <MaintenanceDetailPage key={p.id} id={p.id} />;
  if (match("/admin/incident-templates", pathname)) return <TemplatesPage kind="incident" />;
  if (match("/admin/maintenance-templates", pathname)) return <TemplatesPage kind="maintenance" />;
  if (match("/admin/announcement-templates", pathname)) return <AnnouncementTemplatesPage />;
  if (match("/admin/announcements", pathname)) return <AnnouncementsPage />;
  if (match("/admin/announcements/new", pathname)) return allowed("announcements.write") ? <AnnouncementNewPage /> : <Forbidden />;
  if ((p = match("/admin/announcements/:id", pathname))) return <AnnouncementDetailPage key={p.id} id={p.id} />;
  if (match("/admin/announcement-settings", pathname)) return <AnnouncementSettingsPage />;
  if (match("/admin/teams", pathname)) return <TeamsPage />;
  if (match("/admin/subscriptions", pathname)) return allowed("subscriptions.manage") ? <SubscriptionsPage /> : <Forbidden />;
  if (match("/admin/audit", pathname)) return allowed("audit.read") ? <AuditPage /> : <Forbidden />;
  if (match("/admin/users", pathname)) return allowed("users.manage") ? <UsersPage /> : <Forbidden />;
  if (match("/admin/roles", pathname)) return allowed("roles.manage", "users.manage") ? <RolesPage /> : <Forbidden />;
  return (
    <div className="admin-content">
      <h1>Página não encontrada</h1>
      <Link to="/admin">Voltar à visão geral</Link>
    </div>
  );
}

function AdminShell() {
  const { pathname } = useLocation();
  const { me, can, logout } = useAuth();
  // Exact match or a sub-page ("/admin/incidents/123"), never a sibling prefix.
  const active = (to: string) => {
    const p = pathname.replace(/\/$/, "");
    return to === "/admin" ? p === "/admin" : p === to || p.startsWith(to + "/");
  };
  return (
    <div className="admin">
      <a className="skip-link" href="#admin-main">
        Pular para o conteúdo
      </a>
      <div className="admin-shell">
        <aside className="admin-sidebar">
          <Link className="brand" to="/admin">
            <span className="brand-mark" aria-hidden="true">
              D
            </span>
            <span className="brand-name">Administração</span>
          </Link>
          <div className="admin-user" aria-label="Usuário conectado">
            <strong>{me.user.name}</strong>
            <span className="muted small">{me.is_admin ? "Administrador" : me.user.roles.map((r) => r.name).join(", ") || "Somente leitura"}</span>
            <span className="admin-user-actions">
              <Link to="/admin/account">Minha conta</Link>
              <button type="button" className="link-button" onClick={() => void logout()}>
                Sair
              </button>
            </span>
          </div>
          <AdminNav isActive={active} can={can} />
          <p className="admin-sidebar-footer">
            <Link to="/">Ver página pública</Link>
            <a href="/swagger/">Documentação da API</a>
          </p>
        </aside>
        <main id="admin-main" className="admin-main">
          <Route pathname={pathname} />
        </main>
      </div>
    </div>
  );
}

export function AdminApp() {
  return (
    <AuthGate>
      <AdminShell />
    </AuthGate>
  );
}
