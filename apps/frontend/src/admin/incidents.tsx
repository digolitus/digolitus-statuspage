import { useState, type FormEvent } from "react";
import { adminApi, type IncidentCreateInput } from "../api/endpoints";
import type { Incident, IncidentImpact, IncidentStatus, IncidentTemplate } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager, Timeline } from "../components/ui";
import { browserTimeZone, formatDateTime, impactLabel, incidentStatusLabel, toneForIncident } from "../lib/format";
import { Link, navigate } from "../lib/router";
import { useApi } from "../lib/useApi";
import { ComponentPicker, FormStatus, PageHeader, requireText, TemplateSelect, useSubmit } from "./shared";

const IMPACTS = Object.keys(impactLabel) as IncidentImpact[];
const OPEN_STATUSES: Exclude<IncidentStatus, "resolved">[] = ["investigating", "identified", "monitoring"];
const ALL_STATUSES = Object.keys(incidentStatusLabel) as IncidentStatus[];

function useActiveIncidentTemplates() {
  return useApi((signal) => adminApi.incidentTemplates.list({ is_active: true, limit: 100 }, { signal }), []);
}

export function IncidentsPage() {
  const [state, setStateFilter] = useState<"active" | "resolved" | "">("active");
  const [offset, setOffset] = useState(0);
  const list = useApi((signal) => adminApi.incidents.list({ state: state || undefined, limit: 20, offset }, { signal }), [state, offset]);
  return (
    <div className="admin-content">
      <PageHeader title="Incidentes">
        <Link className="primary-button" to="/admin/incidents/new">
          Novo incidente
        </Link>
      </PageHeader>
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
    impact: "minor" as IncidentImpact,
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
    const input: IncidentCreateInput = {
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
              <select {...p} value={v.impact} onChange={(e) => setV({ ...v, impact: e.target.value as IncidentImpact })}>
                {IMPACTS.map((i) => (
                  <option key={i} value={i}>
                    {impactLabel[i]}
                  </option>
                ))}
              </select>
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
          <Field label="Início (opcional)" error={fe.started_at} hint={`Horário local de ${browserTimeZone()}. Vazio = agora.`}>
            {(p) => <input {...p} type="datetime-local" value={v.started_at} onChange={(e) => setV({ ...v, started_at: e.target.value })} />}
          </Field>
        </div>
        <ComponentPicker selected={v.component_ids} onChange={(ids) => setV({ ...v, component_ids: ids })} error={fe.component_ids} />
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
  const [v, setV] = useState({ title: incident.title, impact: incident.impact, internal_notes: incident.internal_notes });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "title", v.title, 200, "Título");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const saved = await submit.run(
      () => adminApi.incidents.patch(incident.id, { title: v.title.trim(), impact: v.impact, internal_notes: v.internal_notes.trim() }),
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
            <select {...p} value={v.impact} onChange={(e) => setV({ ...v, impact: e.target.value as IncidentImpact })}>
              {IMPACTS.map((i) => (
                <option key={i} value={i}>
                  {impactLabel[i]}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>
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
  const submit = useSubmit();

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "message", message, 5000, "Mensagem");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const saved = await submit.run(
      () =>
        adminApi.incidents.publishUpdate(incident.id, {
          status,
          message: message.trim(),
          component_ids: changeComponents ? componentIds : undefined,
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
      <label className="checkbox">
        <input type="checkbox" checked={changeComponents} onChange={(e) => setChangeComponents(e.target.checked)} />
        Alterar componentes afetados
      </label>
      {changeComponents && (
        <ComponentPicker selected={componentIds} onChange={setComponentIds} keep={incident.components} error={submit.fieldErrors.component_ids} />
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
          <dt>Início</dt>
          <dd>{formatDateTime(inc.started_at)}</dd>
        </div>
        {inc.resolved_at && (
          <div>
            <dt>Resolução</dt>
            <dd>{formatDateTime(inc.resolved_at)}</dd>
          </div>
        )}
        <div>
          <dt>Componentes</dt>
          <dd>{inc.components.length ? inc.components.map((c) => c.name).join(", ") : "Nenhum"}</dd>
        </div>
      </dl>

      {!resolved && (
        <section className="panel" aria-labelledby="publish-title">
          <h2 id="publish-title">Publicar atualização</h2>
          <PublishUpdateForm key={inc.updated_at} incident={inc} onSaved={setData} />
        </section>
      )}

      {!resolved && (
        <section className="panel" aria-labelledby="resolve-title">
          <h2 id="resolve-title">Encerrar incidente</h2>
          <Field label="Mensagem de encerramento">
            {(p) => <textarea {...p} rows={2} maxLength={5000} value={resolveMessage} onChange={(e) => setResolveMessage(e.target.value)} />}
          </Field>
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
              try {
                setData(await adminApi.incidents.resolve(inc.id, resolveMessage.trim()));
              } catch (error) {
                setResolveError(error instanceof Error ? error.message : "Falha ao resolver.");
              }
            }}
          />
        </section>
      )}

      {resolved && <p className="muted">Incidente resolvido. A linha do tempo é preservada e não aceita novas atualizações.</p>}

      <section className="panel" aria-labelledby="meta-title">
        <h2 id="meta-title">Dados do incidente</h2>
        <MetadataForm key={inc.updated_at} incident={inc} onSaved={setData} />
      </section>

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
