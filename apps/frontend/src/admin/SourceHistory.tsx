import { useMemo, useState } from "react";
import { adminApi } from "../api/endpoints";
import type { ExternalSource } from "../api/types";
import { UptimeBar } from "../components/UptimeBar";
import { WithInfo } from "../components/InfoTip";
import { Badge, EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { externalStatusLabel, formatDateTime, formatDuration, formatRelative, toneForStatus } from "../lib/format";
import { useApi } from "../lib/useApi";
import { viewerTimeZone } from "../lib/uptime";

const KIND_LABEL = { incident: "Incidente", maintenance: "Manutenção" } as const;

/**
 * History of a third-party source. What is in progress is only announced at
 * the top; the tables list what the provider already resolved.
 */
export function SourceHistory({ source }: { source: ExternalSource }) {
  const tz = useMemo(() => viewerTimeZone(), []);
  const [eventsOffset, setEventsOffset] = useState(0);
  const [outagesOffset, setOutagesOffset] = useState(0);
  const [onlySelected, setOnlySelected] = useState(true);
  const open = useApi((signal) => adminApi.external.events(source.id, { state: "open", limit: 20 }, { signal }), [source.id]);
  const uptime = useApi((signal) => adminApi.external.uptime(source.id, 90, tz, { signal }), [source.id, tz]);
  const events = useApi(
    (signal) => adminApi.external.events(source.id, { state: "resolved", limit: 10, offset: eventsOffset }, { signal }),
    [source.id, eventsOffset],
  );
  const outages = useApi(
    (signal) => adminApi.external.outages(source.id, { only_selected: onlySelected, limit: 10, offset: outagesOffset }, { signal }),
    [source.id, onlySelected, outagesOffset],
  );

  return (
    <div className="source-history" aria-label={`Histórico de ${source.name}`}>
      {open.state.status === "success" && open.state.data.data.length > 0 && (
        <div className="history-banner" role="status">
          <strong>Em andamento no provedor</strong>
          <ul>
            {open.state.data.data.map((e) => (
              <li key={e.id}>
                {e.url ? (
                  <a href={e.url} target="_blank" rel="noopener noreferrer">
                    {e.name}
                  </a>
                ) : (
                  e.name
                )}{" "}
                · {KIND_LABEL[e.kind]}
                {e.component_names.length > 0 && ` · ${e.component_names.join(", ")}`} · desde {formatDateTime(e.first_seen_at)} (
                {formatRelative(e.first_seen_at)})
              </li>
            ))}
          </ul>
          <span className="small">Entra no histórico abaixo quando o provedor resolver.</span>
        </div>
      )}

      <section>
        <WithInfo block label="Sobre os últimos 90 dias" info="Conforme informado pelo provedor. Períodos sem leitura mantêm o último status conhecido.">
          <h3 className="subheading">Últimos 90 dias dos componentes publicados</h3>
        </WithInfo>
        {uptime.state.status === "loading" && <Loading />}
        {uptime.state.status === "error" && <ErrorPanel error={uptime.state.error} onRetry={uptime.reload} />}
        {uptime.state.status === "success" &&
          (uptime.state.data.components.length === 0 ? (
            <p className="muted">Nenhum componente publicado.</p>
          ) : (
            uptime.state.data.components.map((c) => (
              <div key={c.id}>
                <p className="component-name">{c.name}</p>
                <UptimeBar name={c.name} days={c.days} uptimePercent={c.uptime_percent} size="md" details />
              </div>
            ))
          ))}
      </section>

      <section>
        <WithInfo
          block
          label="Sobre incidentes e manutenções resolvidos"
          info="Início e fim são os momentos em que as leituras do provedor viram o evento aberto e encerrado."
        >
          <h3 className="subheading">Incidentes e manutenções resolvidos</h3>
        </WithInfo>
        {events.state.status === "loading" && <Loading />}
        {events.state.status === "error" && <ErrorPanel error={events.state.error} onRetry={events.reload} />}
        {events.state.status === "success" &&
          (events.state.data.data.length === 0 ? (
            <EmptyState title="Nenhum evento resolvido registrado" />
          ) : (
            <>
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th scope="col">Evento</th>
                      <th scope="col">Tipo</th>
                      <th scope="col">Impacto</th>
                      <th scope="col">Início</th>
                      <th scope="col">Fim</th>
                      <th scope="col">Duração</th>
                      <th scope="col">Componentes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {events.state.data.data.map((e) => (
                      <tr key={e.id}>
                        <td>
                          {e.url ? (
                            <a href={e.url} target="_blank" rel="noopener noreferrer">
                              {e.name}
                            </a>
                          ) : (
                            e.name
                          )}
                        </td>
                        <td>{KIND_LABEL[e.kind]}</td>
                        <td>{e.impact || "—"}</td>
                        <td>{formatDateTime(e.first_seen_at)}</td>
                        <td>{formatDateTime(e.closed_at)}</td>
                        <td>{e.closed_at ? formatDuration(e.first_seen_at, e.closed_at) : "—"}</td>
                        <td>{e.component_names.join(", ") || "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <Pager pagination={events.state.data.pagination} onChange={setEventsOffset} />
            </>
          ))}
      </section>

      <section>
        <div className="section-head">
          <h3 className="subheading">Indisponibilidades resolvidas por componente</h3>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={onlySelected}
              onChange={(e) => {
                setOnlySelected(e.target.checked);
                setOutagesOffset(0);
              }}
            />
            Somente componentes publicados
          </label>
        </div>
        {outages.state.status === "loading" && <Loading />}
        {outages.state.status === "error" && <ErrorPanel error={outages.state.error} onRetry={outages.reload} />}
        {outages.state.status === "success" &&
          (outages.state.data.data.length === 0 ? (
            <EmptyState title="Nenhuma indisponibilidade resolvida registrada" />
          ) : (
            <>
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th scope="col">Componente</th>
                      <th scope="col">Pior status</th>
                      <th scope="col">Início</th>
                      <th scope="col">Fim</th>
                      <th scope="col">Duração</th>
                    </tr>
                  </thead>
                  <tbody>
                    {outages.state.data.data.map((o) => (
                      <tr key={`${o.component_name}-${o.started_at}`}>
                        <td>{o.component_name}</td>
                        <td>
                          <Badge tone={toneForStatus[o.worst_status]}>{externalStatusLabel[o.worst_status]}</Badge>
                        </td>
                        <td>{formatDateTime(o.started_at)}</td>
                        <td>{formatDateTime(o.ended_at)}</td>
                        <td>{formatDuration(o.started_at, o.ended_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <Pager pagination={outages.state.data.pagination} onChange={setOutagesOffset} />
            </>
          ))}
      </section>
    </div>
  );
}
