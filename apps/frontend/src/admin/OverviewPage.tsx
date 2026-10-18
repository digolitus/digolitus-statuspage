import { adminApi, publicApi } from "../api/endpoints";
import { Badge, ErrorPanel, Loading } from "../components/ui";
import { formatDateTime, overallLabel, toneForStatus } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { PageHeader } from "./shared";

export function OverviewPage() {
  const { state, reload } = useApi(async (signal) => {
    const [summary, components, audit] = await Promise.all([
      publicApi.status({ signal }),
      adminApi.components.list({ limit: 1 }, { signal }),
      adminApi.audit.list({ limit: 5 }, { signal }),
    ]);
    return { summary, totalComponents: components.pagination.total, audit: audit.data };
  }, []);

  return (
    <div className="admin-content">
      <PageHeader title="Visão geral">
        <Link className="primary-button" to="/admin/incidents/new">
          Abrir incidente
        </Link>
        <Link className="secondary-button" to="/admin/maintenances/new">
          Programar manutenção
        </Link>
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
          <h2>Alterações recentes</h2>
          {state.data.audit.length === 0 ? (
            <p className="muted">Nenhuma alteração registrada.</p>
          ) : (
            <ul className="recent">
              {state.data.audit.map((e) => (
                <li key={e.id}>
                  <time dateTime={e.occurred_at}>{formatDateTime(e.occurred_at)}</time> — {e.resource_type} · {e.action}
                </li>
              ))}
            </ul>
          )}
          <p>
            <Link to="/admin/audit">Ver histórico completo</Link>
          </p>
        </>
      )}
    </div>
  );
}
