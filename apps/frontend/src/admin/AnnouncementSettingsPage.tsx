import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { AnnouncementCategory, CategoryColor, Team } from "../api/types";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, useSubmit } from "./shared";

const colorLabel: Record<CategoryColor, string> = {
  gray: "Cinza",
  blue: "Azul",
  green: "Verde",
  yellow: "Amarelo",
  orange: "Laranja",
  red: "Vermelho",
  purple: "Roxo",
};
const COLORS = Object.keys(colorLabel) as CategoryColor[];

type CategoryValues = { name: string; description: string; color: CategoryColor; display_order: string };

function CategoryForm({ initial, onSaved, existing }: { initial: CategoryValues; existing?: AnnouncementCategory; onSaved: () => void }) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 60, "Nome");
    const order = Number(v.display_order);
    if (!Number.isInteger(order) || order < 0 || order > 100000) errors.display_order = "Informe um inteiro entre 0 e 100000.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body = { name: v.name.trim(), description: v.description.trim(), color: v.color, display_order: order };
    const ok = await submit.run(
      () => (existing ? adminApi.announcementCategories.update(existing.id, body) : adminApi.announcementCategories.create(body)),
      existing ? "Categoria atualizada." : "Categoria criada.",
    );
    if (ok) {
      if (!existing) setV({ name: "", description: "", color: "gray", display_order: "0" });
      onSaved();
    }
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={existing ? `Editar categoria ${existing.name}` : "Nova categoria"}>
      <div className="form-grid">
        <Field label="Nome da categoria" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={60} placeholder="Ex.: Novidades" onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        <Field label="Cor" error={fe.color}>
          {(p) => (
            <select {...p} value={v.color} onChange={(e) => setV({ ...v, color: e.target.value as CategoryColor })}>
              {COLORS.map((c) => (
                <option key={c} value={c}>
                  {colorLabel[c]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Ordem" error={fe.display_order}>
          {(p) => <input {...p} type="number" min={0} max={100000} value={v.display_order} onChange={(e) => setV({ ...v, display_order: e.target.value })} />}
        </Field>
      </div>
      <Field label="Descrição / orientação de escrita" error={fe.description} hint="Aparece no editor ao escolher a categoria e no filtro público.">
        {(p) => <input {...p} value={v.description} maxLength={500} onChange={(e) => setV({ ...v, description: e.target.value })} />}
      </Field>
      <p className="small">
        Pré-visualização: <span className={`category-chip category-${v.color}`}>{v.name.trim() || "Categoria"}</span>
      </p>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {existing ? "Salvar categoria" : "Criar categoria"}
        </button>
      </div>
    </form>
  );
}

type TeamValues = { name: string; description: string };

function TeamForm({ initial, existing, onSaved }: { initial: TeamValues; existing?: Team; onSaved: () => void }) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 80, "Nome");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body = { name: v.name.trim(), description: v.description.trim() };
    const ok = await submit.run(
      () => (existing ? adminApi.teams.update(existing.id, body) : adminApi.teams.create(body)),
      existing ? "Time atualizado." : "Time criado.",
    );
    if (ok) {
      if (!existing) setV({ name: "", description: "" });
      onSaved();
    }
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={existing ? `Editar time ${existing.name}` : "Novo time"}>
      <div className="form-grid">
        <Field label="Nome do time" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={80} placeholder="Ex.: Time de Plataforma" onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        <Field label="Descrição" error={fe.description}>
          {(p) => <input {...p} value={v.description} maxLength={500} onChange={(e) => setV({ ...v, description: e.target.value })} />}
        </Field>
      </div>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {existing ? "Salvar time" : "Criar time"}
        </button>
      </div>
    </form>
  );
}

function ItemActions({
  name,
  active,
  count,
  editing,
  onEdit,
  onToggle,
  onDelete,
}: {
  name: string;
  active: boolean;
  count: number;
  editing: boolean;
  onEdit: () => void;
  onToggle: () => Promise<unknown>;
  onDelete: () => Promise<unknown>;
}) {
  return (
    <div className="form-actions">
      <button type="button" className="secondary-button" aria-expanded={editing} onClick={onEdit}>
        Editar
      </button>
      <ConfirmButton
        variant="secondary"
        label={active ? "Desativar" : "Ativar"}
        confirmLabel="Confirmar"
        question={active ? `Desativar "${name}"? Deixa de ser oferecido para novos comunicados.` : `Ativar "${name}"?`}
        onConfirm={async () => {
          await onToggle();
        }}
      />
      {count === 0 && (
        <ConfirmButton
          variant="danger"
          label="Excluir"
          confirmLabel="Confirmar exclusão"
          question={`Excluir "${name}"?`}
          onConfirm={async () => {
            await onDelete();
          }}
        />
      )}
    </div>
  );
}

export function AnnouncementSettingsPage() {
  const categories = useApi((signal) => adminApi.announcementCategories.list({}, { signal }), []);
  const teams = useApi((signal) => adminApi.teams.list({}, { signal }), []);
  const [editing, setEditing] = useState<string | null>(null);
  const toggle = (id: string) => setEditing((e) => (e === id ? null : id));
  const canManage = useAuth().can("announcements.settings");

  return (
    <div className="admin-content">
      <PageHeader title="Categorias e times" />
      <p>
        <Link to="/admin/announcements">← Comunicados</Link>
      </p>
      <p className="muted">
        Categorias organizam os comunicados (ex.: Novidades, Avisos, Segurança) e viram filtros na página pública. Times
        identificam quem responde pelo comunicado. Itens em uso não podem ser excluídos; desative-os.
      </p>

      <section className="panel" aria-labelledby="cat-title">
        <h2 id="cat-title">Categorias</h2>
        {canManage ? (
          <CategoryForm initial={{ name: "", description: "", color: "blue", display_order: "0" }} onSaved={categories.reload} />
        ) : (
          <ReadOnlyNote what="criar ou alterar categorias e times" />
        )}
        {categories.state.status === "loading" && <Loading />}
        {categories.state.status === "error" && <ErrorPanel error={categories.state.error} onRetry={categories.reload} />}
        {categories.state.status === "success" &&
          (categories.state.data.data.length === 0 ? (
            <EmptyState title="Nenhuma categoria" />
          ) : (
            <ul className="admin-list">
              {categories.state.data.data.map((c) => (
                <li key={c.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <span className={`category-chip category-${c.color}`}>{c.name}</span>
                      <div className="admin-item-tags">
                        <span className="tag">{c.announcement_count} comunicado(s)</span>
                        <span className="tag">Ordem {c.display_order}</span>
                        {!c.is_active && <span className="tag tag-warn">Desativada</span>}
                      </div>
                      {c.description && <p className="muted small">{c.description}</p>}
                    </div>
                  </div>
                  {canManage && <ItemActions
                    name={c.name}
                    active={c.is_active}
                    count={c.announcement_count}
                    editing={editing === c.id}
                    onEdit={() => toggle(c.id)}
                    onToggle={() => adminApi.announcementCategories.update(c.id, { is_active: !c.is_active }).then(categories.reload)}
                    onDelete={() => adminApi.announcementCategories.remove(c.id).then(categories.reload)}
                  />}
                  {editing === c.id && (
                    <CategoryForm
                      existing={c}
                      initial={{ name: c.name, description: c.description, color: c.color, display_order: String(c.display_order) }}
                      onSaved={categories.reload}
                    />
                  )}
                </li>
              ))}
            </ul>
          ))}
      </section>

      <section className="panel" aria-labelledby="team-title">
        <h2 id="team-title">Times responsáveis</h2>
        {canManage && <TeamForm initial={{ name: "", description: "" }} onSaved={teams.reload} />}
        {teams.state.status === "loading" && <Loading />}
        {teams.state.status === "error" && <ErrorPanel error={teams.state.error} onRetry={teams.reload} />}
        {teams.state.status === "success" &&
          (teams.state.data.data.length === 0 ? (
            <EmptyState title="Nenhum time" />
          ) : (
            <ul className="admin-list">
              {teams.state.data.data.map((t) => (
                <li key={t.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{t.name}</strong>
                      <div className="admin-item-tags">
                        <span className="tag">{t.announcement_count} comunicado(s)</span>
                        {!t.is_active && <span className="tag tag-warn">Desativado</span>}
                      </div>
                      {t.description && <p className="muted small">{t.description}</p>}
                    </div>
                  </div>
                  {canManage && <ItemActions
                    name={t.name}
                    active={t.is_active}
                    count={t.announcement_count}
                    editing={editing === t.id}
                    onEdit={() => toggle(t.id)}
                    onToggle={() => adminApi.teams.update(t.id, { is_active: !t.is_active }).then(teams.reload)}
                    onDelete={() => adminApi.teams.remove(t.id).then(teams.reload)}
                  />}
                  {editing === t.id && <TeamForm existing={t} initial={{ name: t.name, description: t.description }} onSaved={teams.reload} />}
                </li>
              ))}
            </ul>
          ))}
      </section>
    </div>
  );
}
