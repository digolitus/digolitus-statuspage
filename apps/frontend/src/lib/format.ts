import type {
  ComponentStatus,
  ExpectedImpact,
  IncidentImpact,
  IncidentStatus,
  MaintenanceStatus,
  MaintenanceUpdateKind,
  OverallStatus,
} from "../api/types";

export const componentStatusLabel: Record<ComponentStatus, string> = {
  operational: "Operacional",
  degraded_performance: "Performance degradada",
  partial_outage: "Interrupção parcial",
  major_outage: "Interrupção grave",
  maintenance: "Em manutenção",
};

/** Third-party statuses add "unknown" (unrecognized or stale data). */
export const externalStatusLabel: Record<OverallStatus, string> = {
  ...componentStatusLabel,
  unknown: "Desconhecido",
};

export const overallLabel: Record<OverallStatus, string> = {
  operational: "Todos os componentes declarados como operacionais",
  degraded_performance: "Performance degradada",
  partial_outage: "Interrupção parcial",
  major_outage: "Interrupção grave",
  maintenance: "Manutenção em andamento",
  unknown: "Status não informado",
};

export const incidentStatusLabel: Record<IncidentStatus, string> = {
  investigating: "Investigando",
  identified: "Causa identificada",
  monitoring: "Monitorando",
  resolved: "Resolvido",
};

/** Incident impact and maintenance expected impact share the status scale. */
export const impactLabel: Record<IncidentImpact, string> = {
  operational: "Operacional",
  degraded_performance: "Performance degradada",
  partial_outage: "Interrupção parcial",
  major_outage: "Interrupção grave",
};

/** Impact options in severity order. */
export const IMPACTS = ["operational", "degraded_performance", "partial_outage", "major_outage"] as const satisfies readonly IncidentImpact[];

export const maintenanceStatusLabel: Record<MaintenanceStatus, string> = {
  scheduled: "Programada",
  in_progress: "Em andamento",
  completed: "Concluída",
  cancelled: "Cancelada",
};

export const expectedImpactLabel: Record<ExpectedImpact, string> = impactLabel;

export const maintenanceKindLabel: Record<MaintenanceUpdateKind, string> = {
  created: "Programada",
  message: "Atualização",
  started: "Iniciada",
  completed: "Concluída",
  cancelled: "Cancelada",
};

/** Visual tone used for colour + icon; text always accompanies it. */
export type Tone = "ok" | "info" | "warn" | "bad" | "critical" | "neutral";

export const toneForStatus: Record<OverallStatus, Tone> = {
  operational: "ok",
  maintenance: "info",
  degraded_performance: "warn",
  partial_outage: "bad",
  major_outage: "critical",
  unknown: "neutral",
};

export const toneForIncident: Record<IncidentStatus, Tone> = {
  investigating: "bad",
  identified: "warn",
  monitoring: "info",
  resolved: "ok",
};

export const toneForMaintenance: Record<MaintenanceStatus, Tone> = {
  scheduled: "info",
  in_progress: "warn",
  completed: "ok",
  cancelled: "neutral",
};

export const toneIcon: Record<Tone, string> = {
  ok: "✓",
  info: "ℹ",
  warn: "!",
  bad: "▲",
  critical: "✕",
  neutral: "–",
};

/**
 * Formats an instant for display. With timeZone the wall time is shown in
 * that IANA zone (e.g. a maintenance's planning zone); otherwise in the
 * viewer's zone. The zone is always printed, so the moment is unambiguous.
 */
export function formatDateTime(iso: string | null | undefined, timeZone?: string): string {
  if (!iso) {
    return "—";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  // dateStyle/timeStyle cannot be combined with timeZoneName, so the
  // components are listed explicitly.
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZone,
    timeZoneName: "short",
  }).format(date);
}

/** Short relative description, e.g. "há 5 min". */
export function formatRelative(iso: string, now: Date = new Date()): string {
  const diff = (new Date(iso).getTime() - now.getTime()) / 1000;
  const rtf = new Intl.RelativeTimeFormat("pt-BR", { numeric: "auto" });
  const abs = Math.abs(diff);
  if (abs < 60) return rtf.format(Math.round(diff), "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  return rtf.format(Math.round(diff / 86400), "day");
}

/**
 * Converts an instant to the "YYYY-MM-DDTHH:MM" wall time in timeZone, for
 * pre-filling datetime-local inputs. The server interprets the value in the
 * same zone, so the browser's own zone never affects the result.
 */
export function toZonedInput(iso: string, timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(iso));
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "00";
  return `${get("year")}-${get("month")}-${get("day")}T${get("hour")}:${get("minute")}`;
}

const FALLBACK_ZONES = ["UTC", "America/Sao_Paulo", "America/Manaus", "America/New_York", "Europe/Lisbon", "Europe/London"];

/** IANA zones available in this browser. */
export function timeZones(): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  const zones = intl.supportedValuesOf?.("timeZone") ?? FALLBACK_ZONES;
  return zones.includes("UTC") ? zones : ["UTC", ...zones];
}

/** The viewer's zone, used only as a visible, editable default. */
export function browserTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

export function plural(n: number, singular: string, pluralForm: string): string {
  return `${n} ${n === 1 ? singular : pluralForm}`;
}

/** Local calendar day (viewer's zone) used to group announcements. */
export function dayKey(iso: string): string {
  return new Intl.DateTimeFormat("en-CA", { year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date(iso));
}

export function dayLabel(iso: string, now = new Date()): string {
  const key = dayKey(iso);
  if (key === dayKey(now.toISOString())) return "Hoje";
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (key === dayKey(yesterday.toISOString())) return "Ontem";
  const label = new Intl.DateTimeFormat("pt-BR", { weekday: "long", day: "numeric", month: "long", year: "numeric" }).format(new Date(iso));
  return label.charAt(0).toUpperCase() + label.slice(1);
}

/** Whether an instant is still in the future (e.g. a temporary lock). */
export function isFuture(iso: string | null | undefined): boolean {
  return Boolean(iso) && new Date(iso as string).getTime() > Date.now();
}

/** "2 h 15 min", "3 dias e 4 h" — the length of an incident or window. */
export function formatDuration(fromIso: string, toIso: string): string {
  const mins = Math.max(0, Math.round((Date.parse(toIso) - Date.parse(fromIso)) / 60000));
  if (mins < 1) return "menos de 1 min";
  const d = Math.floor(mins / 1440);
  const h = Math.floor((mins % 1440) / 60);
  const m = mins % 60;
  if (d > 0) return h > 0 ? `${plural(d, "dia", "dias")} e ${h} h` : plural(d, "dia", "dias");
  if (h > 0) return m > 0 ? `${h} h ${m} min` : `${h} h`;
  return `${m} min`;
}

/** An instant as a datetime-local value in the browser's zone. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
