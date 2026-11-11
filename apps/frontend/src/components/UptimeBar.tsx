import { useRef, useState, type KeyboardEvent } from "react";
import type { UptimeDay } from "../api/types";
import { Link } from "../lib/router";
import { formatDay, formatPercent, formatSeconds, uptimeStatusLabel } from "../lib/uptime";

type Size = "lg" | "md" | "sm";

const DURATION_ORDER = [
  ["major_outage", "Interrupção grave"],
  ["partial_outage", "Interrupção parcial"],
  ["degraded_performance", "Performance degradada"],
  ["maintenance", "Manutenção"],
] as const;

/**
 * Daily bar (oldest day first, today last) with the period's uptime. With
 * `details`, each day is a button (one tab stop; arrow keys move between
 * days) that opens what happened that day: time in each non-operational
 * status and the incidents. Without it the bar is a summary image.
 */
export function UptimeBar({
  name,
  days,
  uptimePercent,
  size = "md",
  details = false,
  incidentLink,
}: {
  name: string;
  days: UptimeDay[];
  uptimePercent: number | null;
  size?: Size;
  details?: boolean;
  incidentLink?: (id: string) => string;
}) {
  const [selected, setSelected] = useState<number | null>(null);
  const [focus, setFocus] = useState(days.length - 1);
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const summary = `${name}: ${formatPercent(uptimePercent)} de uptime nos últimos ${days.length} dias`;

  const move = (e: KeyboardEvent, i: number) => {
    const next = e.key === "ArrowLeft" ? i - 1 : e.key === "ArrowRight" ? i + 1 : e.key === "Home" ? 0 : e.key === "End" ? days.length - 1 : null;
    if (next === null || next < 0 || next >= days.length) return;
    e.preventDefault();
    setFocus(next);
    refs.current[next]?.focus();
  };

  const day = selected !== null ? days[selected] : undefined;
  return (
    <div className={`uptime uptime-${size}`}>
      {details ? (
        <div className="uptime-days" role="group" aria-label={summary}>
          {days.map((d, i) => (
            <button
              key={d.date}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="button"
              tabIndex={i === focus ? 0 : -1}
              className={`uptime-day uptime-${d.status}${selected === i ? " uptime-day-selected" : ""}`}
              aria-label={`${formatDay(d.date)}: ${uptimeStatusLabel[d.status]}`}
              aria-pressed={selected === i}
              onClick={() => {
                setFocus(i);
                setSelected(selected === i ? null : i);
              }}
              onKeyDown={(e) => move(e, i)}
            />
          ))}
        </div>
      ) : (
        <div className="uptime-days" role="img" aria-label={summary}>
          {days.map((d) => (
            <span key={d.date} className={`uptime-day uptime-${d.status}`} title={`${formatDay(d.date)}: ${uptimeStatusLabel[d.status]}`} />
          ))}
        </div>
      )}
      <div className="uptime-legend" aria-hidden="true">
        {size !== "sm" && <span>{days.length} dias atrás</span>}
        {size !== "sm" && <span className="uptime-rule" />}
        <span className="uptime-percent">{formatPercent(uptimePercent)} de uptime</span>
        {size !== "sm" && <span className="uptime-rule" />}
        {size !== "sm" && <span>Hoje</span>}
      </div>
      {day && (
        <div className="uptime-detail" role="region" aria-label={`${name} em ${formatDay(day.date)}`}>
          <div className="uptime-detail-head">
            <strong>{formatDay(day.date)}</strong>
            <button type="button" className="link-button" onClick={() => setSelected(null)}>
              Fechar
            </button>
          </div>
          <p className="uptime-detail-status">
            <span className={`uptime-swatch uptime-${day.status}`} aria-hidden="true" />
            {uptimeStatusLabel[day.status]}
            {day.uptime_percent !== null && <span className="muted"> · {formatPercent(day.uptime_percent)} de uptime</span>}
          </p>
          {DURATION_ORDER.filter(([k]) => day.durations[k] > 0).map(([k, label]) => (
            <p key={k} className="uptime-detail-row">
              <span>{label}</span>
              <strong>{formatSeconds(day.durations[k])}</strong>
            </p>
          ))}
          {day.incidents.length > 0 && (
            <ul className="uptime-detail-incidents" aria-label="Incidentes do dia">
              {day.incidents.map((inc) => (
                <li key={inc.id}>{incidentLink ? <Link to={incidentLink(inc.id)}>{inc.title}</Link> : inc.title}</li>
              ))}
            </ul>
          )}
          {day.status === "no_data" && <p className="muted small">Não há histórico registrado para este dia.</p>}
          {day.status === "operational" && day.incidents.length === 0 && (
            <p className="muted small">Nenhuma indisponibilidade registrada neste dia.</p>
          )}
        </div>
      )}
    </div>
  );
}
