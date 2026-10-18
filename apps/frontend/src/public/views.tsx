import type { PublicIncident, PublicMaintenance } from "../api/types";
import { Badge, Timeline } from "../components/ui";
import {
  expectedImpactLabel,
  formatDateTime,
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

/** Incident card; with showTimeline it renders all embedded updates. */
export function IncidentCard({ incident, showTimeline, linkTo }: { incident: PublicIncident; showTimeline?: boolean; linkTo?: string }) {
  const updates = showTimeline ? incident.updates ?? [] : incident.latest_update ? [incident.latest_update] : [];
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
          <div>
            <dt>Resolução</dt>
            <dd>{formatDateTime(incident.resolved_at)}</dd>
          </div>
        )}
      </dl>
      <ComponentChips components={incident.components} />
      <Timeline
        formatAt={(iso) => formatDateTime(iso)}
        entries={updates.map((u) => ({
          id: u.id,
          label: incidentStatusLabel[u.status],
          tone: toneForIncident[u.status],
          message: u.message,
          at: u.created_at,
        }))}
      />
    </article>
  );
}

/** Maintenance card. Planned times are shown in the maintenance's own zone. */
export function MaintenanceCard({ maintenance: m, showTimeline, linkTo }: { maintenance: PublicMaintenance; showTimeline?: boolean; linkTo?: string }) {
  const updates = showTimeline ? m.updates ?? [] : m.latest_update ? [m.latest_update] : [];
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
      <Timeline
        formatAt={(iso) => formatDateTime(iso)}
        entries={updates.map((u) => ({
          id: u.id,
          label: maintenanceKindLabel[u.kind],
          tone: toneForMaintenance[u.status],
          message: u.message,
          at: u.created_at,
        }))}
      />
    </article>
  );
}

