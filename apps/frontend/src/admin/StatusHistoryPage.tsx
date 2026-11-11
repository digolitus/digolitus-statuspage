import { useState } from "react";
import { adminApi } from "../api/endpoints";
import type { ComponentStatus, StatusChange, StatusChangeCause } from "../api/types";
import { Badge, EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { componentStatusLabel, formatDateTime, toneForStatus } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { PageHeader } from "./shared";

const CAUSE_LABEL: Record<StatusChangeCause, string> = {
  component: "Alteração do componente",
  incident: "Incidente",
  maintenance: "Manutenção",
  baseline: "Início do registro",
};

function StatusBadge({ status }: { status: ComponentStatus | null }) {
  if (status === null) return <Badge tone="neutral">Desativado</Badge>;
  return <Badge tone={toneForStatus[status]}>{componentStatusLabel[status]}</Badge>;
}

function Cause({ e }: { e: StatusChange }) {
  if (e.cause_type === "incident" && e.cause_id) {
    return <Link to={`/admin/incidents/${e.cause_id}`}>Incidente: {e.cause_name ?? e.cause_id.slice(0, 8)}</Link>;
  }
  if (e.cause_type === "maintenance" && e.cause_id) {
    return <Link to={`/admin/maintenances/${e.cause_id}`}>Manutenção: {e.cause_name ?? e.cause_id.slice(0, 8)}</Link>;
  }
  if (e.cause_type === "component") {
    return <>{e.actor ? `Alteração manual por ${e.actor.name}` : "Alteração do componente"}</>;
  }
  return <>{CAUSE_LABEL[e.cause_type]}</>;
}

/** Every change of the components' effective status, newest first (read-only). */
export function StatusHistoryPage() {
  const [componentId, setComponentId] = useState("");
  const [cause, setCause] = useState<StatusChangeCause | "">("");
  const [offset, setOffset] = useState(0);
  const components = useApi((signal) => adminApi.components.list({ limit: 100 }, { signal }), []);
  const { state, reload } = useApi(
    (signal) =>
      adminApi.statusHistory.list({ component_id: componentId || undefined, cause_type: cause || undefined, limit: 25, offset }, { signal }),
    [componentId, cause, offset],
  );

  return (
    <div className="admin-content">
      <PageHeader title="Histórico de status" />
      <p className="muted">
        Cada mudança do status efetivo dos componentes (status declarado combinado com incidentes e manutenções), com a origem.
        Os registros são gravados junto com a alteração e não podem ser editados.
      </p>
      <div className="form-actions">
        <label className="inline-filter">
          Componente{" "}
          <select
            value={componentId}
            onChange={(e) => {
              setComponentId(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">Todos os componentes</option>
            {components.state.status === "success" &&
              components.state.data.data.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
          </select>
        </label>
        <label className="inline-filter">
          Origem{" "}
          <select
            value={cause}
            onChange={(e) => {
              setCause(e.target.value as StatusChangeCause | "");
              setOffset(0);
            }}
          >
            <option value="">Todas</option>
            {Object.entries(CAUSE_LABEL).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhuma mudança de status registrada" />
        ) : (
          <>
            <div className="table-wrap">
              <table className="table">
                <caption className="visually-hidden">Mudanças de status, da mais recente para a mais antiga</caption>
                <thead>
                  <tr>
                    <th scope="col">Quando</th>
                    <th scope="col">Componente</th>
                    <th scope="col">De</th>
                    <th scope="col">Para</th>
                    <th scope="col">Origem</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.data.map((e) => (
                    <tr key={e.id}>
                      <td>
                        <time dateTime={e.changed_at}>{formatDateTime(e.changed_at)}</time>
                      </td>
                      <td>{e.component_name}</td>
                      <td>{e.previous_status === null ? <span className="muted">—</span> : <StatusBadge status={e.previous_status} />}</td>
                      <td>
                        <StatusBadge status={e.status} />
                      </td>
                      <td>
                        <Cause e={e} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pager pagination={state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}
