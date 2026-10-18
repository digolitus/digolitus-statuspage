import { useState } from "react";
import { publicApi } from "../api/endpoints";
import type { StatusSummary } from "../api/types";
import { Badge, EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { formatDateTime, overallLabel, plural, toneForStatus } from "../lib/format";
import { useApi } from "../lib/useApi";
import { usePolling } from "../lib/usePolling";
import { ComponentList } from "./ComponentList";
import { IncidentCard, MaintenanceCard, MaintenanceRow } from "./views";

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

/**
 * Layout, top to bottom:
 * 1. overall status banner (always);
 * 2. what is happening now — active incidents and in-progress maintenance —
 *    only when there is something, so a healthy page starts with components;
 * 3. components, with groups collapsed;
 * 4. scheduled maintenance as compact rows, only when there are any;
 * 5. history, collapsed and loaded on demand.
 */
export function StatusPage() {
  const poll = usePolling((signal) => publicApi.status({ signal }), {
    intervalMs: POLL_INTERVAL_MS,
    maxBackoffMs: POLL_MAX_BACKOFF_MS,
  });

  if (!poll.data) {
    return (
      <section className="container page-section">
        {poll.error === undefined ? (
          <Loading label="Carregando o status da plataforma…" />
        ) : (
          <ErrorPanel error={poll.error} onRetry={poll.refresh} title="Não foi possível carregar o status" />
        )}
      </section>
    );
  }

  const s = poll.data;
  const tone = toneForStatus[s.overall.status];
  const happeningNow = s.active_incidents.length + s.maintenances.in_progress.length;

  return (
    <>
      <section className="status-banner-wrap" aria-labelledby="overall-title">
        <div className="container">
          <h1 id="overall-title" className="visually-hidden">
            Status da plataforma
          </h1>
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
            <span className="last-check">
              Atualizado em {formatDateTime(s.generated_at)} · status declarado pela equipe de operações, sem verificação
              automática.
            </span>
            <button type="button" className="link-button" onClick={poll.refresh} disabled={poll.refreshing}>
              {poll.refreshing ? "Atualizando…" : "Atualizar agora"}
            </button>
          </div>
        </div>
      </section>

      {happeningNow > 0 && (
        <section className="container page-section" id="now" aria-labelledby="now-title">
          <h2 id="now-title" className="section-title">
            Acontecendo agora
          </h2>
          <div className="event-list">
            {s.active_incidents.map((i) => (
              <IncidentCard key={i.id} incident={i} linkTo={`/incidents/${i.id}`} />
            ))}
            {s.maintenances.in_progress.map((m) => (
              <MaintenanceCard key={m.id} maintenance={m} linkTo={`/maintenances/${m.id}`} />
            ))}
          </div>
        </section>
      )}

      <section className="container page-section" id="components" aria-labelledby="components-title">
        <h2 id="components-title" className="section-title">
          Componentes
        </h2>
        {s.components.length === 0 ? (
          <EmptyState title="Nenhum componente publicado" />
        ) : (
          <ComponentList components={s.components} groups={s.groups} />
        )}
      </section>

      {s.maintenances.upcoming.length > 0 && (
        <section className="container page-section" id="maintenances" aria-labelledby="maintenances-title">
          <h2 id="maintenances-title" className="section-title">
            Manutenções programadas
          </h2>
          <ul className="maintenance-rows">
            {s.maintenances.upcoming.map((m) => (
              <MaintenanceRow key={m.id} maintenance={m} />
            ))}
          </ul>
        </section>
      )}

      <History />
    </>
  );
}

const HISTORY_PAGE = 10;

function History() {
  const [open, setOpen] = useState(false);
  return (
    <section className="container page-section" id="history" aria-labelledby="history-title">
      <h2 id="history-title" className="section-title">
        <button type="button" className="section-toggle" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          <span className="group-chevron" aria-hidden="true">
            ▸
          </span>
          Histórico de eventos
        </button>
      </h2>
      {open && <HistoryContent />}
    </section>
  );
}

function HistoryContent() {
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
    <>
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
    </>
  );
}
