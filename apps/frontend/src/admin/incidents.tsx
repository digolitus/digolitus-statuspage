import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Incident, IncidentCreate, IncidentImpact, IncidentStatus, IncidentTemplate } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager, Timeline } from "../components/ui";
import { StatusSelect } from "../components/StatusSelect";
import { browserTimeZone, formatDateTime, IMPACTS, impactLabel, incidentStatusLabel, toneForIncident, formatDuration, toLocalInput } from "../lib/format";
import { Link, navigate } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { ComponentPicker, FormStatus, PageHeader, ReadOnlyNote, requireText, TemplateSelect, useCanForComponents, useSubmit } from "./shared";

const OPEN_STATUSES: Exclude<IncidentStatus, "resolved">[] = ["investigating", "identified", "monitoring"];
const ALL_STATUSES = Object.keys(incidentStatusLabel) as IncidentStatus[];

function useActiveIncidentTemplates() {
  return useApi((signal) => adminApi.incidentTemplates.list({ is_active: true, limit: 100 }, { signal }), []);
}

export function IncidentsPage() {
  const [state, setStateFilter] = useState<"active" | "resolved" | "">("active");
  const [offset, setOffset] = useState(0);
  const list = useApi((signal) => adminApi.incidents.list({ state: state || undefined, limit: 20, offset }, { signal }), [state, offset]);
  const canManage = useAuth().can("incidents.manage");
  return (
    <div className="admin-content">
      <PageHeader title="Incidentes">
        {canManage && (
          <Link className="primary-button" to="/admin/incidents/new">
            Novo incidente
          </Link>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what="abrir ou atualizar incidentes" />}
      <div className="filters" role="group" aria-label="Filtrar incidentes">
        {([
          ["active", "Ativos"],
          ["resolved", "Resolvidos"],
          ["", "Todos"],
        ] as const).map(([value, label]) => (
          <button
            key={label}
            type="button"
            aria-pressed={state === value}
            className={state === value ? "chip-button chip-button-active" : "chip-button"}
            onClick={() => {
              setStateFilter(value);
              setOffset(0);
            }}
          >
            {label}
          </button>
        ))}
      </div>
      {list.state.status === "loading" && <Loading />}
      {list.state.status === "error" && <ErrorPanel error={list.state.error} onRetry={list.reload} />}
      {list.state.status === "success" &&
        (list.state.data.data.length === 0 ? (
          <EmptyState title="Nenhum incidente encontrado" />
        ) : (
          <>
            <ul className="admin-list">
              {list.state.data.data.map((i) => (
                <li key={i.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <Link to={`/admin/incidents/${i.id}`}>
                        <strong>{i.title}</strong>
                      </Link>
                      <div className="admin-item-tags">
                        <span className="tag">Impacto {impactLabel[i.impact].toLowerCase()}</span>
                        <span className="tag">Início {formatDateTime(i.started_at)}</span>
                        {i.components.map((c) => (
                          <span className="tag" key={c.id}>
                            {c.name}
                          </span>
                        ))}
                      </div>
                      {i.latest_update && <p className="muted clamp">{i.latest_update.message}</p>}
                    </div>
                    <Badge tone={toneForIncident[i.status]}>{incidentStatusLabel[i.status]}</Badge>
                  </div>
                </li>
              ))}
            </ul>
            <Pager pagination={list.state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}

/** Converts a datetime-local value (viewer's zone, shown in the UI) to RFC 3339. */
function localInputToISO(value: string): string | undefined {
  if (!value) return undefined;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

export function IncidentNewPage() {
  const templates = useActiveIncidentTemplates();
  const [v, setV] = useState({
    title: "",
    message: "",
    impact: "degraded_performance" as IncidentImpact,
    status: "investigating" as Exclude<IncidentStatus, "resolved">,
    component_ids: [] as string[],
    started_at: "",
    internal_notes: "",
    template_id: "",
  });
  const submit = useSubmit();
  const creationTemplates = templates.state.status === "success" ? templates.state.data.data.filter((t) => t.status !== "resolved") : [];

  const apply = (t: IncidentTemplate) =>
    setV((s) => ({
      ...s,
      title: t.title,
      message: t.message,
      impact: t.impact,
      status: t.status === "resolved" ? "investigating" : t.status,
      template_id: t.id,
    }));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "title", v.title, 200, "Título");
    requireText(errors, "message", v.message, 5000, "Mensagem");
    if (Object.keys(errors).length) {
      submit.setFieldErrors(errors);
      return;
    }
    const input: IncidentCreate = {
      title: v.title.trim(),
      message: v.message.trim(),
      impact: v.impact,
      status: v.status,
      component_ids: v.component_ids,
    };
    const started = localInputToISO(v.started_at);
    if (started) input.started_at = started;
    if (v.internal_notes.trim()) input.internal_notes = v.internal_notes.trim();
    if (v.template_id) input.template_id = v.template_id;
    const created = await submit.run(() => adminApi.incidents.create(input));
    if (created) navigate(`/admin/incidents/${created.id}`);
  }

  const fe = submit.fieldErrors;
  return (
    <div className="admin-content">
      <PageHeader title="Novo incidente" />
      <form className="form panel" onSubmit={handle} noValidate>
        {creationTemplates.length > 0 && <TemplateSelect templates={creationTemplates} onApply={apply} />}
        <Field label="Título" error={fe.title}>
          {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => setV({ ...v, title: e.target.value })} />}
        </Field>
        <Field label="Mensagem pública inicial" error={fe.message}>
          {(p) => <textarea {...p} rows={4} maxLength={5000} value={v.message} onChange={(e) => setV({ ...v, message: e.target.value })} />}
        </Field>
        <div className="form-grid">
          <Field label="Impacto" error={fe.impact}>
            {(p) => (
              <StatusSelect {...p} value={v.impact} options={IMPACTS} labels={impactLabel} onChange={(x) => setV({ ...v, impact: x as IncidentImpact })} />
            )}
          </Field>
          <Field label="Status inicial" error={fe.status}>
            {(p) => (
              <select {...p} value={v.status} onChange={(e) => setV({ ...v, status: e.target.value as typeof v.status })}>
                {OPEN_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {incidentStatusLabel[s]}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label="Início do impacto (opcional)" error={fe.started_at} hint={`Se começou antes de abrir aqui. Vazio = agora. Horário local (${browserTimeZone()}).`}>
            {(p) => <input {...p} type="datetime-local" value={v.started_at} onChange={(e) => setV({ ...v, started_at: e.target.value })} />}
          </Field>
        </div>
        <ComponentPicker permission="incidents.manage" selected={v.component_ids} onChange={(ids) => setV({ ...v, component_ids: ids })} error={fe.component_ids} />
        <Field label="Notas internas (não publicadas)" error={fe.internal_notes}>
          {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.internal_notes} onChange={(e) => setV({ ...v, internal_notes: e.target.value })} />}
        </Field>
        <FormStatus error={submit.formError} success={submit.success} />
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={submit.busy}>
            {submit.busy ? "Publicando…" : "Publicar incidente"}
          </button>
          <Link className="secondary-button" to="/admin/incidents">
            Cancelar
          </Link>
        </div>
      </form>
    </div>
  );
}

function MetadataForm({ incident, onSaved }: { incident: Incident; onSaved: (i: Incident) => void }) {
  const initialStart = toLocalInput(incident.started_at);
  const initialEnd = toLocalInput(incident.resolved_at);
  const [v, setV] = useState({
    title: incident.title,
    impact: incident.impact,
    internal_notes: incident.internal_notes,
    started_at: initialStart,
    resolved_at: initialEnd,
  });
  const submit = useSubmit();
  const resolved = incident.status === "resolved";
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "title", v.title, 200, "Título");
    const start = v.started_at !== initialStart ? localInputToISO(v.started_at) : undefined;
    const end = resolved && v.resolved_at !== initialEnd ? localInputToISO(v.resolved_at) : undefined;
    if (v.started_at !== initialStart && !start) errors.started_at = "Informe data e hora do início.";
    if (resolved && v.resolved_at !== initialEnd && !end) errors.resolved_at = "Informe data e hora do fim.";
    const startMs = Date.parse(start ?? incident.started_at);
    const endMs = resolved ? Date.parse(end ?? incident.resolved_at!) : NaN;
    if (!errors.started_at && start && startMs > Date.now()) errors.started_at = "O início não pode estar no futuro.";
    if (!errors.resolved_at && end && endMs > Date.now()) errors.resolved_at = "O fim não pode estar no futuro.";
    if (!errors.started_at && !errors.resolved_at && resolved && endMs < startMs) errors.resolved_at = "O fim não pode ser anterior ao início.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const saved = await submit.run(
      () =>
        adminApi.incidents.patch(incident.id, {
          title: v.title.trim(),
          impact: v.impact,
          internal_notes: v.internal_notes.trim(),
          ...(start ? { started_at: start } : {}),
          ...(end ? { resolved_at: end } : {}),
        }),
      "Dados atualizados (sem publicar atualização).",
    );
    if (saved) onSaved(saved);
  }
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Editar dados do incidente">
      <div className="form-grid">
        <Field label="Título" error={submit.fieldErrors.title}>
          {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => setV({ ...v, title: e.target.value })} />}
        </Field>
        <Field label="Impacto" error={submit.fieldErrors.impact}>
          {(p) => (
            <StatusSelect {...p} value={v.impact} options={IMPACTS} labels={impactLabel} onChange={(x) => setV({ ...v, impact: x as IncidentImpact })} />
          )}
        </Field>
      </div>
      <fieldset className="field period-field">
        <legend>Período real do impacto</legend>
        <div className="form-grid form-grid-2">
          <Field label="Início do impacto" error={submit.fieldErrors.started_at} hint={`Horário local (${browserTimeZone()}).`}>
            {(p) => <input {...p} type="datetime-local" value={v.started_at} onChange={(e) => setV({ ...v, started_at: e.target.value })} />}
          </Field>
          {resolved ? (
            <Field label="Fim do impacto" error={submit.fieldErrors.resolved_at} hint="Quando o serviço voltou ao normal.">
              {(p) => <input {...p} type="datetime-local" value={v.resolved_at} onChange={(e) => setV({ ...v, resolved_at: e.target.value })} />}
            </Field>
          ) : (
            <p className="field-hint period-note">O fim é informado ao resolver o incidente.</p>
          )}
        </div>
        <small className="field-hint">
          Pode ser anterior à abertura ou ao encerramento aqui. A página pública mostra este período; a linha do tempo mantém o horário de
          cada publicação.
        </small>
      </fieldset>
      <Field label="Notas internas" error={submit.fieldErrors.internal_notes}>
        {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.internal_notes} onChange={(e) => setV({ ...v, internal_notes: e.target.value })} />}
      </Field>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="secondary-button" disabled={submit.busy}>
          Salvar dados
        </button>
      </div>
    </form>
  );
}

function PublishUpdateForm({ incident, onSaved }: { incident: Incident; onSaved: (i: Incident) => void }) {
  const templates = useActiveIncidentTemplates();
  const [status, setStatus] = useState<IncidentStatus>(incident.status);
  const [message, setMessage] = useState("");
  const [changeComponents, setChangeComponents] = useState(false);
  const [componentIds, setComponentIds] = useState(incident.components.map((c) => c.id));
  const [endLocal, setEndLocal] = useState("");
  const submit = useSubmit();

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "message", message, 5000, "Mensagem");
    const end = status === "resolved" ? localInputToISO(endLocal) : undefined;
    if (end && Date.parse(end) < Date.parse(incident.started_at)) errors.resolved_at = "O fim não pode ser anterior ao início.";
    if (end && Date.parse(end) > Date.now()) errors.resolved_at = "O fim não pode estar no futuro.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const saved = await submit.run(
      () =>
        adminApi.incidents.publishUpdate(incident.id, {
          status,
          message: message.trim(),
          component_ids: changeComponents ? componentIds : undefined,
          ...(end ? { resolved_at: end } : {}),
        }),
      "Atualização publicada.",
    );
    if (saved) {
      setMessage("");
      setChangeComponents(false);
      onSaved(saved);
    }
  }

  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Publicar atualização">
      {templates.state.status === "success" && templates.state.data.data.length > 0 && (
        <TemplateSelect
          templates={templates.state.data.data}
          onApply={(t) => {
            setMessage(t.message);
            setStatus(t.status);
          }}
        />
      )}
      <Field label="Novo status" error={submit.fieldErrors.status} hint="Selecione Resolvido para encerrar o incidente.">
        {(p) => (
          <select {...p} value={status} onChange={(e) => setStatus(e.target.value as IncidentStatus)}>
                {ALL_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {incidentStatusLabel[s]}
                  </option>
                ))}
              </select>
        )}
      </Field>
      <Field label="Mensagem pública" error={submit.fieldErrors.message}>
        {(p) => <textarea {...p} rows={3} maxLength={5000} value={message} onChange={(e) => setMessage(e.target.value)} />}
      </Field>
      {status === "resolved" && (
        <div className="form-grid form-grid-2">
          <Field label="Fim do impacto (opcional)" error={submit.fieldErrors.resolved_at} hint="Vazio = agora.">
            {(p) => <input {...p} type="datetime-local" value={endLocal} onChange={(e) => setEndLocal(e.target.value)} />}
          </Field>
        </div>
      )}
      <label className="checkbox">
        <input type="checkbox" checked={changeComponents} onChange={(e) => setChangeComponents(e.target.checked)} />
        Alterar componentes afetados
      </label>
      {changeComponents && (
        <ComponentPicker permission="incidents.manage" selected={componentIds} onChange={setComponentIds} keep={incident.components} error={submit.fieldErrors.component_ids} />
      )}
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Publicando…" : status === "resolved" ? "Publicar e resolver" : "Publicar atualização"}
        </button>
      </div>
    </form>
  );
}

export function IncidentDetailPage({ id }: { id: string }) {
  const { state, reload, setData } = useApi((signal) => adminApi.incidents.get(id, { signal }), [id]);
  const [resolveMessage, setResolveMessage] = useState("O problema foi resolvido e o serviço está operando normalmente.");
  const [resolveError, setResolveError] = useState<string | null>(null);
  const [resolveEnd, setResolveEnd] = useState("");
  const canManage = useCanForComponents("incidents.manage", state.status === "success" ? state.data.components.map((c) => c.id) : []);

  if (state.status === "loading") return <div className="admin-content"><Loading /></div>;
  if (state.status === "error") return <div className="admin-content"><ErrorPanel error={state.error} onRetry={reload} /></div>;
  const inc = state.data;
  const resolved = inc.status === "resolved";

  return (
    <div className="admin-content">
      <p>
        <Link to="/admin/incidents">← Incidentes</Link>
      </p>
      <PageHeader title={inc.title}>
        <Badge tone={toneForIncident[inc.status]}>{incidentStatusLabel[inc.status]}</Badge>
      </PageHeader>
      <dl className="meta">
        <div>
          <dt>Início do impacto</dt>
          <dd>{formatDateTime(inc.started_at)}</dd>
        </div>
        {inc.resolved_at && (
          <div>
            <dt>Fim do impacto</dt>
            <dd>{formatDateTime(inc.resolved_at)}</dd>
          </div>
        )}
        <div>
          <dt>{inc.resolved_at ? "Duração" : "Em andamento há"}</dt>
          <dd>{formatDuration(inc.started_at, inc.resolved_at ?? new Date().toISOString())}</dd>
        </div>
        <div>
          <dt>Aberto aqui em</dt>
          <dd>{formatDateTime(inc.created_at)}</dd>
        </div>
        <div>
          <dt>Componentes</dt>
          <dd>{inc.components.length ? inc.components.map((c) => c.name).join(", ") : "Nenhum"}</dd>
        </div>
      </dl>

      {!canManage && <ReadOnlyNote what="publicar atualizações ou alterar incidentes" />}

      {canManage && !resolved && (
        <section className="panel" aria-labelledby="publish-title">
          <h2 id="publish-title">Publicar atualização</h2>
          <PublishUpdateForm key={inc.updated_at} incident={inc} onSaved={setData} />
        </section>
      )}

      {canManage && !resolved && (
        <section className="panel" aria-labelledby="resolve-title">
          <h2 id="resolve-title">Encerrar incidente</h2>
          <Field label="Mensagem de encerramento">
            {(p) => <textarea {...p} rows={2} maxLength={5000} value={resolveMessage} onChange={(e) => setResolveMessage(e.target.value)} />}
          </Field>
          <div className="form-grid form-grid-2">
            <Field
              label="Fim do impacto (opcional)"
              hint={`Se o problema terminou antes de encerrar aqui. Vazio = agora. Horário local (${browserTimeZone()}).`}
            >
              {(p) => <input {...p} type="datetime-local" value={resolveEnd} onChange={(e) => setResolveEnd(e.target.value)} />}
            </Field>
          </div>
          {resolveError && (
            <p className="field-error" role="alert">
              {resolveError}
            </p>
          )}
          <ConfirmButton
            variant="danger"
            label="Resolver incidente"
            confirmLabel="Confirmar resolução"
            question="Resolver este incidente? Ele sairá da lista de ativos e não aceitará novas atualizações."
            disabled={!resolveMessage.trim()}
            onConfirm={async () => {
              setResolveError(null);
              const end = localInputToISO(resolveEnd);
              if (end && Date.parse(end) < Date.parse(inc.started_at)) {
                setResolveError("O fim do impacto não pode ser anterior ao início.");
                return;
              }
              if (end && Date.parse(end) > Date.now()) {
                setResolveError("O fim do impacto não pode estar no futuro.");
                return;
              }
              try {
                setData(await adminApi.incidents.resolve(inc.id, resolveMessage.trim(), end));
              } catch (error) {
                setResolveError(error instanceof Error ? error.message : "Falha ao resolver.");
              }
            }}
          />
        </section>
      )}

      {resolved && <p className="muted">Incidente resolvido. A linha do tempo é preservada e não aceita novas atualizações.</p>}

      {canManage && (
        <section className="panel" aria-labelledby="meta-title">
          <h2 id="meta-title">Dados do incidente</h2>
          <MetadataForm key={inc.updated_at} incident={inc} onSaved={setData} />
        </section>
      )}

      <section aria-labelledby="timeline-title">
        <h2 id="timeline-title">Linha do tempo</h2>
        <Timeline
          formatAt={(iso) => formatDateTime(iso)}
          entries={(inc.updates ?? []).map((u) => ({
            id: u.id,
            label: incidentStatusLabel[u.status],
            tone: toneForIncident[u.status],
            message: u.message,
            at: u.created_at,
            extra: u.components ? (
              <small className="muted">
                Componentes: {u.components.length ? u.components.map((c) => c.name).join(", ") : "nenhum"}
              </small>
            ) : undefined,
          }))}
        />
      </section>
    </div>
  );
}
