import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Department, DepartmentLevels, Team, User } from "../api/types";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { plural } from "../lib/format";
import { useApi } from "../lib/useApi";
import { InfoTip } from "../components/InfoTip";
import { useAuth } from "./authContext";
import { FormStatus, requireText, useSubmit } from "./shared";

export const MAX_LEVELS = 4;

/** "Diretoria" when the level is named, "Nível 2" otherwise. */
export function levelName(levels: string[], level: number): string {
  return levels[level - 1]?.trim() || `Nível ${level}`;
}

/** "Tecnologia › Plataforma › SRE". */
export function departmentPath(d: Pick<Department, "path">): string {
  return d.path.map((p) => p.name).join(" › ");
}

/** Departments and level names, for selects and labels (any signed-in user may list them). */
export function useDepartments() {
  const { state, reload } = useApi(async (signal) => {
    const [ds, levels] = await Promise.all([adminApi.departments.list({ signal }), adminApi.departments.levels({ signal })]);
    return { departments: ds.data, levels: levels.names };
  }, []);
  return { state, reload, departments: state.status === "success" ? state.data.departments : [], levels: state.status === "success" ? state.data.levels : [] };
}

export function LevelBadge({ levels, level }: { levels: string[]; level: number }) {
  return <span className={`tag dept-level dept-level-${level}`}>{levelName(levels, level)}</span>;
}

/** Optional department, any level, shown with its full path. */
export function DepartmentSelect({
  departments,
  levels,
  value,
  onChange,
  error,
  label = "Departamento",
  exclude,
  maxLevel = MAX_LEVELS,
  emptyLabel = "Nenhum departamento",
}: {
  departments: Department[];
  levels: string[];
  value: string;
  onChange: (id: string) => void;
  error?: string;
  label?: string;
  exclude?: (d: Department) => boolean;
  maxLevel?: number;
  emptyLabel?: string;
}) {
  const options = departments.filter((d) => d.level <= maxLevel && !exclude?.(d));
  return (
    <Field label={label} error={error}>
      {(p) => (
        <select {...p} value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">{emptyLabel}</option>
          {options.map((d) => (
            <option key={d.id} value={d.id}>
              {`${" ".repeat(d.level - 1)}${departmentPath(d)} (${levelName(levels, d.level)})`}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}

/** Checkbox list of departments for a user's direct placement. */
export function DepartmentPicker({
  departments,
  levels,
  selected,
  onChange,
  error,
}: {
  departments: Department[];
  levels: string[];
  selected: string[];
  onChange: (ids: string[]) => void;
  error?: string;
}) {
  return (
    <fieldset className={`field picker${error ? " field-invalid" : ""}`}>
      <legend>
        Departamentos <InfoTip label="Sobre Departamentos" text="Para quem fica direto no departamento, sem time (ex.: um gerente). É só organização; não dá permissão." />
      </legend>
      {departments.length === 0 ? (
        <small className="field-hint">Nenhum departamento.</small>
      ) : (
        <div className="picker-options">
          {departments.map((d) => (
            <label key={d.id} className="checkbox" style={{ paddingLeft: `${(d.level - 1) * 1.1}rem` }}>
              <input
                type="checkbox"
                checked={selected.includes(d.id)}
                onChange={(e) => onChange(e.target.checked ? [...selected, d.id] : selected.filter((x) => x !== d.id))}
              />
              <span>{d.name}</span>
              <LevelBadge levels={levels} level={d.level} />
            </label>
          ))}
        </div>
      )}
      {error && <small className="field-error">{error}</small>}
    </fieldset>
  );
}

function LevelsEditor({ levels, onSaved, canManage }: { levels: string[]; onSaved: () => void; canManage: boolean }) {
  const [names, setNames] = useState(() => Array.from({ length: MAX_LEVELS }, (_, i) => levels[i] ?? ""));
  const submit = useSubmit();
  async function save(e: FormEvent) {
    e.preventDefault();
    const body: DepartmentLevels = { names: names.map((n) => n.trim()) };
    if (await submit.run(() => adminApi.departments.setLevels(body), "Níveis salvos.")) onSaved();
  }
  return (
    <form className="panel form" onSubmit={save} aria-label="Níveis da organização">
      <h3 className="subheading">
        Níveis da organização{" "}
        <InfoTip
          label="Sobre Níveis"
          text="Até 4 níveis, todos opcionais. O nome é só um rótulo (ex.: Diretoria › Gerência › Coordenação); sem nome aparece “Nível N”. O nível de um departamento é a posição dele na árvore."
        />
      </h3>
      <div className="form-grid dept-levels">
        {names.map((n, i) => (
          <Field key={i} label={`Nível ${i + 1}`} error={submit.fieldErrors[`names[${i}]`]}>
            {(p) => (
              <input
                {...p}
                value={n}
                maxLength={40}
                placeholder="Sem nome"
                disabled={!canManage}
                onChange={(e) => setNames((cur) => cur.map((x, j) => (j === i ? e.target.value : x)))}
              />
            )}
          </Field>
        ))}
      </div>
      <FormStatus error={submit.formError} success={submit.success} />
      {canManage && (
        <div className="form-actions">
          <button type="submit" className="secondary-button" disabled={submit.busy}>
            Salvar níveis
          </button>
        </div>
      )}
    </form>
  );
}

/** Ids of d and every department below it. */
function subtree(departments: Department[], d: Department): Set<string> {
  return new Set(departments.filter((x) => x.path.some((p) => p.id === d.id)).map((x) => x.id));
}

function DepartmentForm({
  existing,
  parentId,
  departments,
  levels,
  users,
  onSaved,
  onCancel,
}: {
  existing?: Department;
  parentId?: string;
  departments: Department[];
  levels: string[];
  users: User[] | null;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(existing?.name ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [parent, setParent] = useState(existing?.parent_id ?? parentId ?? "");
  const [memberIds, setMemberIds] = useState(existing?.members.map((m) => m.id) ?? []);
  const [adding, setAdding] = useState("");
  const submit = useSubmit();
  const below = existing ? subtree(departments, existing) : new Set<string>();
  // Levels the edited subtree spans, so a move cannot push it past level 4.
  const height = existing ? Math.max(...departments.filter((d) => below.has(d.id)).map((d) => d.level)) - existing.level + 1 : 1;
  const parentDept = departments.find((d) => d.id === parent);
  const level = parentDept ? parentDept.level + 1 : 1;
  const known = new Map<string, { name: string; job_title: string }>();
  existing?.members.forEach((m) => known.set(m.id, m));
  users?.forEach((u) => known.set(u.id, u));

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", name, 80, "Nome");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body = { name: name.trim(), description: description.trim(), parent_id: parent || null, member_ids: memberIds };
    const ok = await submit.run(
      () => (existing ? adminApi.departments.update(existing.id, body) : adminApi.departments.create(body)),
      existing ? "Departamento atualizado." : "Departamento criado.",
    );
    if (ok) onSaved();
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={existing ? `Editar departamento ${existing.name}` : "Novo departamento"}>
      <div className="form-grid form-grid-2">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={name} maxLength={80} placeholder="Ex.: Tecnologia" onChange={(e) => setName(e.target.value)} />}
        </Field>
        <DepartmentSelect
          label={`Fica abaixo de · nível ${level}: ${levelName(levels, level)}`}
          departments={departments}
          levels={levels}
          value={parent}
          onChange={setParent}
          error={fe.parent_id}
          exclude={(d) => below.has(d.id)}
          maxLevel={MAX_LEVELS - height}
          emptyLabel="Nenhum (nível 1)"
        />
      </div>
      <Field label="Descrição" error={fe.description}>
        {(p) => <textarea {...p} rows={2} maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} />}
      </Field>
      <fieldset className={`field picker${fe.member_ids ? " field-invalid" : ""}`}>
        <legend>
          Pessoas direto no departamento{" "}
          <InfoTip label="Sobre Pessoas direto" text="Para quem não pertence a um time (ex.: gerente). Quem está num time do departamento já aparece por ele. O cargo é editado em Usuários." />
        </legend>
        {memberIds.length > 0 && (
          <ul className="dept-chips">
            {memberIds.map((id) => {
              const m = known.get(id);
              return (
                <li key={id} className="dept-chip">
                  {m?.name ?? "Pessoa"}
                  {m?.job_title && <span className="muted"> · {m.job_title}</span>}
                  <button type="button" className="link-button" aria-label={`Remover ${m?.name ?? "pessoa"}`} onClick={() => setMemberIds(memberIds.filter((x) => x !== id))}>
                    ×
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        {users && (
          <select
            aria-label="Adicionar pessoa"
            value={adding}
            onChange={(e) => {
              if (e.target.value) setMemberIds([...memberIds, e.target.value]);
              setAdding("");
            }}
          >
            <option value="">Adicionar pessoa…</option>
            {users
              .filter((u) => u.is_active && !memberIds.includes(u.id))
              .map((u) => (
                <option key={u.id} value={u.id}>
                  {u.name} ({u.email})
                </option>
              ))}
          </select>
        )}
        {fe.member_ids && <small className="field-error">{fe.member_ids}</small>}
      </fieldset>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {existing ? "Salvar departamento" : "Criar departamento"}
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Fechar
        </button>
      </div>
    </form>
  );
}

type Editing = { mode: "new"; parentId?: string } | { mode: "edit"; id: string } | null;

/** "Departamentos" tab of the teams page: level names and the tree. */
export function DepartmentsPanel({ teams, editing, setEditing }: { teams: Team[]; editing: Editing; setEditing: (e: Editing) => void }) {
  const { state, reload, departments, levels } = useDepartments();
  const { can } = useAuth();
  const canManage = can("teams.manage");
  const users = useApi((signal) => (can("users.manage") ? adminApi.users.list({ signal }).then((r) => r.data) : Promise.resolve(null)), []);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const userList = users.state.status === "success" ? users.state.data : null;
  const saved = () => {
    setEditing(null);
    reload();
  };

  if (state.status === "loading") return <Loading />;
  if (state.status === "error") return <ErrorPanel error={state.error} onRetry={reload} />;

  const q = query.trim().toLowerCase();
  const teamsOf = (id: string | null) => teams.filter((t) => (t.department_id ?? null) === id);
  const hidden = (d: Department) => d.path.slice(0, -1).some((p) => collapsed.has(p.id));
  const matches = (d: Department) =>
    !q ||
    departments.some((x) => x.path.some((p) => p.id === d.id) && (x.name.toLowerCase().includes(q) || teamsOf(x.id).some((t) => t.name.toLowerCase().includes(q))));
  const toggle = (id: string) =>
    setCollapsed((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const form = (props: { existing?: Department; parentId?: string }) => (
    <div className="dept-form">
      <DepartmentForm {...props} departments={departments} levels={levels} users={userList} onSaved={saved} onCancel={() => setEditing(null)} />
    </div>
  );
  const loose = teamsOf(null);

  return (
    <>
      <LevelsEditor key={levels.join("|")} levels={levels} onSaved={reload} canManage={canManage} />
      {editing?.mode === "new" && !editing.parentId && <section className="panel">{form({})}</section>}
      {departments.length === 0 ? (
        <EmptyState title="Nenhum departamento">Opcional. Crie a estrutura e depois vincule times e pessoas.</EmptyState>
      ) : (
        <div className="dept-tree">
          <div className="dept-tools">
            <input type="search" aria-label="Buscar departamento ou time" placeholder="Buscar departamento ou time…" value={query} onChange={(e) => setQuery(e.target.value)} />
            <div className="form-actions">
              <button type="button" className="link-button" onClick={() => setCollapsed(new Set())}>
                Expandir tudo
              </button>
              <button type="button" className="link-button" onClick={() => setCollapsed(new Set(departments.filter((d) => d.child_count > 0 || d.team_count > 0 || d.members.length > 0).map((d) => d.id)))}>
                Recolher
              </button>
            </div>
          </div>
          <ul className="dept-rows">
            {departments
              .filter((d) => !hidden(d) && matches(d))
              .map((d) => {
                const open = !collapsed.has(d.id);
                const hasContent = d.child_count + d.team_count + d.members.length > 0;
                const indent = { paddingLeft: `${(d.level - 1) * 1.4 + 0.75}rem` };
                const childIndent = { paddingLeft: `${d.level * 1.4 + 0.75}rem` };
                return (
                  <li key={d.id}>
                    <div className="dept-row" style={indent}>
                      <button
                        type="button"
                        className="dept-toggle"
                        aria-expanded={hasContent ? open : undefined}
                        aria-label={open ? `Recolher ${d.name}` : `Expandir ${d.name}`}
                        disabled={!hasContent}
                        onClick={() => toggle(d.id)}
                      >
                        {hasContent ? (open ? "▾" : "▸") : ""}
                      </button>
                      <LevelBadge levels={levels} level={d.level} />
                      <strong>{d.name}</strong>
                      {d.description && <InfoTip label={`Sobre ${d.name}`} text={d.description} />}
                      <span className="dept-meta">
                        {d.child_count > 0 && <span className="tag">{plural(d.child_count, "subdepartamento", "subdepartamentos")}</span>}
                        <span className="tag">
                          {plural(d.total_team_count, "time", "times")} · {plural(d.total_people_count, "pessoa", "pessoas")}
                        </span>
                        {canManage && (
                          <>
                            {d.level < MAX_LEVELS && (
                              <button type="button" className="link-button" onClick={() => setEditing({ mode: "new", parentId: d.id })}>
                                + Subdepartamento
                              </button>
                            )}
                            <button type="button" className="link-button" onClick={() => setEditing(editing?.mode === "edit" && editing.id === d.id ? null : { mode: "edit", id: d.id })}>
                              Editar
                            </button>
                            <ConfirmButton
                              variant="danger"
                              label="Excluir"
                              confirmLabel="Confirmar exclusão"
                              question={`Excluir "${d.name}"?`}
                              disabled={d.child_count + d.team_count > 0}
                              onConfirm={async () => {
                                await adminApi.departments.remove(d.id);
                                reload();
                              }}
                            />
                          </>
                        )}
                      </span>
                    </div>
                    {editing?.mode === "edit" && editing.id === d.id && form({ existing: d })}
                    {editing?.mode === "new" && editing.parentId === d.id && form({ parentId: d.id })}
                    {open && (
                      <ul className="dept-leaves">
                        {d.members.map((m) => (
                          <li key={m.id} className="dept-row dept-person" style={childIndent}>
                            <span className="dept-avatar" aria-hidden="true">
                              {initials(m.name)}
                            </span>
                            <span>{m.name}</span>
                            {m.job_title && <span className="tag">{m.job_title}</span>}
                            {!m.is_active && <span className="tag tag-warn">Desativado</span>}
                          </li>
                        ))}
                        {teamsOf(d.id).map((t) => (
                          <li key={t.id} className="dept-row dept-team" style={childIndent}>
                            <span className="dept-avatar dept-avatar-team" aria-hidden="true">
                              T
                            </span>
                            <span>{t.name}</span>
                            {t.kind && <span className="tag tag-team">{t.kind}</span>}
                            <span className="dept-meta">
                              <span className="tag">{plural(t.member_count, "pessoa", "pessoas")}</span>
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </li>
                );
              })}
            {loose.length > 0 && !q && (
              <li>
                <div className="dept-row dept-loose">
                  <button type="button" className="dept-toggle" aria-expanded={!collapsed.has("")} aria-label="Times sem departamento" onClick={() => toggle("")}>
                    {collapsed.has("") ? "▸" : "▾"}
                  </button>
                  <strong>Sem departamento</strong>
                  <span className="dept-meta">
                    <span className="tag">{plural(loose.length, "time", "times")}</span>
                  </span>
                </div>
                {!collapsed.has("") && (
                  <ul className="dept-leaves">
                    {loose.map((t) => (
                      <li key={t.id} className="dept-row dept-team" style={{ paddingLeft: "2.15rem" }}>
                        <span className="dept-avatar dept-avatar-team" aria-hidden="true">
                          T
                        </span>
                        <span>{t.name}</span>
                        {t.kind && <span className="tag tag-team">{t.kind}</span>}
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            )}
          </ul>
        </div>
      )}
    </>
  );
}

function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join("");
}
