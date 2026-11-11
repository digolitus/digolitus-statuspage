import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { ComponentGroup, ComponentGroupCreate, Team } from "../api/types";
import { ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { plural } from "../lib/format";
import { groupTree } from "../lib/groups";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, TeamSelect, useSubmit, useTeams } from "./shared";

type Values = { name: string; description: string; display_order: string; team_id: string; parent_id: string };
const blank: Values = { name: "", description: "", display_order: "0", team_id: "", parent_id: "" };

function toValues(g: ComponentGroup): Values {
  return { name: g.name, description: g.description, display_order: String(g.display_order), team_id: g.team_id ?? "", parent_id: g.parent_id ?? "" };
}

/**
 * Creates or edits a group or a subgroup. The parent can be chosen for
 * subgroups (moving it with its components) and for empty groups (turning
 * them into a subgroup); otherwise the level is fixed, as the API requires.
 */
function GroupForm({
  initial,
  teams,
  parents,
  canChangeParent,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: Values;
  teams: Team[];
  /** Top-level groups the item may belong to. */
  parents: ComponentGroup[];
  canChangeParent: boolean;
  submitLabel: string;
  onSubmit: (body: ComponentGroupCreate) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const isSub = v.parent_id !== "";
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    const order = Number(v.display_order);
    if (!Number.isInteger(order) || order < 0 || order > 100000) errors.display_order = "Informe um inteiro entre 0 e 100000.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body: ComponentGroupCreate = { name: v.name.trim(), description: v.description.trim(), display_order: order, team_id: v.team_id || null };
    if (canChangeParent || initial.parent_id !== v.parent_id) body.parent_id = v.parent_id || null;
    const ok = await submit.run(() => onSubmit(body), isSub ? "Subgrupo salvo." : "Grupo salvo.");
    if (ok && initial === blank) setV(blank);
  }
  const fe = submit.fieldErrors;
  const parentTeam = teams.find((t) => t.id === parents.find((p) => p.id === v.parent_id)?.team_id);
  return (
    <form className="form" onSubmit={handle} noValidate>
      <div className="form-grid form-grid-3">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        {canChangeParent && (
          <Field
            label={initial.parent_id ? "Grupo pai" : "Grupo pai (opcional)"}
            error={fe.parent_id}
            info={initial.parent_id ? "Ao mover, os componentes do subgrupo vão junto." : "Escolha um grupo para transformar este em subgrupo."}
          >
            {(p) => (
              <select {...p} value={v.parent_id} onChange={(e) => setV({ ...v, parent_id: e.target.value })}>
                <option value="">{initial.parent_id ? "Nenhum (vira grupo principal)" : "Nenhum (grupo principal)"}</option>
                {parents.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.name}
                  </option>
                ))}
              </select>
            )}
          </Field>
        )}
        <Field label="Ordem de exibição" error={fe.display_order}>
          {(p) => (
            <input {...p} type="number" min={0} max={100000} value={v.display_order} onChange={(e) => setV({ ...v, display_order: e.target.value })} />
          )}
        </Field>
        <TeamSelect
          teams={teams}
          value={v.team_id}
          onChange={(team_id) => setV({ ...v, team_id })}
          permission="components.manage"
          error={fe.team_id}
          info={isSub ? "Vale para os componentes do subgrupo sem time próprio." : "Vale para os componentes do grupo sem time próprio."}
          emptyLabel={isSub && parentTeam ? `Herdar do grupo (${parentTeam.name})` : undefined}
        />
      </div>
      <Field label="Descrição" error={fe.description} info={isSub ? "Exibida ao expandir o subgrupo na página pública." : "Exibida ao expandir o grupo na página pública."}>
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

function Tags({ g, teamName, parentTeam }: { g: ComponentGroup; teamName: (id: string | null) => string | undefined; parentTeam?: string | null }) {
  return (
    <div className="admin-item-tags">
      <span className="tag">{plural(g.component_count, "componente", "componentes")}</span>
      {!g.parent_id && <span className="tag">{g.subgroup_count > 0 ? plural(g.subgroup_count, "subgrupo", "subgrupos") : "Sem subgrupos"}</span>}
      <span className="tag">Ordem {g.display_order}</span>
      {g.team_id && <span className="tag tag-team">Time: {teamName(g.team_id) ?? "…"}</span>}
      {!g.team_id && g.parent_id && parentTeam && <span className="tag">Time herdado do grupo</span>}
    </div>
  );
}

export function GroupsPage() {
  const [creating, setCreating] = useState(false);
  const [creatingSub, setCreatingSub] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const { state, reload } = useApi((signal) => adminApi.groups.all({ signal }), []);
  const { can, canFor } = useAuth();
  const canManage = can("components.manage");
  const teams = useTeams();
  const teamName = (id: string | null) => teams.find((t) => t.id === id)?.name;
  const tree = state.status === "success" ? groupTree(state.data) : [];
  const parents = tree.map((n) => n.group);

  const actions = (g: ComponentGroup, blocked: string | null) =>
    canFor("components.manage", g.team_id) && (
      <>
        <button type="button" className="secondary-button" aria-expanded={editing === g.id} onClick={() => setEditing(editing === g.id ? null : g.id)}>
          Editar
        </button>
        <ConfirmButton
          variant="danger"
          label="Excluir"
          confirmLabel="Confirmar exclusão"
          question={`Excluir ${g.parent_id ? "o subgrupo" : "o grupo"} "${g.name}"?`}
          disabled={blocked !== null}
          onConfirm={async () => {
            await adminApi.groups.remove(g.id);
            reload();
          }}
        />
        {blocked && <small className="muted">{blocked}</small>}
      </>
    );

  const editForm = (g: ComponentGroup) =>
    editing === g.id && (
      <GroupForm
        initial={toValues(g)}
        teams={teams}
        parents={parents.filter((p) => p.id !== g.id)}
        // Level changes need an empty group; subgroups can always move.
        canChangeParent={g.parent_id !== null || (g.component_count === 0 && g.subgroup_count === 0)}
        submitLabel="Salvar alterações"
        onSubmit={async (body) => {
          const saved = await adminApi.groups.update(g.id, body);
          reload();
          return saved;
        }}
        onCancel={() => setEditing(null)}
      />
    );

  return (
    <div className="admin-content">
      <p>
        <Link to="/admin/components">← Componentes</Link>
      </p>
      <PageHeader title="Grupos de componentes"
        info="Grupos aparecem recolhidos na página pública, com o status mais grave entre seus componentes. Subgrupos são opcionais e só há dois níveis. Para incluir um componente, edite-o em Componentes. Só dá para excluir um grupo vazio (sem componentes e subgrupos) e só um grupo vazio vira subgrupo, ou o contrário.">
        {canManage && (
          <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
            Novo grupo
          </button>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what="criar, editar ou excluir grupos" />}
      {creating && (
        <section className="panel" aria-label="Novo grupo">
          <GroupForm
            initial={blank}
            teams={teams}
            parents={parents}
            canChangeParent={false}
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
        (tree.length === 0 ? (
          <EmptyState title="Nenhum grupo cadastrado" />
        ) : (
          <ul className="admin-list">
            {tree.map(({ group: g, subgroups }) => {
              const direct = g.component_count - subgroups.reduce((n, s) => n + s.component_count, 0);
              const blocked =
                g.component_count > 0 ? "Mova os componentes para excluir o grupo." : g.subgroup_count > 0 ? "Exclua ou mova os subgrupos para excluir o grupo." : null;
              return (
                <li key={g.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{g.name}</strong>
                      {g.description && <p className="muted">{g.description}</p>}
                      <Tags g={g} teamName={teamName} />
                    </div>
                  </div>
                  <div className="form-actions">
                    {canManage && (
                      <button
                        type="button"
                        className="secondary-button subgroup-add"
                        aria-expanded={creatingSub === g.id}
                        onClick={() => setCreatingSub(creatingSub === g.id ? null : g.id)}
                      >
                        + Subgrupo
                      </button>
                    )}
                    {actions(g, blocked)}
                  </div>
                  {editForm(g)}
                  {creatingSub === g.id && (
                    <section className="panel" aria-label={`Novo subgrupo em ${g.name}`}>
                      <GroupForm
                        initial={{ ...blank, parent_id: g.id }}
                        teams={teams}
                        parents={parents}
                        canChangeParent={false}
                        submitLabel="Criar subgrupo"
                        onSubmit={async (body) => {
                          const s = await adminApi.groups.create({ ...body, parent_id: g.id });
                          reload();
                          return s;
                        }}
                        onCancel={() => setCreatingSub(null)}
                      />
                    </section>
                  )}
                  {subgroups.length > 0 && (
                    <ul className="subgroup-list" aria-label={`Subgrupos de ${g.name}`}>
                      {subgroups.map((s) => (
                        <li key={s.id} className="subgroup-item">
                          <div className="admin-item-head">
                            <div>
                              <strong>{s.name}</strong>
                              {s.description && <p className="muted">{s.description}</p>}
                              <Tags g={s} teamName={teamName} parentTeam={g.team_id} />
                            </div>
                          </div>
                          <div className="form-actions">{actions(s, s.component_count > 0 ? "Mova os componentes para excluir o subgrupo." : null)}</div>
                          {editForm(s)}
                        </li>
                      ))}
                    </ul>
                  )}
                  {subgroups.length > 0 && direct > 0 && (
                    <p className="muted subgroup-direct">{plural(direct, "componente direto no grupo (sem subgrupo)", "componentes diretos no grupo (sem subgrupo)")}</p>
                  )}
                </li>
              );
            })}
          </ul>
        ))}
    </div>
  );
}
