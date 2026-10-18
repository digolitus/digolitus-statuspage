import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { ExpectedImpact, IncidentImpact, IncidentStatus, IncidentTemplate, MaintenanceTemplate } from "../api/types";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { expectedImpactLabel, impactLabel, incidentStatusLabel } from "../lib/format";
import { useApi } from "../lib/useApi";
import { FormStatus, PageHeader, requireText, useSubmit } from "./shared";

type Kind = "incident" | "maintenance";
type AnyTemplate = IncidentTemplate | MaintenanceTemplate;

interface Values {
  name: string;
  description: string;
  title: string;
  message: string;
  guidance: string;
  impact: IncidentImpact;
  status: IncidentStatus;
  expected_impact: ExpectedImpact;
}

const blank: Values = {
  name: "",
  description: "",
  title: "",
  message: "",
  guidance: "",
  impact: "minor",
  status: "investigating",
  expected_impact: "none",
};

function valuesOf(t: AnyTemplate): Values {
  return {
    ...blank,
    name: t.name,
    description: t.description,
    title: t.title,
    message: t.message,
    guidance: t.guidance,
    ...("impact" in t ? { impact: t.impact, status: t.status } : { expected_impact: t.expected_impact }),
  };
}

const api = {
  incident: adminApi.incidentTemplates,
  maintenance: adminApi.maintenanceTemplates,
};

function TemplateForm({ kind, initial, onSubmit, submitLabel, onCancel }: {
  kind: Kind;
  initial: Values;
  onSubmit: (body: Record<string, unknown>) => Promise<unknown>;
  submitLabel: string;
  onCancel?: () => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const set = <K extends keyof Values>(k: K, value: Values[K]) => setV((s) => ({ ...s, [k]: value }));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    requireText(errors, "title", v.title, 200, "Título sugerido");
    requireText(errors, "message", v.message, 5000, "Mensagem sugerida");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body: Record<string, unknown> = {
      name: v.name.trim(),
      description: v.description.trim(),
      title: v.title.trim(),
      message: v.message.trim(),
      guidance: v.guidance.trim(),
      ...(kind === "incident" ? { impact: v.impact, status: v.status } : { expected_impact: v.expected_impact }),
    };
    const ok = await submit.run(() => onSubmit(body), "Template salvo.");
    if (ok && initial === blank) setV(blank);
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate>
      <div className="form-grid">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => set("name", e.target.value)} />}
        </Field>
        {kind === "incident" ? (
          <>
            <Field label="Impacto padrão" error={fe.impact}>
              {(p) => (
                <select {...p} value={v.impact} onChange={(e) => set("impact", e.target.value as IncidentImpact)}>
                  {(Object.keys(impactLabel) as IncidentImpact[]).map((i) => (
                    <option key={i} value={i}>
                      {impactLabel[i]}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <Field label="Status sugerido" error={fe.status}>
              {(p) => (
                <select {...p} value={v.status} onChange={(e) => set("status", e.target.value as IncidentStatus)}>
                  {(Object.keys(incidentStatusLabel) as IncidentStatus[]).map((s) => (
                    <option key={s} value={s}>
                      {incidentStatusLabel[s]}
                    </option>
                  ))}
                </select>
              )}
            </Field>
          </>
        ) : (
          <Field label="Impacto esperado" error={fe.expected_impact}>
            {(p) => (
              <select {...p} value={v.expected_impact} onChange={(e) => set("expected_impact", e.target.value as ExpectedImpact)}>
                {(Object.keys(expectedImpactLabel) as ExpectedImpact[]).map((i) => (
                  <option key={i} value={i}>
                    {expectedImpactLabel[i]}
                  </option>
                ))}
              </select>
            )}
          </Field>
        )}
      </div>
      <Field label="Descrição" error={fe.description}>
        {(p) => <input {...p} value={v.description} maxLength={1000} onChange={(e) => set("description", e.target.value)} />}
      </Field>
      <Field label="Título sugerido" error={fe.title}>
        {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => set("title", e.target.value)} />}
      </Field>
      <Field label="Mensagem sugerida" error={fe.message}>
        {(p) => <textarea {...p} rows={3} maxLength={5000} value={v.message} onChange={(e) => set("message", e.target.value)} />}
      </Field>
      <Field label="Orientação de uso (não publicada)" error={fe.guidance}>
        {(p) => <textarea {...p} rows={2} maxLength={2000} value={v.guidance} onChange={(e) => set("guidance", e.target.value)} />}
      </Field>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Salvando…" : submitLabel}
        </button>
        {onCancel && (
          <button type="button" className="secondary-button" onClick={onCancel}>
            Fechar
          </button>
        )}
      </div>
    </form>
  );
}

export function TemplatesPage({ kind }: { kind: Kind }) {
  const [filter, setFilter] = useState<"active" | "inactive" | "all">("active");
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const isActive = filter === "all" ? undefined : filter === "active";
  const { state, reload } = useApi(
    (signal) => api[kind].list({ is_active: isActive, limit: 50, offset }, { signal }) as Promise<{ data: AnyTemplate[]; pagination: { limit: number; offset: number; total: number } }>,
    [kind, filter, offset],
  );
  const title = kind === "incident" ? "Templates de incidente" : "Templates de manutenção";

  return (
    <div className="admin-content">
      <PageHeader title={title}>
        <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
          Novo template
        </button>
      </PageHeader>
      <p className="muted">
        Templates apenas preenchem formulários para revisão. Editar ou desativar um template não altera eventos já publicados.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo template">
          <TemplateForm
            kind={kind}
            initial={blank}
            submitLabel="Criar template"
            onSubmit={async (body) => {
              const t = await api[kind].create(body);
              await reload();
              return t;
            }}
            onCancel={() => setCreating(false)}
          />
        </section>
      )}
      <div className="filters" role="group" aria-label="Filtrar templates">
        {(["active", "inactive", "all"] as const).map((f) => (
          <button
            key={f}
            type="button"
            aria-pressed={filter === f}
            className={filter === f ? "chip-button chip-button-active" : "chip-button"}
            onClick={() => {
              setFilter(f);
              setOffset(0);
            }}
          >
            {f === "active" ? "Ativos" : f === "inactive" ? "Desativados" : "Todos"}
          </button>
        ))}
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum template encontrado" />
        ) : (
          <>
            <ul className="admin-list">
              {state.data.data.map((t) => (
                <li key={t.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{t.name}</strong>
                      {t.description && <p className="muted">{t.description}</p>}
                      <div className="admin-item-tags">
                        {"impact" in t ? (
                          <>
                            <span className="tag">Impacto {impactLabel[t.impact].toLowerCase()}</span>
                            <span className="tag">{incidentStatusLabel[t.status]}</span>
                          </>
                        ) : (
                          <span className="tag">{expectedImpactLabel[t.expected_impact]}</span>
                        )}
                        {!t.is_active && <span className="tag">Desativado</span>}
                      </div>
                    </div>
                  </div>
                  <div className="form-actions">
                    <button type="button" className="secondary-button" aria-expanded={editing === t.id} onClick={() => setEditing(editing === t.id ? null : t.id)}>
                      Editar
                    </button>
                    <ConfirmButton
                      variant={t.is_active ? "danger" : "secondary"}
                      label={t.is_active ? "Desativar" : "Ativar"}
                      confirmLabel="Confirmar"
                      question={t.is_active ? `Desativar "${t.name}"? Ele deixará de ser oferecido nos formulários.` : `Ativar "${t.name}"?`}
                      onConfirm={async () => {
                        await api[kind].update(t.id, { is_active: !t.is_active });
                        await reload();
                      }}
                    />
                  </div>
                  {editing === t.id && (
                    <TemplateForm
                      kind={kind}
                      initial={valuesOf(t)}
                      submitLabel="Salvar alterações"
                      onSubmit={async (body) => {
                        const saved = await api[kind].update(t.id, body);
                        await reload();
                        return saved;
                      }}
                      onCancel={() => setEditing(null)}
                    />
                  )}
                </li>
              ))}
            </ul>
            <Pager pagination={state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}
