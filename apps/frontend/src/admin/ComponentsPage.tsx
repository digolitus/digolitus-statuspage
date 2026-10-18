import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Component, ComponentCreate, ComponentGroup, ComponentStatus } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { componentStatusLabel, toneForStatus } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { FormStatus, PageHeader, requireText, useSubmit } from "./shared";

const STATUSES = Object.keys(componentStatusLabel) as ComponentStatus[];

type FormValues = {
  name: string;
  slug: string;
  description: string;
  status: ComponentStatus;
  is_public: boolean;
  display_order: string;
  group_id: string;
};

const empty: FormValues = { name: "", slug: "", description: "", status: "operational", is_public: true, display_order: "0", group_id: "" };

function toValues(c: Component): FormValues {
  return {
    name: c.name,
    slug: c.slug,
    description: c.description,
    status: c.status,
    is_public: c.is_public,
    display_order: String(c.display_order),
    group_id: c.group_id ?? "",
  };
}

function ComponentForm({
  initial,
  groups,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: FormValues;
  groups: ComponentGroup[];
  submitLabel: string;
  onSubmit: (input: ComponentCreate) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const set = <K extends keyof FormValues>(k: K, value: FormValues[K]) => setV((s) => ({ ...s, [k]: value }));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    const order = Number(v.display_order);
    if (!Number.isInteger(order) || order < 0 || order > 100000) errors.display_order = "Informe um inteiro entre 0 e 100000.";
    if (Object.keys(errors).length) {
      submit.setFieldErrors(errors);
      return;
    }
    const input: ComponentCreate = {
      name: v.name.trim(),
      description: v.description.trim(),
      status: v.status,
      is_public: v.is_public,
      display_order: order,
      group_id: v.group_id || null,
    };
    if (v.slug.trim()) input.slug = v.slug.trim();
    const ok = await submit.run(() => onSubmit(input), "Componente salvo.");
    if (ok && initial === empty) setV(empty);
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate>
      <div className="form-grid">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field label="Slug" error={fe.slug} hint="Opcional na criação; derivado do nome.">
          {(p) => <input {...p} value={v.slug} maxLength={80} onChange={(e) => set("slug", e.target.value)} />}
        </Field>
        <Field label="Status declarado" error={fe.status}>
          {(p) => (
            <select {...p} value={v.status} onChange={(e) => set("status", e.target.value as ComponentStatus)}>
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {componentStatusLabel[s]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Grupo" error={fe.group_id} hint="Opcional. Grupos aparecem recolhidos na página pública.">
          {(p) => (
            <select {...p} value={v.group_id} onChange={(e) => set("group_id", e.target.value)}>
              <option value="">Sem grupo</option>
              {groups.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Ordem de exibição" error={fe.display_order}>
          {(p) => <input {...p} type="number" min={0} max={100000} value={v.display_order} onChange={(e) => set("display_order", e.target.value)} />}
        </Field>
      </div>
      <Field label="Descrição" error={fe.description}>
        {(p) => <textarea {...p} rows={2} maxLength={1000} value={v.description} onChange={(e) => set("description", e.target.value)} />}
      </Field>
      <label className="checkbox">
        <input type="checkbox" checked={v.is_public} onChange={(e) => set("is_public", e.target.checked)} />
        Exibir na página pública
      </label>
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

export function ComponentsPage() {
  const [filter, setFilter] = useState<"active" | "inactive" | "all">("active");
  const [offset, setOffset] = useState(0);
  const [editing, setEditing] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [groupFilter, setGroupFilter] = useState("");
  const isActive = filter === "all" ? undefined : filter === "active";
  const { state, reload } = useApi(
    (signal) => adminApi.components.list({ is_active: isActive, group_id: groupFilter || undefined, limit: 50, offset }, { signal }),
    [filter, groupFilter, offset],
  );
  const groupsState = useApi((signal) => adminApi.groups.list({ limit: 100 }, { signal }), []).state;
  const groups = groupsState.status === "success" ? groupsState.data.data : [];
  const groupName = (id: string | null) => groups.find((g) => g.id === id)?.name;

  return (
    <div className="admin-content">
      <PageHeader title="Componentes">
        <Link className="secondary-button" to="/admin/component-groups">
          Gerenciar grupos
        </Link>
        <button type="button" className="primary-button" onClick={() => setCreating((c) => !c)} aria-expanded={creating}>
          Novo componente
        </button>
      </PageHeader>
      <p className="muted">
        O status de um componente é declarado manualmente. Incidentes ativos e manutenções em andamento podem agravar o status
        exibido publicamente, conforme a política documentada na API.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo componente">
          <ComponentForm
            initial={empty}
            groups={groups}
            submitLabel="Criar componente"
            onSubmit={async (input) => {
              const c = await adminApi.components.create(input);
              reload();
              return c;
            }}
            onCancel={() => setCreating(false)}
          />
        </section>
      )}
      <div className="filters" role="group" aria-label="Filtrar componentes">
        {(["active", "inactive", "all"] as const).map((f) => (
          <button
            key={f}
            type="button"
            className={filter === f ? "chip-button chip-button-active" : "chip-button"}
            aria-pressed={filter === f}
            onClick={() => {
              setFilter(f);
              setOffset(0);
            }}
          >
            {f === "active" ? "Ativos" : f === "inactive" ? "Desativados" : "Todos"}
          </button>
        ))}
      </div>
      {groups.length > 0 && (
        <label className="inline-filter">
          Grupo{" "}
          <select
            value={groupFilter}
            onChange={(e) => {
              setGroupFilter(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">Todos</option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </label>
      )}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum componente encontrado" />
        ) : (
          <>
            <ul className="admin-list">
              {state.data.data.map((c) => (
                <li key={c.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{c.name}</strong> <code>{c.slug}</code>
                      <div className="admin-item-tags">
                        {c.group_id && <span className="tag">Grupo: {groupName(c.group_id) ?? "…"}</span>}
                        {!c.is_public && <span className="tag">Oculto</span>}
                        {!c.is_active && <span className="tag">Desativado</span>}
                        <span className="tag">Ordem {c.display_order}</span>
                      </div>
                    </div>
                    <Badge tone={toneForStatus[c.status]}>{componentStatusLabel[c.status]}</Badge>
                  </div>
                  <div className="form-actions">
                    <button type="button" className="secondary-button" onClick={() => setEditing(editing === c.id ? null : c.id)} aria-expanded={editing === c.id}>
                      Editar
                    </button>
                    <ConfirmButton
                      variant={c.is_active ? "danger" : "secondary"}
                      label={c.is_active ? "Desativar" : "Reativar"}
                      confirmLabel={c.is_active ? "Confirmar desativação" : "Confirmar reativação"}
                      question={
                        c.is_active
                          ? `Desativar "${c.name}"? Ele sairá da página pública; o histórico é preservado.`
                          : `Reativar "${c.name}"?`
                      }
                      onConfirm={async () => {
                        await adminApi.components.update(c.id, { is_active: !c.is_active });
                        reload();
                      }}
                    />
                  </div>
                  {editing === c.id && (
                    <ComponentForm
                      initial={toValues(c)}
                      groups={groups}
                      submitLabel="Salvar alterações"
                      onSubmit={async (input) => {
                        const updated = await adminApi.components.update(c.id, input);
                        reload();
                        return updated;
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
