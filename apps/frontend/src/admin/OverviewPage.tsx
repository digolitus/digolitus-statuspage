import { adminApi, publicApi } from "../api/endpoints";
import { Badge, ErrorPanel, Loading } from "../components/ui";
import { formatDateTime, overallLabel, toneForStatus } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { PageHeader } from "./shared";

export function OverviewPage() {
  const { can } = useAuth();
  const canAudit = can("audit.read");
  const { state, reload } = useApi(async (signal) => {
    const [summary, components, audit] = await Promise.all([
      publicApi.status({ signal }),
      adminApi.components.list({ limit: 1 }, { signal }),
      // Recent changes only for roles that may read the audit log.
      canAudit ? adminApi.audit.list({ limit: 5 }, { signal }).then((a) => a.data) : Promise.resolve(null),
    ]);
    return { summary, totalComponents: components.pagination.total, audit };
  }, [canAudit]);

  return (
    <div className="admin-content">
      <PageHeader title="Visão geral">
        {can("incidents.manage") && (
          <Link className="primary-button" to="/admin/incidents/new">
            Abrir incidente
          </Link>
        )}
        {can("maintenances.manage") && (
          <Link className="secondary-button" to="/admin/maintenances/new">
            Programar manutenção
          </Link>
        )}
        {can("announcements.write") && (
          <Link className="secondary-button" to="/admin/announcements/new">
            Novo comunicado
          </Link>
        )}
      </PageHeader>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" && (
        <>
          <div className="stats">
            <div className="stat">
              <span>Status geral publicado</span>
              <Badge tone={toneForStatus[state.data.summary.overall.status]}>{overallLabel[state.data.summary.overall.status]}</Badge>
            </div>
            <Link className="stat" to="/admin/incidents">
              <span>Incidentes ativos</span>
              <strong>{state.data.summary.active_incidents.length}</strong>
            </Link>
            <Link className="stat" to="/admin/maintenances">
              <span>Manutenções em andamento / programadas</span>
              <strong>
                {state.data.summary.maintenances.in_progress.length} / {state.data.summary.maintenances.upcoming.length}
              </strong>
            </Link>
            <Link className="stat" to="/admin/components">
              <span>Componentes cadastrados</span>
              <strong>{state.data.totalComponents}</strong>
            </Link>
          </div>
          {state.data.audit && (
            <>
          <h2>Alterações recentes</h2>
          {state.data.audit.length === 0 ? (
            <p className="muted">Nenhuma alteração registrada.</p>
          ) : (
            <ul className="recent">
              {state.data.audit.map((e) => (
                <li key={e.id}>
                  <time dateTime={e.occurred_at}>{formatDateTime(e.occurred_at)}</time> — {e.resource_type} · {e.action}
                  {e.actor && <span className="muted"> · {e.actor.name}</span>}
                </li>
              ))}
            </ul>
          )}
          <p>
            <Link to="/admin/audit">Ver histórico completo</Link>
          </p>
            </>
          )}
        </>
      )}
    </div>
  );
}
