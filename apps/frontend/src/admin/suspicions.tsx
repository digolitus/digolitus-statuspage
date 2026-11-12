import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../api/client";
import { adminApi } from "../api/endpoints";
import type { AdminSuspicion, Feature, IncidentImpact, SuspicionCloseReason, SuspicionDetail } from "../api/types";
import { InfoTip } from "../components/InfoTip";
import { StatusSelect } from "../components/StatusSelect";
import { Badge, EmptyState, ErrorPanel, Field, Loading, Notice, Pager } from "../components/ui";
import { refreshFeatures, useFeatures } from "../lib/features";
import { formatDateTime, IMPACTS, impactLabel, plural } from "../lib/format";
import { Link } from "../lib/router";
import {
  adminStatusLabel,
  choiceLabel,
  CLOSE_REASONS,
  closeReasonLabel,
  componentName,
  frequencyLabel,
  noticedLabel,
  reachLabel,
  suspicionNumber,
  suspicionTone,
  symptomLabel,
} from "../lib/suspicions";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, useSubmit } from "./shared";

// Triage of "Reportar suspeita" (ADR 0019).

const COUNTS_EVENT = "app:suspicion-counts";
const COUNTS_INTERVAL_MS = 60_000;

/** Pending suspicions in the user's queue, for the menu badge (null = hidden). */
export function useSuspicionBadge(): number | null {
  const features = useFeatures();
  const can = useAuth().can("suspicions.manage");
  const [count, setCount] = useState<number | null>(null);
  useEffect(() => {
    if (!features.suspicions || !can) return;
    let alive = true;
    const load = () =>
      adminApi.suspicions.list({ status: "pending", limit: 1 }).then(
        (r) => alive && setCount(r.counts.pending),
        () => undefined,
      );
    void load();
    const timer = window.setInterval(load, COUNTS_INTERVAL_MS);
    window.addEventListener(COUNTS_EVENT, load);
    return () => {
      alive = false;
      window.clearInterval(timer);
      window.removeEventListener(COUNTS_EVENT, load);
    };
  }, [features.suspicions, can]);
  return features.suspicions && can ? count : null;
}

function countsChanged() {
  window.dispatchEvent(new Event(COUNTS_EVENT));
}

function age(iso: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60_000));
  if (mins < 60) return `${mins} min`;
  const h = Math.floor(mins / 60);
  if (h < 48) return `${h} h ${mins % 60} min`;
  return `${Math.floor(h / 24)} dias`;
}

type StatusFilter = "open" | "pending" | "published" | "incident" | "closed" | "";
type Queue = "mine" | "general" | "all";

export function SuspicionsPage() {
  const { me, teamOnly } = useAuth();
  const scoped = teamOnly("suspicions.manage");
  const [status, setStatus] = useState<StatusFilter>("open");
  const [queue, setQueue] = useState<Queue>(scoped || me.user.teams.length > 0 ? "mine" : "all");
  const [offset, setOffset] = useState(0);
  const list = useApi((signal) => adminApi.suspicions.list({ status: status || undefined, queue, limit: 20, offset }, { signal }), [status, queue, offset]);
  return (
    <div className="admin-content">
      <PageHeader
        title="Suspeitas"
        info="Relatos de visitantes da página pública. Relatos do mesmo componente entram na suspeita aberta dele. Publique para mostrar na página pública, abra um incidente se confirmar ou finalize a análise."
      />
      <div className="filters suspicion-filters">
        <label>
          <span className="visually-hidden">Estado</span>
          <select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as StatusFilter);
              setOffset(0);
            }}
          >
            <option value="open">Abertas</option>
            <option value="pending">Pendentes de triagem</option>
            <option value="published">Publicadas</option>
            <option value="incident">Viraram incidente</option>
            <option value="closed">Finalizadas</option>
            <option value="">Todas</option>
          </select>
        </label>
        <label>
          <span className="visually-hidden">Fila</span>
          <select
            value={queue}
            onChange={(e) => {
              setQueue(e.target.value as Queue);
              setOffset(0);
            }}
          >
            <option value="mine">Meus times</option>
            {!scoped && <option value="general">Fila geral (sem time)</option>}
            {!scoped && <option value="all">Todas as filas</option>}
          </select>
        </label>
      </div>
      {list.state.status === "loading" && <Loading />}
      {list.state.status === "error" && <ErrorPanel error={list.state.error} onRetry={list.reload} />}
      {list.state.status === "success" &&
        (list.state.data.data.length === 0 ? (
          <EmptyState title="Nenhuma suspeita nesta fila" />
        ) : (
          <>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th scope="col">Componente</th>
                    <th scope="col">Sintoma principal</th>
                    <th scope="col">Relatos</th>
                    <th scope="col">Estado</th>
                    <th scope="col">Time</th>
                    <th scope="col">Aberta há</th>
                  </tr>
                </thead>
                <tbody>
                  {list.state.data.data.map((s) => (
                    <QueueRow key={s.id} s={s} />
                  ))}
                </tbody>
              </table>
            </div>
            <Pager pagination={list.state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}

/** Pending for more than 30 minutes: highlighted in the queue. */
function isLate(iso: string): boolean {
  return Date.now() - new Date(iso).getTime() > 30 * 60_000;
}

function QueueRow({ s }: { s: AdminSuspicion }) {
  const late = s.status === "pending" && isLate(s.opened_at);
  return (
    <tr>
      <td>
        <Link to={`/admin/suspicions/${s.id}`}>
          <strong>{componentName(s.component)}</strong>
        </Link>
        <div className="muted small">{suspicionNumber(s.number)}</div>
      </td>
      <td>{s.summary.symptom ? choiceLabel(s.summary.symptom) : "—"}</td>
      <td className="num">{s.report_count}</td>
      <td>
        <Badge tone={suspicionTone[s.status]}>{adminStatusLabel[s.status]}</Badge>
      </td>
      <td>{s.team ? s.team.name : <span className="muted">Fila geral</span>}</td>
      <td className={late ? "age age-late" : "age"}>{age(s.opened_at)}</td>
    </tr>
  );
}

export function SuspicionDetailPage({ id }: { id: string }) {
  const res = useApi((signal) => adminApi.suspicions.get(id, { signal }), [id]);
  if (res.state.status === "loading") return <Loading />;
  if (res.state.status === "error")
    return (
      <div className="admin-content">
        <ErrorPanel error={res.state.error} onRetry={res.reload} />
      </div>
    );
  return <Detail initial={res.state.data} onChange={res.setData} />;
}

type Dialog = "none" | "close" | "incident";

function Detail({ initial: d, onChange }: { initial: SuspicionDetail; onChange: (d: SuspicionDetail) => void }) {
  const { me, can } = useAuth();
  const [dialog, setDialog] = useState<Dialog>("none");
  const act = useSubmit();
  const open = d.status === "pending" || d.status === "published";
  const update = (next: SuspicionDetail | undefined) => {
    if (next) {
      onChange(next);
      countsChanged();
    }
  };
  const mine = d.assignee?.id === me.user.id;
  return (
    <div className="admin-content">
      <p className="breadcrumb small">
        <Link to="/admin/suspicions">Suspeitas</Link> / {suspicionNumber(d.number)}
      </p>
      <div className="admin-page-header">
        <h1 className="suspicion-title">
          {componentName(d.component)} <Badge tone={suspicionTone[d.status]}>{adminStatusLabel[d.status]}</Badge>
        </h1>
        {open && (
          <div className="admin-page-actions">
            {d.status === "pending" && (
              <button type="button" className="secondary-button" disabled={act.busy} onClick={async () => update(await act.run(() => adminApi.suspicions.publish(d.id), "Suspeita publicada."))}>
                Publicar
              </button>
            )}
            {can("incidents.manage") && (
              <button type="button" className="primary-button" onClick={() => setDialog("incident")}>
                Abrir incidente
              </button>
            )}
            <button type="button" className="secondary-button" onClick={() => setDialog("close")}>
              Finalizar análise
            </button>
          </div>
        )}
      </div>
      <FormStatus error={act.formError} success={act.success} />
      {dialog === "close" && <CloseForm d={d} onDone={(n) => (update(n), setDialog("none"))} onCancel={() => setDialog("none")} />}
      {dialog === "incident" && <IncidentForm d={d} onDone={(n) => (update(n), setDialog("none"))} onCancel={() => setDialog("none")} />}
      <div className="suspicion-detail">
        <div className="suspicion-main">
          <Reports d={d} onBlocked={() => update(d)} />
          <Conversation d={d} onChange={update} />
        </div>
        <aside className="panel suspicion-side">
          <dl className="kv">
            <dt>Estado</dt>
            <dd>{adminStatusLabel[d.status]}</dd>
            <dt>Time</dt>
            <dd>{d.team?.name ?? "Fila geral"}</dd>
            <dt>Aberta</dt>
            <dd>{formatDateTime(d.opened_at)}</dd>
            <dt>Último relato</dt>
            <dd>{formatDateTime(d.last_report_at)}</dd>
            {d.published_at && (
              <>
                <dt>Publicada</dt>
                <dd>
                  {formatDateTime(d.published_at)}
                  {d.published_by && ` por ${d.published_by.name}`}
                </dd>
              </>
            )}
            {d.incident && (
              <>
                <dt>Incidente</dt>
                <dd>
                  <Link to={`/admin/incidents/${d.incident.id}`}>{d.incident.title}</Link>
                </dd>
              </>
            )}
            {d.closed_at && d.close_reason && (
              <>
                <dt>Finalizada</dt>
                <dd>
                  {formatDateTime(d.closed_at)}
                  {d.closed_by && ` por ${d.closed_by.name}`} · {closeReasonLabel[d.close_reason]}
                </dd>
              </>
            )}
            <dt>Responsável</dt>
            <dd>
              {d.assignee?.name ?? "Ninguém"}{" "}
              <button
                type="button"
                className="link-button"
                disabled={act.busy}
                onClick={async () => update(await act.run(() => adminApi.suspicions.assign(d.id, mine ? "" : me.user.id)))}
              >
                {mine ? "Liberar" : "Assumir"}
              </button>
            </dd>
          </dl>
        </aside>
      </div>
    </div>
  );
}

function Reports({ d, onBlocked }: { d: SuspicionDetail; onBlocked: () => void }) {
  const { teamOnly } = useAuth();
  const canBlock = !teamOnly("suspicions.manage");
  const [blocked, setBlocked] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  const max = Math.max(1, ...d.distribution.symptom.map((c) => c.count));
  async function toggle(rid: string, next: boolean) {
    setError(null);
    try {
      const r = await adminApi.reporters.block(rid, next);
      setBlocked((b) => ({ ...b, [rid]: r.blocked }));
      onBlocked();
    } catch (e) {
      setError(errorMessage(e));
    }
  }
  return (
    <section className="panel" aria-labelledby="reports-title">
      <div className="section-heading">
        <h2 id="reports-title">
          Relatos <span className="count-pill">{d.report_count}</span>
        </h2>
      </div>
      <div className="dist">
        {d.distribution.symptom.map((c) => (
          <div key={c.value} className="dist-row">
            <span>{choiceLabel(c.value)}</span>
            <span className="dist-track">
              <i style={{ width: `${(c.count / max) * 100}%` }} />
            </span>
            <span className="num">{c.count}</span>
          </div>
        ))}
      </div>
      <div className="admin-item-tags">
        {[...d.distribution.frequency, ...d.distribution.reach].map((c) => (
          <span key={c.value} className="tag">
            {choiceLabel(c.value)} {c.count}
          </span>
        ))}
      </div>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      <details className="reports-details">
        <summary>Ver {plural(d.reports.length, "o relato", `os ${d.reports.length} relatos`)}</summary>
        <ol className="conversation">
          {d.reports.map((r) => {
            const isBlocked = blocked[r.reporter.id] ?? r.reporter.blocked;
            return (
              <li key={r.id} className="msg">
                <small>
                  {r.reporter.name} · {r.reporter.email} · {formatDateTime(r.created_at)}
                  {r.location && ` · ${r.location}`}
                </small>
                <div className="admin-item-tags">
                  <span className="tag">{symptomLabel[r.symptom]}</span>
                  <span className="tag">{frequencyLabel[r.frequency]}</span>
                  <span className="tag">{reachLabel[r.reach]}</span>
                  <span className="tag">
                    Percebeu: {r.noticed === "other" ? formatDateTime(r.noticed_at) : noticedLabel[r.noticed].toLowerCase()}
                  </span>
                </div>
                <p>{r.description}</p>
                {r.error_ref && <code className="small">{r.error_ref}</code>}
                {canBlock && (
                  <button type="button" className="link-button small" onClick={() => void toggle(r.reporter.id, !isBlocked)}>
                    {isBlocked ? "Desbloquear quem reportou" : "Bloquear quem reportou"}
                  </button>
                )}
              </li>
            );
          })}
        </ol>
      </details>
    </section>
  );
}

function Conversation({ d, onChange }: { d: SuspicionDetail; onChange: (d: SuspicionDetail | undefined) => void }) {
  const [kind, setKind] = useState<"internal" | "reporter">("internal");
  const [to, setTo] = useState("");
  const [body, setBody] = useState("");
  const submit = useSubmit();
  const reporters = Array.from(new Map(d.reports.map((r) => [r.reporter.id, r.reporter.name])).entries());
  const target = to || reporters[0]?.[0] || "";
  const targetName = reporters.find(([rid]) => rid === target)?.[1] ?? "";
  async function send(e: FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    const next = await submit.run(() => adminApi.suspicions.message(d.id, kind, body.trim(), kind === "reporter" ? target : undefined));
    if (next) {
      setBody("");
      onChange(next);
    }
  }
  return (
    <section className="panel" aria-labelledby="conv-title">
      <h2 id="conv-title">Conversa e anotações</h2>
      <ol className="conversation">
        <li className="sys">
          {formatDateTime(d.opened_at)} · Suspeita aberta
        </li>
        {d.messages.map((m) => (
          <li key={m.id} className={m.kind === "internal" ? "msg msg-internal" : m.kind === "to_reporter" ? "msg msg-team" : "msg"}>
            <small>
              {m.kind === "internal" && `Anotação interna · ${m.author_name}`}
              {m.kind === "to_reporter" && `Para ${m.reporter?.name ?? "quem reportou"} · ${m.author_name}`}
              {m.kind === "from_reporter" && (m.reporter?.name ?? m.author_name)} · {formatDateTime(m.created_at)}
            </small>
            <p>{m.body}</p>
          </li>
        ))}
        {d.published_at && <li className="sys">{formatDateTime(d.published_at)} · Publicada{d.published_by && ` por ${d.published_by.name}`}</li>}
        {d.closed_at && (
          <li className="sys">
            {formatDateTime(d.closed_at)} · Análise finalizada: {d.close_message}
          </li>
        )}
      </ol>
      <form className={kind === "internal" ? "form composer composer-internal" : "form composer"} onSubmit={send} noValidate>
        <div className="segmented" role="radiogroup" aria-label="Tipo de texto">
          {(
            [
              ["internal", "Anotação interna"],
              ["reporter", "Mensagem a quem reportou"],
            ] as const
          ).map(([value, label]) => (
            <label key={value} className={kind === value ? "segmented-option segmented-option-active" : "segmented-option"}>
              <input type="radio" className="visually-hidden" name="composer-kind" checked={kind === value} onChange={() => setKind(value)} />
              {label}
            </label>
          ))}
        </div>
        {kind === "reporter" && reporters.length > 1 && (
          <Field label="Para">
            {(p) => (
              <select {...p} value={target} onChange={(e) => setTo(e.target.value)}>
                {reporters.map(([rid, name]) => (
                  <option key={rid} value={rid}>
                    {name}
                  </option>
                ))}
              </select>
            )}
          </Field>
        )}
        <Field label={kind === "internal" ? "Anotação" : "Mensagem"} error={submit.fieldErrors.body}>
          {(p) => (
            <textarea
              {...p}
              rows={3}
              maxLength={2000}
              value={body}
              placeholder={kind === "internal" ? "Só o time vê esta anotação" : `${targetName} recebe por e-mail e em Minhas suspeitas`}
              onChange={(e) => setBody(e.target.value)}
            />
          )}
        </Field>
        <FormStatus error={submit.formError} success={null} />
        <div className="form-actions composer-actions">
          <span className="muted small">{kind === "internal" ? "Visível só para o time" : `Vai para ${targetName}`}</span>
          <button type="submit" className="primary-button" disabled={submit.busy || !body.trim()}>
            {kind === "internal" ? "Salvar anotação" : "Enviar mensagem"}
          </button>
        </div>
      </form>
    </section>
  );
}

function CloseForm({ d, onDone, onCancel }: { d: SuspicionDetail; onDone: (n: SuspicionDetail | undefined) => void; onCancel: () => void }) {
  const [reason, setReason] = useState<SuspicionCloseReason>("no_evidence");
  const [message, setMessage] = useState("");
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "message", message, 2000, "Mensagem");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const n = await submit.run(() => adminApi.suspicions.close(d.id, reason, message.trim()));
    if (n) onDone(n);
  }
  return (
    <form className="form panel dialog-panel" onSubmit={handle} noValidate aria-label="Finalizar análise">
      <h2>Finalizar análise</h2>
      <fieldset className="field">
        <legend>Motivo</legend>
        <div className="choice-group">
          {CLOSE_REASONS.map((r) => (
            <label key={r} className={reason === r ? "choice choice-active" : "choice"}>
              <input type="radio" name="close-reason" checked={reason === r} onChange={() => setReason(r)} />
              {closeReasonLabel[r]}
            </label>
          ))}
        </div>
      </fieldset>
      <Field label={`Mensagem ${plural(d.report_count, "a quem reportou", `aos ${d.report_count} que reportaram`)}`} error={submit.fieldErrors.message}>
        {(p) => <textarea {...p} rows={3} maxLength={2000} value={message} onChange={(e) => setMessage(e.target.value)} />}
      </Field>
      <FormStatus error={submit.formError} success={null} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          Finalizar
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}

/** Impact suggested by what was reported most. */
function suggestedImpact(d: SuspicionDetail): IncidentImpact {
  const top = d.summary.symptom;
  if (top === "down") return d.summary.reach === "many_places" ? "major_outage" : "partial_outage";
  if (top === "error" || top === "partial") return "partial_outage";
  return "degraded_performance";
}

function IncidentForm({ d, onDone, onCancel }: { d: SuspicionDetail; onDone: (n: SuspicionDetail | undefined) => void; onCancel: () => void }) {
  const [v, setV] = useState({
    title: `${d.summary.symptom ? choiceLabel(d.summary.symptom) : "Problema"} em ${d.component.name}`,
    impact: suggestedImpact(d),
    message: `Estamos investigando relatos de ${(d.summary.symptom ? choiceLabel(d.summary.symptom) : "problemas").toLowerCase()} em ${d.component.name}.`,
  });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "title", v.title, 200, "Título");
    requireText(errors, "message", v.message, 5000, "Mensagem");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const n = await submit.run(() =>
      adminApi.suspicions.openIncident(d.id, { title: v.title.trim(), message: v.message.trim(), impact: v.impact, component_ids: [d.component.id] }),
    );
    if (n) onDone(n);
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form panel dialog-panel" onSubmit={handle} noValidate aria-label="Abrir incidente">
      <h2>
        Novo incidente <span className="tag">a partir de {suspicionNumber(d.number)}</span>
      </h2>
      <Field label="Título" error={fe.title}>
        {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => setV({ ...v, title: e.target.value })} />}
      </Field>
      <div className="form-grid">
        <Field label="Componente">{(p) => <input {...p} value={componentName(d.component)} readOnly />}</Field>
        <Field label="Impacto" error={fe.impact} info="Sugerido pela abrangência e pelo sintoma mais relatado. Ajuste se precisar.">
          {(p) => <StatusSelect {...p} value={v.impact} options={IMPACTS} labels={impactLabel} onChange={(x) => setV({ ...v, impact: x as IncidentImpact })} />}
        </Field>
      </div>
      <Field label="Mensagem pública inicial" error={fe.message}>
        {(p) => <textarea {...p} rows={3} maxLength={5000} value={v.message} onChange={(e) => setV({ ...v, message: e.target.value })} />}
      </Field>
      <FormStatus error={submit.formError} success={null} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Abrindo…" : "Abrir incidente"}
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}

/** Configurações › Funcionalidades. */
export function FeaturesPage() {
  const can = useAuth().can("features.manage");
  const list = useApi((signal) => adminApi.features.list({ signal }), []);
  const [error, setError] = useState<string | null>(null);
  const info: Record<Feature["key"], string> = {
    suspicions:
      "Desligada: o link some da página pública, as telas e a API respondem como inexistentes e o menu Suspeitas some. Nada é apagado.",
  };
  async function toggle(f: Feature) {
    setError(null);
    try {
      const next = await adminApi.features.set(f.key, !f.enabled);
      if (list.state.status === "success") list.setData({ data: list.state.data.data.map((x) => (x.key === next.key ? next : x)) });
      refreshFeatures();
    } catch (e) {
      setError(errorMessage(e));
    }
  }
  return (
    <div className="admin-content">
      <PageHeader title="Funcionalidades" />
      {!can && <ReadOnlyNote what="ligar ou desligar funcionalidades" />}
      {error && <Notice tone="warn">{error}</Notice>}
      {list.state.status === "loading" && <Loading />}
      {list.state.status === "error" && <ErrorPanel error={list.state.error} onRetry={list.reload} />}
      {list.state.status === "success" && (
        <ul className="admin-list">
          {list.state.data.data.map((f) => (
            <li key={f.key} className="admin-item feature-row">
              <div>
                <strong className="feature-name">
                  {f.label} <InfoTip label={`Sobre ${f.label}`} text={info[f.key]} />
                </strong>
                <span className="muted small">
                  {f.locked
                    ? "Travada pelo servidor (SUSPICIONS_ENABLED=false)"
                    : f.updated_at
                      ? `${f.enabled ? "Ligada" : "Desligada"}${f.updated_by ? ` por ${f.updated_by}` : ""} em ${formatDateTime(f.updated_at)}`
                      : "Desligada"}
                </span>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={f.enabled}
                aria-label={f.label}
                className="switch"
                disabled={!can || f.locked}
                onClick={() => void toggle(f)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
