import { Link, match, useLocation } from "../lib/router";
import { AuditPage } from "./AuditPage";
import { ComponentsPage } from "./ComponentsPage";
import { GroupsPage } from "./GroupsPage";
import { IncidentDetailPage, IncidentNewPage, IncidentsPage } from "./incidents";
import { MaintenanceDetailPage, MaintenanceNewPage, MaintenancesPage } from "./maintenances";
import { OverviewPage } from "./OverviewPage";
import { TemplatesPage } from "./TemplatesPage";
import { ThirdPartyPage } from "./ThirdPartyPage";

const NAV = [
  { to: "/admin", label: "Visão geral" },
  { to: "/admin/components", label: "Componentes" },
  { to: "/admin/component-groups", label: "Grupos de componentes" },
  { to: "/admin/third-party", label: "Serviços de terceiros" },
  { to: "/admin/incidents", label: "Incidentes" },
  { to: "/admin/incident-templates", label: "Templates de incidente" },
  { to: "/admin/maintenances", label: "Manutenções" },
  { to: "/admin/maintenance-templates", label: "Templates de manutenção" },
  { to: "/admin/audit", label: "Histórico operacional" },
];

function route(pathname: string) {
  let p: Record<string, string> | null;
  if (match("/admin", pathname)) return <OverviewPage />;
  if (match("/admin/components", pathname)) return <ComponentsPage />;
  if (match("/admin/component-groups", pathname)) return <GroupsPage />;
  if (match("/admin/third-party", pathname)) return <ThirdPartyPage />;
  if (match("/admin/incidents", pathname)) return <IncidentsPage />;
  if (match("/admin/incidents/new", pathname)) return <IncidentNewPage />;
  if ((p = match("/admin/incidents/:id", pathname))) return <IncidentDetailPage key={p.id} id={p.id} />;
  if (match("/admin/maintenances", pathname)) return <MaintenancesPage />;
  if (match("/admin/maintenances/new", pathname)) return <MaintenanceNewPage />;
  if ((p = match("/admin/maintenances/:id", pathname))) return <MaintenanceDetailPage key={p.id} id={p.id} />;
  if (match("/admin/incident-templates", pathname)) return <TemplatesPage kind="incident" />;
  if (match("/admin/maintenance-templates", pathname)) return <TemplatesPage kind="maintenance" />;
  if (match("/admin/audit", pathname)) return <AuditPage />;
  return (
    <div className="admin-content">
      <h1>Página não encontrada</h1>
      <Link to="/admin">Voltar à visão geral</Link>
    </div>
  );
}

export function AdminApp() {
  const { pathname } = useLocation();
  const active = (to: string) => (to === "/admin" ? pathname.replace(/\/$/, "") === "/admin" : pathname.startsWith(to));
  return (
    <div className="admin">
      <a className="skip-link" href="#admin-main">
        Pular para o conteúdo
      </a>
      <div className="admin-warning" role="note">
        <strong>Área administrativa sem autenticação.</strong> Qualquer pessoa com acesso a esta rede pode alterar os
        dados. Use apenas em ambiente local ou rede confiável.
      </div>
      <div className="admin-shell">
        <aside className="admin-sidebar">
          <Link className="brand" to="/admin">
            <span className="brand-mark" aria-hidden="true">
              D
            </span>
            <span className="brand-name">Administração</span>
          </Link>
          <nav aria-label="Administração">
            <ul>
              {NAV.map((n) => (
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
          {route(pathname)}
        </main>
      </div>
    </div>
  );
}
