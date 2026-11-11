import { createContext, useContext, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { adminApi, presenceApi } from "../api/endpoints";
import type { PresenceArea, PresenceSnapshot } from "../api/types";
import { browserTimeZone, formatRelative } from "../lib/format";
import { tabId, usePresenceBeacon } from "../lib/presence";
import { Link } from "../lib/router";
import { usePolling, type PollingState } from "../lib/usePolling";
import { InfoTip } from "../components/InfoTip";
import { useAuth } from "./authContext";

/** Refresh of the admin panel; the server window is 2 minutes. */
const SNAPSHOT_INTERVAL_MS = 30_000;
const SNAPSHOT_MAX_BACKOFF_MS = 5 * 60_000;

const AREA_LABEL: Record<PresenceArea, string> = {
  status: "Página de status",
  incident: "Detalhe de incidente",
  maintenance: "Detalhe de manutenção",
  announcements: "Comunicados",
  subscriptions: "Assinatura de notificações",
  other: "Outras páginas",
};

type PresenceState = (PollingState<PresenceSnapshot> & { refresh: () => void }) | null;

const PresenceContext = createContext<PresenceState>(null);

/** Snapshot shared by the top counter and the overview panel (one poll). */
export function usePresence(): PresenceState {
  return useContext(PresenceContext);
}

/** Polls the snapshot for users with presence.read; null for everyone else. */
export function PresenceProvider({ children }: { children: ReactNode }) {
  const { can } = useAuth();
  return can("presence.read") ? <PollingPresence>{children}</PollingPresence> : <>{children}</>;
}

function PollingPresence({ children }: { children: ReactNode }) {
  const poll = usePolling((signal) => adminApi.presence(browserTimeZone(), { signal }), {
    intervalMs: SNAPSHOT_INTERVAL_MS,
    maxBackoffMs: SNAPSHOT_MAX_BACKOFF_MS,
  });
  return <PresenceContext.Provider value={poll}>{children}</PresenceContext.Provider>;
}

/**
 * Reports this admin tab (also while in the background, flagged hidden).
 * `screen` is the label other administrators see, e.g. "Incidentes".
 * Detail pages add their heading once it has loaded.
 */
export function AdminPresenceBeacon({ screen, detail, pathname }: { screen: string; detail: boolean; pathname: string }) {
  usePresenceBeacon((hidden) => presenceApi.admin(tabId(), screenLabel(screen, detail), hidden), pathname, {
    whileHidden: true,
    // Detail pages load their heading after the first beat.
    resendAfterMs: detail ? 3_000 : undefined,
  });
  return null;
}

function screenLabel(screen: string, detail: boolean): string {
  if (!detail) return screen;
  const heading = document.querySelector("#admin-main h1")?.textContent?.trim();
  const label = heading && heading !== screen ? `${screen} › ${heading}` : screen;
  return label.length > 120 ? `${label.slice(0, 119)}…` : label;
}

/** "N online" button with a short summary, shown in the sidebar. */
export function PresenceChip() {
  const state = usePresence();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const popId = useId();
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  const data = state?.data;
  if (!data) return null;
  const total = data.public.total + data.admin.total;
  return (
    <div className="presence-chip-wrap" ref={ref}>
      <button
        type="button"
        className="presence-chip"
        aria-expanded={open}
        aria-controls={popId}
        aria-label={`${total} ${total === 1 ? "pessoa acessando" : "pessoas acessando"} agora`}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="presence-dot" aria-hidden="true" />
        {total} online
      </button>
      <div id={popId} className="presence-pop" hidden={!open}>
        <p>
          <span>Página pública</span>
          <strong>{data.public.total}</strong>
        </p>
        <p>
          <span>Usuários no admin</span>
          <strong>{data.admin.total}</strong>
        </p>
        <Link to="/admin" onClick={() => setOpen(false)}>
          Ver detalhes na Visão geral
        </Link>
      </div>
    </div>
  );
}

function timeLabel(iso: string): string {
  const t = new Date(iso);
  return new Intl.DateTimeFormat("pt-BR", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(t);
}

function seenLabel(iso: string, now: Date): string {
  return now.getTime() - new Date(iso).getTime() < 90_000 ? "agora" : formatRelative(iso, now);
}

/** Line chart of public visitors over the last hour. */
function Sparkline({ data }: { data: PresenceSnapshot }) {
  const end = new Date(data.generated_at).getTime();
  // Past minutes come from the samples; the line ends at the live count
  // (the sample of the current minute may still be partial).
  const points = data.history
    .map((s) => ({ x: 60 - (end - new Date(s.minute).getTime()) / 60_000, y: s.public }))
    .filter((p) => p.x >= 0 && p.x <= 59);
  points.push({ x: 60, y: data.public.total });
  if (points.length < 2) {
    return <p className="muted small">A curva da última hora aparece depois de alguns minutos de registro.</p>;
  }
  const max = Math.max(1, ...points.map((p) => p.y));
  const W = 300;
  const H = 46;
  const px = (x: number) => (x / 60) * W;
  const py = (y: number) => H - 3 - (y / max) * (H - 8);
  const line = points.map((p, i) => `${i ? "L" : "M"}${px(p.x).toFixed(1)},${py(p.y).toFixed(1)}`).join(" ");
  const first = points[0];
  const last = points[points.length - 1];
  const area = `${line} L${px(last.x).toFixed(1)},${H} L${px(first.x).toFixed(1)},${H} Z`;
  const min = Math.min(...points.map((p) => p.y));
  return (
    <svg
      className="presence-spark"
      viewBox={`0 0 ${W} ${H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={`Visitantes da página pública na última hora: entre ${min} e ${max}.`}
    >
      <line x1="0" y1={H - 0.5} x2={W} y2={H - 0.5} className="presence-spark-base" />
      <path d={area} className="presence-spark-area" />
      <path d={line} className="presence-spark-line" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

/** "Acessando agora" panel of the overview page. */
export function PresencePanel() {
  const state = usePresence();
  const { me } = useAuth();
  if (!state) return null;
  if (!state.data) {
    return state.error === undefined ? null : (
      <p className="muted presence-error">Não foi possível carregar quem está acessando agora. Nova tentativa em instantes.</p>
    );
  }
  const data = state.data;
  const now = new Date(data.generated_at);
  const max = Math.max(1, ...data.public.areas.map((a) => a.count));
  const windowMin = Math.round(data.window_seconds / 60);
  const oldest = data.history[0];
  return (
    <section className="presence-panel" aria-labelledby="presence-title">
      <h2 id="presence-title" className="visually-hidden">
        Acessando agora
      </h2>
      <div className="presence-col">
        <div className="presence-head">
          <h3>
            <span className="presence-dot" aria-hidden="true" />
            Página pública
            <InfoTip
              label="Sobre a contagem da página pública"
              text={`Contagem aproximada de abas com a página aberta e visível nos últimos ${windowMin} minutos. Robôs e leitores sem JavaScript não entram. Nenhum IP ou cookie é guardado.`}
            />
          </h3>
        </div>
        <p className="presence-big">
          <strong>{data.public.total}</strong>
          <span>{data.public.total === 1 ? "visitante agora" : "visitantes agora"}</span>
        </p>
        <div>
          <Sparkline data={data} />
          <p className="presence-spark-cap">
            <span>{oldest ? `${formatRelative(oldest.minute, now)} · ${oldest.public}` : ""}</span>
            <span>{data.peak_today ? `pico de hoje: ${data.peak_today.count} às ${timeLabel(data.peak_today.at)}` : "sem visitantes hoje"}</span>
          </p>
        </div>
        <ul className="presence-areas">
          {data.public.areas.map((a) => (
            <li key={a.area}>
              <span className="presence-area-label">
                <span>{AREA_LABEL[a.area]}</span>
                <span className="presence-bar" aria-hidden="true">
                  <i style={{ width: `${(a.count / max) * 100}%` }} />
                </span>
              </span>
              <strong>{a.count}</strong>
            </li>
          ))}
        </ul>
      </div>
      <div className="presence-col">
        <div className="presence-head">
          <h3>
            <span className="presence-dot" aria-hidden="true" />
            Usuários no admin
            <InfoTip
              label="Sobre os usuários no admin"
              text={`Quem está com o admin aberto nos últimos ${windowMin} minutos e em qual tela. Ponto cinza: aba em segundo plano.`}
            />
          </h3>
          <small>{data.admin.total === 1 ? "1 conectado" : `${data.admin.total} conectados`}</small>
        </div>
        {data.admin.users.length === 0 ? (
          <p className="muted small">Ninguém com o admin aberto agora.</p>
        ) : (
          <ul className="presence-people">
            {data.admin.users.map((u) => (
              <li key={u.id}>
                <span className={u.hidden ? "presence-avatar idle" : "presence-avatar"} aria-hidden="true">
                  {initials(u.name)}
                </span>
                <span className="presence-who">
                  <strong>
                    {u.name}
                    {u.id === me.user.id && <span className="presence-you">você</span>}
                  </strong>
                  <span>
                    {u.area}
                    {u.hidden && " · aba em segundo plano"}
                  </span>
                </span>
                <span className="presence-when">{seenLabel(u.last_seen_at, now)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  const a = parts[0]?.[0] ?? "?";
  const b = parts.length > 1 ? parts[parts.length - 1][0] : (parts[0]?.[1] ?? "");
  return (a + b).toUpperCase();
}
