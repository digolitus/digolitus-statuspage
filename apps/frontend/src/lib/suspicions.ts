import type {
  SuspicionCloseReason,
  SuspicionFrequency,
  SuspicionNoticed,
  SuspicionReach,
  SuspicionStatus,
  SuspicionSymptom,
} from "../api/types";
import type { Tone } from "./format";

// Labels for "Reportar suspeita" (the API keeps the raw values).

export const SYMPTOMS = ["down", "slow", "error", "partial", "wrong_data", "other"] as const satisfies readonly SuspicionSymptom[];
export const FREQUENCIES = ["continuous", "intermittent", "once"] as const satisfies readonly SuspicionFrequency[];
export const REACHES = ["only_me", "my_team", "many_places", "unknown"] as const satisfies readonly SuspicionReach[];
export const NOTICED = ["now", "last_30m", "last_2h", "other"] as const satisfies readonly SuspicionNoticed[];
export const CLOSE_REASONS = ["no_evidence", "duplicate", "out_of_scope"] as const satisfies readonly SuspicionCloseReason[];

export const symptomLabel: Record<SuspicionSymptom, string> = {
  down: "Fora do ar",
  slow: "Lentidão",
  error: "Erro ao acessar",
  partial: "Falha em parte das funções",
  wrong_data: "Dados incorretos",
  other: "Outro",
};

export const frequencyLabel: Record<SuspicionFrequency, string> = {
  continuous: "Contínuo",
  intermittent: "Intermitente",
  once: "Aconteceu uma vez",
};

export const reachLabel: Record<SuspicionReach, string> = {
  only_me: "Só comigo",
  my_team: "Minha equipe",
  many_places: "Vários locais",
  unknown: "Não sei",
};

export const noticedLabel: Record<SuspicionNoticed, string> = {
  now: "Agora",
  last_30m: "Últimos 30 min",
  last_2h: "Últimas 2 h",
  other: "Outro horário",
};

export const closeReasonLabel: Record<SuspicionCloseReason, string> = {
  no_evidence: "Sem evidência de falha generalizada",
  duplicate: "Duplicada",
  out_of_scope: "Fora de escopo",
};

/** Status as the team sees it. */
export const adminStatusLabel: Record<SuspicionStatus, string> = {
  pending: "Pendente de triagem",
  published: "Publicada",
  incident: "Virou incidente",
  closed: "Finalizada",
};

/** Status as the reporter sees it: triage stages are internal. */
export const publicStatusLabel: Record<SuspicionStatus, string> = {
  pending: "Em análise",
  published: "Em análise",
  incident: "Virou incidente",
  closed: "Finalizada",
};

export const suspicionTone: Record<SuspicionStatus, Tone> = {
  pending: "warn",
  published: "info",
  incident: "bad",
  closed: "neutral",
};

/** Label of a raw value of any choice (summary and distribution rows). */
export function choiceLabel(value: string): string {
  const all: Record<string, string> = { ...symptomLabel, ...frequencyLabel, ...reachLabel };
  return all[value] ?? value;
}

export function suspicionNumber(n: number): string {
  return `#S-${String(n).padStart(4, "0")}`;
}

export function componentName(c: { name: string; group: string | null }): string {
  return c.group ? `${c.group} · ${c.name}` : c.name;
}
