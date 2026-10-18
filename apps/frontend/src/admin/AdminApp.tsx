import type { PermissionKey } from "../api/types";
import { Link, match, useLocation } from "../lib/router";
import { AccountPage, RolesPage, UsersPage } from "./AccessPages";
import { AnnouncementDetailPage, AnnouncementNewPage, AnnouncementsPage } from "./announcements";
import { AnnouncementSettingsPage } from "./AnnouncementSettingsPage";
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

/** Navigation entries; `need` hides entries the user cannot use at all. */
const NAV: { to: string; label: string; need?: PermissionKey[] }[] = [
  { to: "/admin", label: "Visão geral" },
  { to: "/admin/components", label: "Componentes" },
  { to: "/admin/component-groups", label: "Grupos de componentes" },
  { to: "/admin/third-party", label: "Serviços de terceiros" },
  { to: "/admin/incidents", label: "Incidentes" },
  { to: "/admin/incident-templates", label: "Templates de incidente" },
  { to: "/admin/maintenances", label: "Manutenções" },
  { to: "/admin/maintenance-templates", label: "Templates de manutenção" },
  { to: "/admin/announcements", label: "Comunicados" },
  { to: "/admin/announcement-settings", label: "Categorias de comunicado" },
  { to: "/admin/teams", label: "Times responsáveis" },
  { to: "/admin/subscriptions", label: "Inscrições", need: ["subscriptions.manage"] },
  { to: "/admin/audit", label: "Histórico operacional", need: ["audit.read"] },
  { to: "/admin/users", label: "Usuários", need: ["users.manage"] },
  { to: "/admin/roles", label: "Papéis e permissões", need: ["roles.manage", "users.manage"] },
];

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
  const active = (to: string) => (to === "/admin" ? pathname.replace(/\/$/, "") === "/admin" : pathname.startsWith(to));
  const nav = NAV.filter((n) => !n.need || n.need.some((p) => can(p)));
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
          <nav aria-label="Administração">
            <ul>
              {nav.map((n) => (
                <li key={n.to}>
                  <Link to={n.to} aria-current={active(n.to) ? "page" : undefined}>
                    {n.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
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
