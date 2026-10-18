import { useState } from "react";
import { publicApi } from "../api/endpoints";
import type { StatusSummary } from "../api/types";
import { Badge, EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { componentStatusLabel, formatDateTime, overallLabel, plural, toneForStatus } from "../lib/format";
import { useApi } from "../lib/useApi";
import { usePolling } from "../lib/usePolling";
import { IncidentCard, MaintenanceCard } from "./views";

/** Public refresh policy: every 60 s, backing off up to 5 min on failures. */
export const POLL_INTERVAL_MS = 60_000;
export const POLL_MAX_BACKOFF_MS = 300_000;

function overallDescription(s: StatusSummary): string {
  const o = s.overall;
  switch (o.status) {
    case "unknown":
      return "Nenhum componente foi publicado e não há incidentes ativos.";
    case "operational":
      return "Nenhum incidente ativo e nenhum componente com problema declarado.";
    case "maintenance":
      return `Manutenção em andamento em ${plural(o.components_under_maintenance, "componente", "componentes")}. Não há indisponibilidade declarada.`;
    default:
      if (o.affected_components === 0) {
        return "Há incidente ativo afetando a plataforma.";
      }
      return `${plural(o.affected_components, "componente afetado", "componentes afetados")}${
        o.components_under_maintenance > 0 ? `; ${o.components_under_maintenance} em manutenção` : ""
      }.`;
  }
}

export function StatusPage() {
  const poll = usePolling((signal) => publicApi.status({ signal }), {
    intervalMs: POLL_INTERVAL_MS,
    maxBackoffMs: POLL_MAX_BACKOFF_MS,
  });

  if (poll.loading && !poll.data) {
    return (
      <section className="container page-section">
        <Loading label="Carregando o status da plataforma…" />
      </section>
    );
  }
  if (!poll.data) {
    return (
      <section className="container page-section">
        <ErrorPanel error={poll.error} onRetry={() => void poll.refresh()} title="Não foi possível carregar o status" />
      </section>
    );
  }

  const s = poll.data;
  const tone = toneForStatus[s.overall.status];
  return (
    <>
      <section className="status-hero">
        <div className="container status-hero-content">
          <p className="eyebrow">STATUS DA PLATAFORMA</p>
          <h1>Digolitus Status Page</h1>
          <div className={`global-status global-status-${tone}`} role="status" aria-live="polite">
            <Badge tone={tone}>{overallLabel[s.overall.status]}</Badge>
            <span>{overallDescription(s)}</span>
          </div>
          {poll.staleError !== undefined && (
            <p className="stale" role="alert">
              Não foi possível atualizar. Exibindo dados de {formatDateTime(s.generated_at)}.
            </p>
          )}
          <div className="status-actions">
            <button type="button" className="secondary-button" onClick={() => void poll.refresh()} disabled={poll.refreshing}>
              {poll.refreshing ? "Atualizando…" : "Atualizar agora"}
            </button>
            <span className="last-check">Dados de {formatDateTime(s.generated_at)}. Atualização automática a cada minuto.</span>
          </div>
          <p className="basis-note">
            Os status são declarados pela equipe de operações. Esta página não realiza verificações automáticas de
            disponibilidade.
          </p>
        </div>
      </section>

      <section className="container page-section" id="incidents" aria-labelledby="incidents-title">
        <div className="section-heading">
          <div>
            <p className="eyebrow">ACOMPANHAMENTO</p>
            <h2 id="incidents-title">Incidentes ativos</h2>
          </div>
          <span className="section-count">{plural(s.active_incidents.length, "ativo", "ativos")}</span>
        </div>
        {s.active_incidents.length === 0 ? (
          <EmptyState title="Nenhum incidente ativo" />
        ) : (
          <div className="event-list">
            {s.active_incidents.map((i) => (
              <IncidentCard key={i.id} incident={i} showTimeline linkTo={`/incidents/${i.id}`} />
            ))}
          </div>
        )}
      </section>

      <section className="container page-section" id="maintenances" aria-labelledby="maintenances-title">
        <div className="section-heading">
          <div>
            <p className="eyebrow">PLANEJAMENTO</p>
            <h2 id="maintenances-title">Manutenções</h2>
          </div>
        </div>
        <h3 className="subheading">Em andamento</h3>
        {s.maintenances.in_progress.length === 0 ? (
          <EmptyState title="Nenhuma manutenção em andamento" />
        ) : (
          <div className="event-list">
            {s.maintenances.in_progress.map((m) => (
              <MaintenanceCard key={m.id} maintenance={m} linkTo={`/maintenances/${m.id}`} />
            ))}
          </div>
        )}
        <h3 className="subheading">Programadas</h3>
        {s.maintenances.upcoming.length === 0 ? (
          <EmptyState title="Nenhuma manutenção programada" />
        ) : (
          <div className="event-list">
            {s.maintenances.upcoming.map((m) => (
              <MaintenanceCard key={m.id} maintenance={m} linkTo={`/maintenances/${m.id}`} />
            ))}
          </div>
        )}
      </section>

      <section className="container page-section" id="components" aria-labelledby="components-title">
        <div className="section-heading">
          <div>
            <p className="eyebrow">SERVIÇOS</p>
            <h2 id="components-title">Componentes</h2>
          </div>
          <span className="section-count">{plural(s.components.length, "componente", "componentes")}</span>
        </div>
        {s.components.length === 0 ? (
          <EmptyState title="Nenhum componente publicado" />
        ) : (
          <ul className="component-list">
            {s.components.map((c) => (
              <li className="component-card" key={c.id}>
                <div>
                  <h3>{c.name}</h3>
                  {c.description && <p>{c.description}</p>}
                </div>
                <Badge tone={toneForStatus[c.status]}>{componentStatusLabel[c.status]}</Badge>
              </li>
            ))}
          </ul>
        )}
      </section>

      <History />
    </>
  );
}

const HISTORY_PAGE = 10;

function History() {
  const [incOffset, setIncOffset] = useState(0);
  const [mtOffset, setMtOffset] = useState(0);
  const incidents = useApi(
    (signal) => publicApi.incidents({ state: "resolved", limit: HISTORY_PAGE, offset: incOffset }, { signal }),
    [incOffset],
  );
  const maintenances = useApi(
    (signal) => publicApi.maintenances({ state: "past", limit: HISTORY_PAGE, offset: mtOffset }, { signal }),
    [mtOffset],
  );

  return (
    <section className="container page-section" id="history" aria-labelledby="history-title">
      <div className="section-heading">
        <div>
          <p className="eyebrow">HISTÓRICO</p>
          <h2 id="history-title">Eventos anteriores</h2>
        </div>
      </div>
      <h3 className="subheading">Incidentes resolvidos</h3>
      {incidents.state.status === "loading" && <Loading />}
      {incidents.state.status === "error" && <ErrorPanel error={incidents.state.error} onRetry={incidents.reload} />}
      {incidents.state.status === "success" &&
        (incidents.state.data.data.length === 0 ? (
          <EmptyState title="Nenhum incidente resolvido" />
        ) : (
          <>
            <div className="event-list">
              {incidents.state.data.data.map((i) => (
                <IncidentCard key={i.id} incident={i} linkTo={`/incidents/${i.id}`} />
              ))}
            </div>
            <Pager pagination={incidents.state.data.pagination} onChange={setIncOffset} />
          </>
        ))}

      <h3 className="subheading">Manutenções concluídas e canceladas</h3>
      {maintenances.state.status === "loading" && <Loading />}
      {maintenances.state.status === "error" && <ErrorPanel error={maintenances.state.error} onRetry={maintenances.reload} />}
      {maintenances.state.status === "success" &&
        (maintenances.state.data.data.length === 0 ? (
          <EmptyState title="Nenhuma manutenção encerrada" />
        ) : (
          <>
            <div className="event-list">
              {maintenances.state.data.data.map((m) => (
                <MaintenanceCard key={m.id} maintenance={m} linkTo={`/maintenances/${m.id}`} />
              ))}
            </div>
            <Pager pagination={maintenances.state.data.pagination} onChange={setMtOffset} />
          </>
        ))}
    </section>
  );
}
