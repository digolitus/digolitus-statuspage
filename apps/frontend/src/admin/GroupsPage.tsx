import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { ComponentGroup, ComponentGroupCreate } from "../api/types";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { plural } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { FormStatus, PageHeader, requireText, useSubmit } from "./shared";

type Values = { name: string; description: string; display_order: string };
const blank: Values = { name: "", description: "", display_order: "0" };

function GroupForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: Values;
  submitLabel: string;
  onSubmit: (body: ComponentGroupCreate) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    const order = Number(v.display_order);
    if (!Number.isInteger(order) || order < 0 || order > 100000) errors.display_order = "Informe um inteiro entre 0 e 100000.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const ok = await submit.run(
      () => onSubmit({ name: v.name.trim(), description: v.description.trim(), display_order: order }),
      "Grupo salvo.",
    );
    if (ok && initial === blank) setV(blank);
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate>
      <div className="form-grid">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        <Field label="Ordem de exibição" error={fe.display_order}>
          {(p) => (
            <input {...p} type="number" min={0} max={100000} value={v.display_order} onChange={(e) => setV({ ...v, display_order: e.target.value })} />
          )}
        </Field>
      </div>
      <Field label="Descrição" error={fe.description} hint="Exibida ao expandir o grupo na página pública.">
        {(p) => <textarea {...p} rows={2} maxLength={1000} value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} />}
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

export function GroupsPage() {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const { state, reload } = useApi((signal) => adminApi.groups.list({ limit: 100 }, { signal }), []);

  return (
    <div className="admin-content">
      <p>
        <Link to="/admin/components">← Componentes</Link>
      </p>
      <PageHeader title="Grupos de componentes">
        <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
          Novo grupo
        </button>
      </PageHeader>
      <p className="muted">
        Grupos organizam componentes na página pública, onde aparecem recolhidos com o status mais grave entre seus componentes.
        Para incluir um componente, edite-o em Componentes e escolha o grupo.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo grupo">
          <GroupForm
            initial={blank}
            submitLabel="Criar grupo"
            onSubmit={async (body) => {
              const g = await adminApi.groups.create(body);
              reload();
              return g;
            }}
            onCancel={() => setCreating(false)}
          />
        </section>
      )}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum grupo cadastrado" />
        ) : (
          <ul className="admin-list">
            {state.data.data.map((g: ComponentGroup) => (
              <li key={g.id} className="admin-item">
                <div className="admin-item-head">
                  <div>
                    <strong>{g.name}</strong>
                    {g.description && <p className="muted">{g.description}</p>}
                    <div className="admin-item-tags">
                      <span className="tag">{plural(g.component_count, "componente", "componentes")}</span>
                      <span className="tag">Ordem {g.display_order}</span>
                    </div>
                  </div>
                </div>
                <div className="form-actions">
                  <button type="button" className="secondary-button" aria-expanded={editing === g.id} onClick={() => setEditing(editing === g.id ? null : g.id)}>
                    Editar
                  </button>
                  <ConfirmButton
                    variant="danger"
                    label="Excluir"
                    confirmLabel="Confirmar exclusão"
                    question={`Excluir o grupo "${g.name}"? Só é possível excluir grupos sem componentes.`}
                    disabled={g.component_count > 0}
                    onConfirm={async () => {
                      await adminApi.groups.remove(g.id);
                      reload();
                    }}
                  />
                  {g.component_count > 0 && <small className="muted">Mova os componentes para excluir o grupo.</small>}
                </div>
                {editing === g.id && (
                  <GroupForm
                    initial={{ name: g.name, description: g.description, display_order: String(g.display_order) }}
                    submitLabel="Salvar alterações"
                    onSubmit={async (body) => {
                      const saved = await adminApi.groups.update(g.id, body);
                      reload();
                      return saved;
                    }}
                    onCancel={() => setEditing(null)}
                  />
                )}
              </li>
            ))}
          </ul>
        ))}
    </div>
  );
}
