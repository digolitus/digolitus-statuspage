import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { AnnouncementCategory, AnnouncementImportance, AnnouncementTemplate, AnnouncementTemplateCreate } from "../api/types";
import { MarkdownEditor } from "../components/MarkdownEditor";
import { RichText } from "../components/RichText";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, useSubmit } from "./shared";

interface Values {
  name: string;
  description: string;
  title: string;
  body: string;
  importance: AnnouncementImportance;
  category_id: string;
  guidance: string;
}

const blank: Values = { name: "", description: "", title: "", body: "", importance: "info", category_id: "", guidance: "" };

const valuesOf = (t: AnnouncementTemplate): Values => ({
  name: t.name,
  description: t.description,
  title: t.title,
  body: t.body,
  importance: t.importance,
  category_id: t.category_id ?? "",
  guidance: t.guidance,
});

function TemplateForm({
  initial,
  categories,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: Values;
  categories: AnnouncementCategory[];
  submitLabel: string;
  onSubmit: (body: AnnouncementTemplateCreate) => Promise<unknown>;
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
    requireText(errors, "body", v.body, 10000, "Texto sugerido");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const ok = await submit.run(
      () =>
        onSubmit({
          name: v.name.trim(),
          description: v.description.trim(),
          title: v.title.trim(),
          body: v.body.trim(),
          importance: v.importance,
          category_id: v.category_id || null,
          guidance: v.guidance.trim(),
        }),
      "Template salvo.",
    );
    if (ok && initial === blank) setV(blank);
  }

  const fe = submit.fieldErrors;
  const options = categories.filter((c) => c.is_active || c.id === initial.category_id);
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={initial === blank ? "Novo template de comunicado" : `Editar ${initial.name}`}>
      <div className="form-grid form-grid-3">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} placeholder="Ex.: Nova funcionalidade" onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field label="Categoria sugerida" error={fe.category_id}>
          {(p) => (
            <select {...p} value={v.category_id} onChange={(e) => set("category_id", e.target.value)}>
              <option value="">Sem categoria</option>
              {options.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Destaque" error={fe.importance}>
          {(p) => (
            <select {...p} value={v.importance} onChange={(e) => set("importance", e.target.value as AnnouncementImportance)}>
              <option value="info">Informativo</option>
              <option value="important">Importante</option>
            </select>
          )}
        </Field>
      </div>
      <Field label="Descrição" error={fe.description} hint="Quando usar este template.">
        {(p) => <input {...p} value={v.description} maxLength={1000} onChange={(e) => set("description", e.target.value)} />}
      </Field>
      <Field label="Título sugerido" error={fe.title} hint="Use [colchetes] para o que deve ser trocado.">
        {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => set("title", e.target.value)} />}
      </Field>
      <div className={`field${fe.body ? " field-invalid" : ""}`}>
        <MarkdownEditor id={initial === blank ? "tpl-body-new" : `tpl-body-${initial.name.replace(/\W+/g, "-")}`} label="Texto sugerido" value={v.body} onChange={(b) => set("body", b)} invalid={!!fe.body} rows={8} />
        {fe.body && <small className="field-error">{fe.body}</small>}
      </div>
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

export function AnnouncementTemplatesPage() {
  const [filter, setFilter] = useState<"active" | "inactive" | "all">("active");
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [previewing, setPreviewing] = useState<string | null>(null);
  const isActive = filter === "all" ? undefined : filter === "active";
  const { state, reload } = useApi(
    (signal) => adminApi.announcementTemplates.list({ is_active: isActive, limit: 50, offset }, { signal }),
    [filter, offset],
  );
  const categoriesState = useApi((signal) => adminApi.announcementCategories.list({}, { signal }), []).state;
  const categories = categoriesState.status === "success" ? categoriesState.data.data : [];
  const categoryName = (id: string | null) => categories.find((c) => c.id === id)?.name;
  const canManage = useAuth().can("templates.manage");

  return (
    <div className="admin-content">
      <PageHeader title="Templates de comunicado">
        {canManage && (
          <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
            Novo template
          </button>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what="criar ou editar templates" />}
      <p className="muted">
        Templates preenchem o editor de comunicados com título, texto, destaque e categoria para revisão. Editar ou desativar um
        template não altera comunicados já publicados.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo template">
          <TemplateForm
            initial={blank}
            categories={categories}
            submitLabel="Criar template"
            onSubmit={async (body) => {
              const t = await adminApi.announcementTemplates.create(body);
              reload();
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
                <li key={`${t.id}-${t.updated_at}`} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{t.name}</strong>
                      {t.description && <p className="muted small">{t.description}</p>}
                      <div className="admin-item-tags">
                        <span className={t.importance === "important" ? "tag tag-warn" : "tag"}>{t.importance === "important" ? "Importante" : "Informativo"}</span>
                        {t.category_id && <span className="tag">Categoria: {categoryName(t.category_id) ?? "…"}</span>}
                        {!t.is_active && <span className="tag">Desativado</span>}
                      </div>
                    </div>
                  </div>
                  <div className="form-actions">
                    <button type="button" className="secondary-button" aria-expanded={previewing === t.id} onClick={() => setPreviewing(previewing === t.id ? null : t.id)}>
                      Ver texto
                    </button>
                    {canManage && (
                      <>
                        <button type="button" className="secondary-button" aria-expanded={editing === t.id} onClick={() => setEditing(editing === t.id ? null : t.id)}>
                          Editar
                        </button>
                        <ConfirmButton
                          variant={t.is_active ? "danger" : "secondary"}
                          label={t.is_active ? "Desativar" : "Ativar"}
                          confirmLabel="Confirmar"
                          question={t.is_active ? `Desativar "${t.name}"? Ele deixará de ser oferecido no editor.` : `Ativar "${t.name}"?`}
                          onConfirm={async () => {
                            await adminApi.announcementTemplates.update(t.id, { is_active: !t.is_active });
                            reload();
                          }}
                        />
                      </>
                    )}
                  </div>
                  {previewing === t.id && (
                    <div className="template-preview">
                      <h3>{t.title}</h3>
                      <RichText text={t.body} />
                      {t.guidance && (
                        <p className="guidance">
                          <strong>Orientação:</strong> {t.guidance}
                        </p>
                      )}
                    </div>
                  )}
                  {editing === t.id && (
                    <TemplateForm
                      initial={valuesOf(t)}
                      categories={categories}
                      submitLabel="Salvar alterações"
                      onSubmit={async (body) => {
                        const saved = await adminApi.announcementTemplates.update(t.id, body);
                        setEditing(null);
                        reload();
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
