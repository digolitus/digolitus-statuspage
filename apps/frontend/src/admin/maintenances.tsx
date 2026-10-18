import { useMemo, useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { ExpectedImpact, Maintenance, MaintenanceCreate, MaintenancePatch, MaintenanceStatus, MaintenanceTemplate } from "../api/types";
import { StatusSelect } from "../components/StatusSelect";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager, Timeline } from "../components/ui";
import {
  browserTimeZone,
  expectedImpactLabel,
  formatDateTime,
  IMPACTS,
  maintenanceKindLabel,
  maintenanceStatusLabel,
  timeZones,
  toneForMaintenance,
  toZonedInput,
} from "../lib/format";
import { Link, navigate } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { ComponentPicker, FormStatus, PageHeader, ReadOnlyNote, requireText, TemplateSelect, useCanForComponents, useSubmit } from "./shared";


const STATES = [
  ["open", "Abertas"],
  ["upcoming", "Programadas"],
  ["in_progress", "Em andamento"],
  ["past", "Encerradas"],
  ["", "Todas"],
] as const;

export function MaintenancesPage() {
  const [state, setStateFilter] = useState<string>("open");
  const [offset, setOffset] = useState(0);
  const list = useApi((signal) => adminApi.maintenances.list({ state: state || undefined, limit: 20, offset }, { signal }), [state, offset]);
  const canManage = useAuth().can("maintenances.manage");
  return (
    <div className="admin-content">
      <PageHeader title="Manutenções programadas">
        {canManage && (
          <Link className="primary-button" to="/admin/maintenances/new">
            Programar manutenção
          </Link>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what="programar ou alterar manutenções" />}
      <div className="filters" role="group" aria-label="Filtrar manutenções">
        {STATES.map(([value, label]) => (
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
          <EmptyState title="Nenhuma manutenção encontrada" />
        ) : (
          <>
            <ul className="admin-list">
              {list.state.data.data.map((m) => (
                <li key={m.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <Link to={`/admin/maintenances/${m.id}`}>
                        <strong>{m.title}</strong>
                      </Link>
                      <div className="admin-item-tags">
                        <span className="tag">
                          {formatDateTime(m.scheduled_start, m.timezone)} → {formatDateTime(m.scheduled_end, m.timezone)}
                        </span>
                        <span className="tag">{expectedImpactLabel[m.expected_impact]}</span>
                        {m.start_overdue && <span className="tag tag-warn">Início previsto já passou</span>}
                        {m.end_overdue && <span className="tag tag-warn">Término previsto já passou</span>}
                      </div>
                    </div>
                    <Badge tone={toneForMaintenance[m.status]}>{maintenanceStatusLabel[m.status]}</Badge>
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

interface FormValues {
  title: string;
  description: string;
  message: string;
  expected_impact: ExpectedImpact;
  timezone: string;
  scheduled_start: string;
  scheduled_end: string;
  component_ids: string[];
  internal_notes: string;
  template_id: string;
}

function TimeZoneSelect({ value, onChange, id, ...aria }: { value: string; onChange: (v: string) => void; id: string }) {
  const zones = useMemo(() => {
    const all = timeZones();
    return all.includes(value) ? all : [value, ...all];
  }, [value]);
  return (
    <select id={id} {...aria} value={value} onChange={(e) => onChange(e.target.value)}>
      {zones.map((z) => (
        <option key={z} value={z}>
          {z}
        </option>
      ))}
    </select>
  );
}

function validateWindow(v: FormValues, errors: Record<string, string>) {
  requireText(errors, "title", v.title, 200, "Título");
  if (!v.scheduled_start) errors.scheduled_start = "Informe o início previsto.";
  if (!v.scheduled_end) errors.scheduled_end = "Informe o término previsto.";
  // Same zone for both values, so lexical order equals chronological order.
  if (v.scheduled_start && v.scheduled_end && v.scheduled_end <= v.scheduled_start) {
    errors.scheduled_end = "O término deve ser posterior ao início.";
  }
}

export function MaintenanceNewPage() {
  const templates = useApi((signal) => adminApi.maintenanceTemplates.list({ is_active: true, limit: 100 }, { signal }), []);
  const [v, setV] = useState<FormValues>({
    title: "",
    description: "",
    message: "",
    expected_impact: "operational",
    timezone: browserTimeZone(),
    scheduled_start: "",
    scheduled_end: "",
    component_ids: [],
    internal_notes: "",
    template_id: "",
  });
  const submit = useSubmit();
  const set = <K extends keyof FormValues>(k: K, value: FormValues[K]) => setV((s) => ({ ...s, [k]: value }));

  const apply = (t: MaintenanceTemplate) =>
    setV((s) => ({ ...s, title: t.title, message: t.message, expected_impact: t.expected_impact, template_id: t.id }));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    validateWindow(v, errors);
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const input: MaintenanceCreate = {
      title: v.title.trim(),
      description: v.description.trim(),
      expected_impact: v.expected_impact,
      timezone: v.timezone,
      // Local wall times; the server interprets them in `timezone`.
      scheduled_start: v.scheduled_start,
      scheduled_end: v.scheduled_end,
      component_ids: v.component_ids,
    };
    if (v.message.trim()) input.message = v.message.trim();
    if (v.internal_notes.trim()) input.internal_notes = v.internal_notes.trim();
    if (v.template_id) input.template_id = v.template_id;
    const created = await submit.run(() => adminApi.maintenances.create(input));
    if (created) navigate(`/admin/maintenances/${created.id}`);
  }

  const fe = submit.fieldErrors;
  return (
    <div className="admin-content">
      <PageHeader title="Programar manutenção" />
      <form className="form panel" onSubmit={handle} noValidate>
        {templates.state.status === "success" && templates.state.data.data.length > 0 && (
          <TemplateSelect templates={templates.state.data.data} onApply={apply} />
        )}
        <Field label="Título" error={fe.title}>
          {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => set("title", e.target.value)} />}
        </Field>
        <Field label="Descrição" error={fe.description}>
          {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.description} onChange={(e) => set("description", e.target.value)} />}
        </Field>
        <Field label="Mensagem pública inicial (opcional)" error={fe.message}>
          {(p) => <textarea {...p} rows={3} maxLength={5000} value={v.message} onChange={(e) => set("message", e.target.value)} />}
        </Field>
        <div className="form-grid">
          <Field label="Timezone da janela" error={fe.timezone} hint="Os horários abaixo são interpretados neste timezone.">
            {(p) => <TimeZoneSelect {...p} value={v.timezone} onChange={(z) => set("timezone", z)} />}
          </Field>
          <Field label="Início previsto" error={fe.scheduled_start}>
            {(p) => <input {...p} type="datetime-local" value={v.scheduled_start} onChange={(e) => set("scheduled_start", e.target.value)} />}
          </Field>
          <Field label="Término previsto" error={fe.scheduled_end}>
            {(p) => <input {...p} type="datetime-local" value={v.scheduled_end} onChange={(e) => set("scheduled_end", e.target.value)} />}
          </Field>
          <Field label="Impacto esperado" error={fe.expected_impact}>
            {(p) => (
              <StatusSelect {...p} value={v.expected_impact} options={IMPACTS} labels={expectedImpactLabel} onChange={(x) => set("expected_impact", x as ExpectedImpact)} />
            )}
          </Field>
        </div>
        <ComponentPicker permission="maintenances.manage" selected={v.component_ids} onChange={(ids) => set("component_ids", ids)} error={fe.component_ids} />
        <Field label="Notas internas (não publicadas)" error={fe.internal_notes}>
          {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.internal_notes} onChange={(e) => set("internal_notes", e.target.value)} />}
        </Field>
        <FormStatus error={submit.formError} success={submit.success} />
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={submit.busy}>
            {submit.busy ? "Programando…" : "Programar manutenção"}
          </button>
          <Link className="secondary-button" to="/admin/maintenances">
            Cancelar
          </Link>
        </div>
      </form>
    </div>
  );
}

function EditForm({ m, onSaved }: { m: Maintenance; onSaved: (m: Maintenance) => void }) {
  const initial = {
    title: m.title,
    description: m.description,
    expected_impact: m.expected_impact,
    timezone: m.timezone,
    scheduled_start: toZonedInput(m.scheduled_start, m.timezone),
    scheduled_end: toZonedInput(m.scheduled_end, m.timezone),
    component_ids: m.components.map((c) => c.id),
    internal_notes: m.internal_notes,
  };
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const started = m.status === "in_progress";

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    validateWindow({ ...v, message: "", template_id: "" }, errors);
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const patch: MaintenancePatch = {};
    if (v.title.trim() !== initial.title) patch.title = v.title.trim();
    if (v.description.trim() !== initial.description) patch.description = v.description.trim();
    if (v.expected_impact !== initial.expected_impact) patch.expected_impact = v.expected_impact;
    if (v.internal_notes.trim() !== initial.internal_notes) patch.internal_notes = v.internal_notes.trim();
    const zoneChanged = v.timezone !== initial.timezone;
    if (zoneChanged) patch.timezone = v.timezone;
    // When the zone changes, resend the wall times so they are reinterpreted
    // in the new zone, exactly as the operator sees them in the form.
    if (!started && (zoneChanged || v.scheduled_start !== initial.scheduled_start)) patch.scheduled_start = v.scheduled_start;
    if (zoneChanged || v.scheduled_end !== initial.scheduled_end) patch.scheduled_end = v.scheduled_end;
    if (v.component_ids.join() !== initial.component_ids.join()) patch.component_ids = v.component_ids;
    if (Object.keys(patch).length === 0) {
      submit.setSuccess("Nenhuma alteração para salvar.");
      return;
    }
    const saved = await submit.run(() => adminApi.maintenances.patch(m.id, patch), "Manutenção atualizada.");
    if (saved) onSaved(saved);
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Editar manutenção">
      <Field label="Título" error={fe.title}>
        {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => setV({ ...v, title: e.target.value })} />}
      </Field>
      <Field label="Descrição" error={fe.description}>
        {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} />}
      </Field>
      <div className="form-grid">
        <Field label="Timezone da janela" error={fe.timezone}>
          {(p) => <TimeZoneSelect {...p} value={v.timezone} onChange={(z) => setV({ ...v, timezone: z })} />}
        </Field>
        <Field label="Início previsto" error={fe.scheduled_start} hint={started ? "Não pode ser alterado após o início." : undefined}>
          {(p) => (
            <input {...p} type="datetime-local" disabled={started} value={v.scheduled_start} onChange={(e) => setV({ ...v, scheduled_start: e.target.value })} />
          )}
        </Field>
        <Field label="Término previsto" error={fe.scheduled_end}>
          {(p) => <input {...p} type="datetime-local" value={v.scheduled_end} onChange={(e) => setV({ ...v, scheduled_end: e.target.value })} />}
        </Field>
        <Field label="Impacto esperado" error={fe.expected_impact}>
          {(p) => (
            <StatusSelect {...p} value={v.expected_impact} options={IMPACTS} labels={expectedImpactLabel} onChange={(x) => setV({ ...v, expected_impact: x as ExpectedImpact })} />
          )}
        </Field>
      </div>
      <ComponentPicker permission="maintenances.manage" selected={v.component_ids} onChange={(ids) => setV({ ...v, component_ids: ids })} keep={m.components} error={fe.component_ids} />
      <Field label="Notas internas" error={fe.internal_notes}>
        {(p) => <textarea {...p} rows={2} maxLength={5000} value={v.internal_notes} onChange={(e) => setV({ ...v, internal_notes: e.target.value })} />}
      </Field>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="secondary-button" disabled={submit.busy}>
          Salvar alterações
        </button>
      </div>
    </form>
  );
}

const ACTIONS: Record<MaintenanceStatus, { action: "start" | "complete" | "cancel"; label: string; question: string; variant: "primary" | "danger" }[]> = {
  scheduled: [
    { action: "start", label: "Iniciar manutenção", question: "Iniciar agora? A página pública passará a mostrá-la em andamento.", variant: "primary" },
    { action: "cancel", label: "Cancelar manutenção", question: "Cancelar esta manutenção? Ela não poderá ser retomada.", variant: "danger" },
  ],
  in_progress: [{ action: "complete", label: "Concluir manutenção", question: "Concluir esta manutenção?", variant: "primary" }],
  completed: [],
  cancelled: [],
};

export function MaintenanceDetailPage({ id }: { id: string }) {
  const { state, reload, setData } = useApi((signal) => adminApi.maintenances.get(id, { signal }), [id]);
  const [actionMessage, setActionMessage] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const msgSubmit = useSubmit();
  const canManage = useCanForComponents("maintenances.manage", state.status === "success" ? state.data.components.map((c) => c.id) : []);

  if (state.status === "loading") return <div className="admin-content"><Loading /></div>;
  if (state.status === "error") return <div className="admin-content"><ErrorPanel error={state.error} onRetry={reload} /></div>;
  const m = state.data;
  const open = m.status === "scheduled" || m.status === "in_progress";
  // Lifecycle actions and edits require the permission; reading does not.
  const editable = open && canManage;

  return (
    <div className="admin-content">
      <p>
        <Link to="/admin/maintenances">← Manutenções</Link>
      </p>
      <PageHeader title={m.title}>
        <Badge tone={toneForMaintenance[m.status]}>{maintenanceStatusLabel[m.status]}</Badge>
      </PageHeader>
      <dl className="meta">
        <div>
          <dt>Janela prevista ({m.timezone})</dt>
          <dd>
            {formatDateTime(m.scheduled_start, m.timezone)} → {formatDateTime(m.scheduled_end, m.timezone)}
          </dd>
        </div>
        <div>
          <dt>No seu horário</dt>
          <dd>
            {formatDateTime(m.scheduled_start)} → {formatDateTime(m.scheduled_end)}
          </dd>
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
      {m.start_overdue && <p className="overdue">O início previsto já passou. Inicie ou cancele a manutenção; nada muda automaticamente.</p>}
      {m.end_overdue && <p className="overdue">O término previsto já passou. Conclua a manutenção ou ajuste o término previsto.</p>}

      {open && !canManage && <ReadOnlyNote what="iniciar, concluir, cancelar ou editar manutenções" />}

      {editable && (
        <section className="panel" aria-labelledby="actions-title">
          <h2 id="actions-title">Ciclo de vida</h2>
          <Field label="Mensagem pública da ação (opcional)">
            {(p) => <textarea {...p} rows={2} maxLength={5000} value={actionMessage} onChange={(e) => setActionMessage(e.target.value)} />}
          </Field>
          {actionError && (
            <p className="field-error" role="alert">
              {actionError}
            </p>
          )}
          <div className="form-actions">
            {ACTIONS[m.status].map((a) => (
              <ConfirmButton
                key={a.action}
                variant={a.variant}
                label={a.label}
                confirmLabel="Confirmar"
                question={a.question}
                onConfirm={async () => {
                  setActionError(null);
                  try {
                    setData(await adminApi.maintenances.transition(m.id, a.action, actionMessage.trim() || undefined));
                    setActionMessage("");
                  } catch (error) {
                    setActionError(error instanceof Error ? error.message : "Falha ao executar a ação.");
                  }
                }}
              />
            ))}
          </div>
        </section>
      )}

      {editable && (
        <section className="panel" aria-labelledby="message-title">
          <h2 id="message-title">Publicar mensagem</h2>
          <form
            className="form"
            noValidate
            onSubmit={async (e) => {
              e.preventDefault();
              if (!message.trim()) return msgSubmit.setFieldErrors({ message: "Mensagem é obrigatória." });
              const saved = await msgSubmit.run(() => adminApi.maintenances.postMessage(m.id, message.trim()), "Mensagem publicada.");
              if (saved) {
                setMessage("");
                setData(saved);
              }
            }}
          >
            <Field label="Mensagem pública" error={msgSubmit.fieldErrors.message}>
              {(p) => <textarea {...p} rows={3} maxLength={5000} value={message} onChange={(e) => setMessage(e.target.value)} />}
            </Field>
            <FormStatus error={msgSubmit.formError} success={msgSubmit.success} />
            <div className="form-actions">
              <button type="submit" className="primary-button" disabled={msgSubmit.busy}>
                Publicar mensagem
              </button>
            </div>
          </form>
        </section>
      )}

      {editable ? (
        <section className="panel" aria-labelledby="edit-title">
          <h2 id="edit-title">Editar ou reprogramar</h2>
          <p className="muted">Reprogramações ficam no histórico operacional. Para explicá-las publicamente, publique uma mensagem.</p>
          <EditForm key={m.updated_at} m={m} onSaved={setData} />
        </section>
      ) : (
        !open && <p className="muted">Manutenção encerrada; o histórico é preservado e não aceita alterações.</p>
      )}

      <section aria-labelledby="timeline-title">
        <h2 id="timeline-title">Histórico de atualizações</h2>
        <Timeline
          formatAt={(iso) => formatDateTime(iso)}
          entries={(m.updates ?? []).map((u) => ({
            id: u.id,
            label: maintenanceKindLabel[u.kind],
            tone: toneForMaintenance[u.status],
            message: u.message,
            at: u.created_at,
          }))}
        />
      </section>
    </div>
  );
}
