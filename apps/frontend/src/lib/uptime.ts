import type { UptimeDay, UptimeDayStatus } from "../api/types";
import { componentStatusLabel } from "./format";

/** Severity used to pick the worst status of a day (no data ranks lowest). */
const severity: Record<UptimeDayStatus, number> = {
  no_data: -1,
  operational: 0,
  maintenance: 1,
  degraded_performance: 2,
  partial_outage: 3,
  major_outage: 4,
};

export const uptimeStatusLabel: Record<UptimeDayStatus, string> = {
  ...componentStatusLabel,
  maintenance: "Manutenção",
  no_data: "Sem dados",
};

/** A bar aggregated from several components (a group or subgroup). */
export interface AggregateBar {
  days: UptimeDay[];
  uptimePercent: number | null;
}

/**
 * Consolidates the bars of several components: each day shows the worst
 * status among them (the same rule as a group's status badge), durations are
 * summed, incidents are merged, and uptime is the mean of the components'
 * uptime, so one component down for an hour does not count as the whole
 * group being down.
 */
export function aggregate(bars: { days: UptimeDay[]; uptime_percent: number | null }[]): AggregateBar | null {
  if (bars.length === 0) return null;
  const n = bars[0].days.length;
  const days: UptimeDay[] = [];
  for (let i = 0; i < n; i++) {
    const day: UptimeDay = {
      date: bars[0].days[i].date,
      status: "no_data",
      uptime_percent: null,
      durations: { major_outage: 0, partial_outage: 0, degraded_performance: 0, maintenance: 0 },
      incidents: [],
    };
    const seen = new Set<string>();
    const ups: number[] = [];
    for (const b of bars) {
      const d = b.days[i];
      if (!d) continue;
      if (severity[d.status] > severity[day.status]) day.status = d.status;
      day.durations.major_outage += d.durations.major_outage;
      day.durations.partial_outage += d.durations.partial_outage;
      day.durations.degraded_performance += d.durations.degraded_performance;
      day.durations.maintenance += d.durations.maintenance;
      for (const inc of d.incidents) {
        if (!seen.has(inc.id)) {
          seen.add(inc.id);
          day.incidents.push(inc);
        }
      }
      if (d.uptime_percent !== null) ups.push(d.uptime_percent);
    }
    day.uptime_percent = mean(ups);
    days.push(day);
  }
  return { days, uptimePercent: mean(bars.map((b) => b.uptime_percent).filter((p): p is number => p !== null)) };
}

function mean(values: number[]): number | null {
  if (values.length === 0) return null;
  return Math.floor((values.reduce((a, b) => a + b, 0) / values.length) * 100) / 100;
}

/** "99,95%" (never rounds a period with downtime up to 100%). */
export function formatPercent(p: number | null): string {
  if (p === null) return "sem dados";
  return `${p.toLocaleString("pt-BR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`;
}

/** "1 h 30 min", "45 min", "menos de 1 min". */
export function formatSeconds(secs: number): string {
  const mins = Math.round(secs / 60);
  if (mins < 1) return "menos de 1 min";
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  if (h === 0) return `${m} min`;
  return m > 0 ? `${h} h ${m} min` : `${h} h`;
}

/** A calendar date ("2026-11-11") as "11 de nov. de 2026", without timezone shifts. */
export function formatDay(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric", timeZone: "UTC" });
}

/** The browser's IANA timezone (UTC when unavailable). */
export function viewerTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
