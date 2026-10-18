import { useId, useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Team, TeamCreate } from "../api/types";
import { TagInput } from "../components/TagInput";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { plural } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, useSubmit } from "./shared";

/** Suggestions only; any label up to 40 characters is accepted. */
const KINDS = ["Squad", "RT", "Comunidade", "Tribo", "Chapter", "Guilda", "Plataforma"];

type Values = { name: string; kind: string; description: string; documentation_url: string; support_url: string; tags: string[] };
const blank: Values = { name: "", kind: "", description: "", documentation_url: "", support_url: "", tags: [] };

const toValues = (t: Team): Values => ({
  name: t.name,
  kind: t.kind,
  description: t.description,
  documentation_url: t.documentation_url ?? "",
  support_url: t.support_url ?? "",
  tags: t.tags,
});

const isLink = (v: string) => /^https?:\/\/\S+$/i.test(v);

function TeamForm({ existing, onSaved, onCancel, tagSuggestions }: { existing?: Team; onSaved: () => void; onCancel?: () => void; tagSuggestions: string[] }) {
  const [v, setV] = useState(existing ? toValues(existing) : blank);
  const submit = useSubmit();
  const kindsId = useId();
  const set = (k: Exclude<keyof Values, "tags">, value: string) => setV((s) => ({ ...s, [k]: value }));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 80, "Nome");
    if (v.kind.trim().length > 40) errors.kind = "Tipo deve ter no máximo 40 caracteres.";
    for (const k of ["documentation_url", "support_url"] as const) {
      if (v[k].trim() && !isLink(v[k].trim())) errors[k] = "Informe um endereço completo, começando com https:// ou http://.";
    }
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body: TeamCreate = {
      name: v.name.trim(),
      kind: v.kind.trim(),
      description: v.description.trim(),
      documentation_url: v.documentation_url.trim() || null,
      support_url: v.support_url.trim() || null,
      tags: v.tags,
    };
    const ok = await submit.run(
      () => (existing ? adminApi.teams.update(existing.id, body) : adminApi.teams.create(body)),
      existing ? "Time atualizado." : "Time criado.",
    );
    if (ok) {
      if (!existing) setV(blank);
      onSaved();
    }
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={existing ? `Editar time ${existing.name}` : "Novo time"}>
      <div className="form-grid form-grid-2">
        <Field label="Nome do time" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={80} placeholder="Ex.: Squad Pagamentos" onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field label="Tipo" error={fe.kind} hint="Squad, RT, Comunidade, Tribo… ou outro rótulo.">
          {(p) => (
            <>
              <input {...p} list={kindsId} value={v.kind} maxLength={40} placeholder="Ex.: Squad" onChange={(e) => set("kind", e.target.value)} />
              <datalist id={kindsId}>
                {KINDS.map((k) => (
                  <option key={k} value={k} />
                ))}
              </datalist>
            </>
          )}
        </Field>
        <Field label="Documentação" error={fe.documentation_url} hint="Link da documentação do time ou dos serviços.">
          {(p) => (
            <input
              {...p}
              type="url"
              inputMode="url"
              maxLength={500}
              placeholder="https://wiki.exemplo.com/time"
              value={v.documentation_url}
              onChange={(e) => set("documentation_url", e.target.value)}
            />
          )}
        </Field>
        <Field label="Suporte" error={fe.support_url} hint="Como obter suporte (canal, formulário, plantão).">
          {(p) => (
            <input
              {...p}
              type="url"
              inputMode="url"
              maxLength={500}
              placeholder="https://suporte.exemplo.com/time"
              value={v.support_url}
              onChange={(e) => set("support_url", e.target.value)}
            />
          )}
        </Field>
      </div>
      <Field label="Descrição" error={fe.description}>
        {(p) => <textarea {...p} rows={2} maxLength={500} value={v.description} onChange={(e) => set("description", e.target.value)} />}
      </Field>
      <TagInput
        label="Tags"
        value={v.tags}
        onChange={(tags) => setV((s) => ({ ...s, tags }))}
        error={fe.tags}
        suggestions={tagSuggestions}
        hint="Rótulos livres (ex.: pagamentos, on-call 24x7). Enter ou vírgula para adicionar."
      />
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Salvando…" : existing ? "Salvar time" : "Criar time"}
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

export function TeamsPage() {
  const { state, reload } = useApi((signal) => adminApi.teams.list({}, { signal }), []);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [tagFilter, setTagFilter] = useState<string | null>(null);
  const canManage = useAuth().can("teams.manage");
  const allTeams = state.status === "success" ? state.data.data : [];
  const allTags = [...new Set(allTeams.flatMap((t) => t.tags))].sort((a, b) => a.localeCompare(b, "pt-BR"));
  const visible = tagFilter ? allTeams.filter((t) => t.tags.some((x) => x.toLowerCase() === tagFilter.toLowerCase())) : allTeams;

  return (
    <div className="admin-content">
      <PageHeader title="Times responsáveis">
        {canManage && (
          <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
            Novo time
          </button>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what="criar ou alterar times" />}
      <p className="muted">
        Times respondem por grupos, componentes e comunicados. Usuários membros de um time com um papel de escopo “time” operam
        apenas os componentes desse time — cada time cuida dos seus serviços. Membros são definidos em{" "}
        <Link to="/admin/users">Usuários</Link>.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo time">
          <TeamForm onSaved={reload} onCancel={() => setCreating(false)} tagSuggestions={allTags} />
        </section>
      )}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {allTags.length > 0 && (
        <div className="filters" role="group" aria-label="Filtrar por tag">
          <button type="button" className={tagFilter ? "chip-button" : "chip-button chip-button-active"} aria-pressed={!tagFilter} onClick={() => setTagFilter(null)}>
            Todas as tags
          </button>
          {allTags.map((tag) => (
            <button
              key={tag}
              type="button"
              className={tagFilter === tag ? "chip-button chip-button-active" : "chip-button"}
              aria-pressed={tagFilter === tag}
              onClick={() => setTagFilter(tagFilter === tag ? null : tag)}
            >
              #{tag}
            </button>
          ))}
        </div>
      )}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum time cadastrado">Crie times para distribuir a responsabilidade pelos componentes.</EmptyState>
        ) : (
          <ul className="admin-list">
            {visible.map((t) => {
              const inUse = t.component_count + t.group_count + t.announcement_count;
              return (
                <li key={t.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{t.name}</strong>
                      {t.kind && <span className="tag tag-team">{t.kind}</span>}
                      {!t.is_active && <span className="tag tag-warn">Desativado</span>}
                      {t.description && <p className="muted small">{t.description}</p>}
                      {t.tags.length > 0 && (
                        <ul className="team-tags" aria-label={`Tags de ${t.name}`}>
                          {t.tags.map((tag) => (
                            <li key={tag}>
                              <button type="button" className="tag tag-hash" onClick={() => setTagFilter(tag)} title="Filtrar por esta tag">
                                #{tag}
                              </button>
                            </li>
                          ))}
                        </ul>
                      )}
                      <div className="admin-item-tags">
                        <Link className="tag" to={`/admin/components?team=${t.id}`}>
                          {plural(t.component_count, "componente", "componentes")}
                        </Link>
                        <span className="tag">{plural(t.group_count, "grupo", "grupos")}</span>
                        <span className="tag">{plural(t.member_count, "membro", "membros")}</span>
                        <span className="tag">{plural(t.announcement_count, "comunicado", "comunicados")}</span>
                      </div>
                      {(t.documentation_url || t.support_url) && (
                        <p className="team-links small">
                          {t.documentation_url && (
                            <a href={t.documentation_url} target="_blank" rel="noopener noreferrer">
                              Documentação ↗
                            </a>
                          )}
                          {t.support_url && (
                            <a href={t.support_url} target="_blank" rel="noopener noreferrer">
                              Suporte ↗
                            </a>
                          )}
                        </p>
                      )}
                    </div>
                  </div>
                  {canManage && (
                    <div className="form-actions">
                      <button type="button" className="secondary-button" aria-expanded={editing === t.id} onClick={() => setEditing(editing === t.id ? null : t.id)}>
                        Editar
                      </button>
                      <ConfirmButton
                        variant="secondary"
                        label={t.is_active ? "Desativar" : "Ativar"}
                        confirmLabel="Confirmar"
                        question={t.is_active ? `Desativar "${t.name}"? Deixa de ser oferecido em novos vínculos.` : `Ativar "${t.name}"?`}
                        onConfirm={async () => {
                          await adminApi.teams.update(t.id, { is_active: !t.is_active });
                          reload();
                        }}
                      />
                      <ConfirmButton
                        variant="danger"
                        label="Excluir"
                        confirmLabel="Confirmar exclusão"
                        question={`Excluir "${t.name}"?`}
                        disabled={inUse > 0}
                        onConfirm={async () => {
                          await adminApi.teams.remove(t.id);
                          reload();
                        }}
                      />
                      {inUse > 0 && <small className="muted">Em uso: desvincule ou desative.</small>}
                    </div>
                  )}
                  {editing === t.id && <TeamForm existing={t} onSaved={reload} onCancel={() => setEditing(null)} tagSuggestions={allTags} />}
                </li>
              );
            })}
          </ul>
        ))}
    </div>
  );
}
