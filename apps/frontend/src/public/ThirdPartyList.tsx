import { useId, useState } from "react";
import type { PublicThirdPartySource } from "../api/types";
import { Badge } from "../components/ui";
import { externalStatusLabel, formatDateTime, formatRelative, plural, toneForStatus } from "../lib/format";

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function summary(s: PublicThirdPartySource): string {
  const parts = [plural(s.components.length, "componente", "componentes")];
  if (s.stale) {
    parts.push("dados desatualizados");
  } else if (s.last_success_at) {
    parts.push(`atualizado ${formatRelative(s.last_success_at)}`);
  }
  if (s.events.length > 0) {
    parts.push(plural(s.events.length, "evento em aberto", "eventos em aberto"));
  }
  return parts.join(" · ");
}

/** One provider, collapsed by default like component groups. */
function SourceRow({ source: s }: { source: PublicThirdPartySource }) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  return (
    <li className={`group-row${open ? " group-row-open" : ""}`}>
      <button type="button" className="group-toggle" aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((o) => !o)}>
        <span className="group-chevron" aria-hidden="true">
          ▸
        </span>
        <span className="group-title">
          <span className="component-name">{s.name}</span>
          <span className="component-description">{summary(s)}</span>
        </span>
        <Badge tone={toneForStatus[s.status]}>{externalStatusLabel[s.status]}</Badge>
      </button>
      <div id={panelId} hidden={!open} className="third-party-panel">
        {s.stale && (
          <p className="overdue">
            Não foi possível obter dados recentes deste provedor
            {s.last_success_at ? ` desde ${formatDateTime(s.last_success_at)}` : ""}. Consulte a página dele.
          </p>
        )}
        {s.events.length > 0 && (
          <ul className="third-party-events" aria-label={`Eventos em aberto em ${s.name}`}>
            {s.events.map((e, i) => (
              <li key={`${e.name}-${i}`}>
                <span className="tag">{e.kind === "maintenance" ? "Manutenção" : "Incidente"}</span>{" "}
                {e.url ? (
                  <a href={e.url} target="_blank" rel="noopener noreferrer">
                    {e.name}
                  </a>
                ) : (
                  e.name
                )}{" "}
                <span className="muted">({e.status})</span>
              </li>
            ))}
          </ul>
        )}
        <ul className="component-rows nested" aria-label={`Componentes de ${s.name}`}>
          {s.components.map((c) => (
            <li key={c.id} className="component-row">
              <div>
                <span className="component-name">{c.name}</span>
                {c.group_name && <span className="component-description">{c.group_name}</span>}
              </div>
              <Badge tone={toneForStatus[c.status]}>{externalStatusLabel[c.status]}</Badge>
            </li>
          ))}
        </ul>
        <p className="third-party-link">
          <a href={s.page_url} target="_blank" rel="noopener noreferrer">
            Ver status em {hostOf(s.page_url)} ↗
          </a>
        </p>
      </div>
    </li>
  );
}

export function ThirdPartyList({ sources }: { sources: PublicThirdPartySource[] }) {
  return (
    <ul className="component-rows">
      {sources.map((s) => (
        <SourceRow key={s.id} source={s} />
      ))}
    </ul>
  );
}
