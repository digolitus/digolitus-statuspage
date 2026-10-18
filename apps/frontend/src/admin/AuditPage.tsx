import { useState } from "react";
import { adminApi } from "../api/endpoints";
import type { AuditEvent } from "../api/types";
import { EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { formatDateTime } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { PageHeader } from "./shared";

const RESOURCE_LABEL: Record<AuditEvent["resource_type"], string> = {
  component: "Componente",
  component_group: "Grupo de componentes",
  external_source: "Provedor de terceiros",
  incident: "Incidente",
  maintenance: "Manutenção",
  incident_template: "Template de incidente",
  maintenance_template: "Template de manutenção",
  announcement: "Comunicado",
  announcement_category: "Categoria de comunicado",
  team: "Time responsável",
  user: "Usuário",
  role: "Papel",
  subscription: "Inscrição",
  announcement_template: "Template de comunicado",
};

function resourceLink(e: AuditEvent): string | null {
  if (e.resource_type === "incident") return `/admin/incidents/${e.resource_id}`;
  if (e.resource_type === "maintenance") return `/admin/maintenances/${e.resource_id}`;
  return null;
}

export function AuditPage() {
  const [resourceType, setResourceType] = useState("");
  const [offset, setOffset] = useState(0);
  const { state, reload } = useApi(
    (signal) => adminApi.audit.list({ resource_type: resourceType || undefined, limit: 25, offset }, { signal }),
    [resourceType, offset],
  );

  return (
    <div className="admin-content">
      <PageHeader title="Histórico operacional" />
      <p className="muted">
        Cada alteração é registrada na mesma transação da operação, com o usuário que a fez. Registros anteriores à
        autenticação aparecem sem autor.
      </p>
      <label className="inline-filter">
        Tipo de recurso{" "}
        <select
          value={resourceType}
          onChange={(e) => {
            setResourceType(e.target.value);
            setOffset(0);
          }}
        >
          <option value="">Todos</option>
          {Object.entries(RESOURCE_LABEL).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum evento registrado" />
        ) : (
          <>
            <div className="table-wrap">
              <table className="table">
                <caption className="visually-hidden">Eventos de auditoria, do mais recente para o mais antigo</caption>
                <thead>
                  <tr>
                    <th scope="col">Quando</th>
                    <th scope="col">Recurso</th>
                    <th scope="col">Ação</th>
                    <th scope="col">Autor</th>
                    <th scope="col">Origem</th>
                    <th scope="col">Alterações</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.data.map((e) => {
                    const link = resourceLink(e);
                    return (
                      <tr key={e.id}>
                        <td>
                          <time dateTime={e.occurred_at}>{formatDateTime(e.occurred_at)}</time>
                        </td>
                        <td>
                          {RESOURCE_LABEL[e.resource_type]}
                          <br />
                          {link ? <Link to={link}><code>{e.resource_id.slice(0, 8)}</code></Link> : <code>{e.resource_id.slice(0, 8)}</code>}
                        </td>
                        <td>{e.action}</td>
                        <td>{e.actor ? e.actor.name : <span className="muted">—</span>}</td>
                        <td>
                          {e.source}
                          {e.request_id && (
                            <>
                              <br />
                              <small className="muted">req {e.request_id}</small>
                            </>
                          )}
                        </td>
                        <td>
                          <details>
                            <summary>Ver</summary>
                            <pre>{JSON.stringify(e.changes, null, 2)}</pre>
                          </details>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <Pager pagination={state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}
