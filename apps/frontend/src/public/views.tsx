import { useState } from "react";
import type { PublicIncident, PublicMaintenance } from "../api/types";
import { Badge, Timeline, type TimelineEntry } from "../components/ui";
import {
  expectedImpactLabel,
  formatDateTime,
  formatDuration,
  impactLabel,
  incidentStatusLabel,
  maintenanceKindLabel,
  maintenanceStatusLabel,
  toneForIncident,
  toneForMaintenance,
} from "../lib/format";
import { Link } from "../lib/router";

export function ComponentChips({ components }: { components: { id: string; name: string }[] }) {
  if (components.length === 0) {
    return null;
  }
  return (
    <ul className="chips" aria-label="Componentes afetados">
      {components.map((c) => (
        <li key={c.id}>{c.name}</li>
      ))}
    </ul>
  );
}

/**
 * Shows the latest update and, on demand, the earlier ones. Keeps the page
 * short while preserving the full public timeline. full=true always shows
 * everything (detail pages).
 */
function CollapsibleTimeline({ entries, full = false }: { entries: TimelineEntry[]; full?: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const showAll = full || expanded || entries.length <= 1;
  return (
    <>
      <Timeline formatAt={(iso) => formatDateTime(iso)} entries={showAll ? entries : entries.slice(0, 1)} />
      {!full && entries.length > 1 && (
        <button type="button" className="link-button" aria-expanded={expanded} onClick={() => setExpanded((e) => !e)}>
          {expanded ? "Mostrar só a última atualização" : `Ver todas as ${entries.length} atualizações`}
        </button>
      )}
    </>
  );
}

/** Incident card. full=true renders the whole timeline (detail page). */
export function IncidentCard({ incident, full, linkTo }: { incident: PublicIncident; full?: boolean; linkTo?: string }) {
  const updates = incident.updates ?? (incident.latest_update ? [incident.latest_update] : []);
  const entries: TimelineEntry[] = updates.map((u) => ({
    id: u.id,
    label: incidentStatusLabel[u.status],
    tone: toneForIncident[u.status],
    message: u.message,
    at: u.created_at,
  }));
  return (
    <article className="event-card" aria-labelledby={`incident-${incident.id}`}>
      <header className="event-head">
        <h3 id={`incident-${incident.id}`}>{linkTo ? <Link to={linkTo}>{incident.title}</Link> : incident.title}</h3>
        <Badge tone={toneForIncident[incident.status]}>{incidentStatusLabel[incident.status]}</Badge>
      </header>
      <dl className="meta">
        <div>
          <dt>Impacto</dt>
          <dd>{impactLabel[incident.impact]}</dd>
        </div>
        <div>
          <dt>Início</dt>
          <dd>{formatDateTime(incident.started_at)}</dd>
        </div>
        {incident.resolved_at && (
          <>
            <div>
              <dt>Fim</dt>
              <dd>{formatDateTime(incident.resolved_at)}</dd>
            </div>
            <div>
              <dt>Duração</dt>
              <dd>{formatDuration(incident.started_at, incident.resolved_at)}</dd>
            </div>
          </>
        )}
      </dl>
      <ComponentChips components={incident.components} />
      <CollapsibleTimeline entries={entries} full={full} />
    </article>
  );
}

/** Maintenance card. Planned times are shown in the maintenance's own zone. */
export function MaintenanceCard({ maintenance: m, full, linkTo }: { maintenance: PublicMaintenance; full?: boolean; linkTo?: string }) {
  const updates = m.updates ?? (m.latest_update ? [m.latest_update] : []);
  const entries: TimelineEntry[] = updates.map((u) => ({
    id: u.id,
    label: maintenanceKindLabel[u.kind],
    tone: toneForMaintenance[u.status],
    message: u.message,
    at: u.created_at,
  }));
  return (
    <article className="event-card" aria-labelledby={`maintenance-${m.id}`}>
      <header className="event-head">
        <h3 id={`maintenance-${m.id}`}>{linkTo ? <Link to={linkTo}>{m.title}</Link> : m.title}</h3>
        <Badge tone={toneForMaintenance[m.status]}>{maintenanceStatusLabel[m.status]}</Badge>
      </header>
      {m.description && <p>{m.description}</p>}
      <dl className="meta">
        <div>
          <dt>Janela prevista ({m.timezone})</dt>
          <dd>
            {formatDateTime(m.scheduled_start, m.timezone)} → {formatDateTime(m.scheduled_end, m.timezone)}
          </dd>
        </div>
        <div>
          <dt>Impacto esperado</dt>
          <dd>{expectedImpactLabel[m.expected_impact]}</dd>
        </div>
        {m.started_at && (
          <div>
            <dt>Iniciada em</dt>
            <dd>{formatDateTime(m.started_at)}</dd>
          </div>
        )}
        {m.completed_at && (
          <div>
            <dt>Concluída em</dt>
            <dd>{formatDateTime(m.completed_at)}</dd>
          </div>
        )}
        {m.cancelled_at && (
          <div>
            <dt>Cancelada em</dt>
            <dd>{formatDateTime(m.cancelled_at)}</dd>
          </div>
        )}
      </dl>
      {m.start_overdue && <p className="overdue">O início previsto já passou; a manutenção ainda não foi iniciada.</p>}
      {m.end_overdue && <p className="overdue">O término previsto já passou; a manutenção ainda não foi concluída.</p>}
      <ComponentChips components={m.components} />
      <CollapsibleTimeline entries={entries} full={full} />
    </article>
  );
}

/** One-line summary for upcoming maintenance, to keep the page compact. */
export function MaintenanceRow({ maintenance: m }: { maintenance: PublicMaintenance }) {
  return (
    <li className="maintenance-row">
      <div>
        <Link to={`/maintenances/${m.id}`}>{m.title}</Link>
        <span className="muted">
          {" "}
          · {formatDateTime(m.scheduled_start, m.timezone)} → {formatDateTime(m.scheduled_end, m.timezone)}
        </span>
        {m.components.length > 0 && <span className="muted"> · {m.components.map((c) => c.name).join(", ")}</span>}
      </div>
      <Badge tone="info">{expectedImpactLabel[m.expected_impact]}</Badge>
    </li>
  );
}
