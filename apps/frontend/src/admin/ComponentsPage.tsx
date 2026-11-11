import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Component, ComponentCreate, ComponentGroup, ComponentStatus, Monitor, Team } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { StatusSelect } from "../components/StatusSelect";
import { componentStatusLabel, toneForStatus } from "../lib/format";
import { groupPath, groupTree } from "../lib/groups";
import { Link, useLocation } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, TeamSelect, useSubmit, useTeams } from "./shared";

const STATUSES = Object.keys(componentStatusLabel) as ComponentStatus[];

/** Tags summarising the monitors linked to a component (see MonitoringPage). */
function MonitorTags({ monitors }: { monitors: Monitor[] }) {
  if (monitors.length === 0) return null;
  const down = monitors.filter((m) => m.state === "down");
  return (
    <>
      <Link to="/admin/monitoring" className="tag" title={monitors.map((m) => m.name || m.external_id).join(", ")}>
        {monitors.length === 1 ? "1 monitor" : `${monitors.length} monitores`}
      </Link>
      {down.map((m) => (
        <span key={m.id} className="tag tag-warn">
          Monitor “{m.name || m.external_id}” fora do ar: exibido como {componentStatusLabel[m.down_status].toLowerCase()}
        </span>
      ))}
    </>
  );
}

type FormValues = {
  name: string;
  slug: string;
  description: string;
  show_description: boolean;
  status: ComponentStatus;
  is_public: boolean;
  display_order: string;
  group_id: string;
  subgroup_id: string;
  url: string;
  team_id: string;
};

const empty: FormValues = { name: "", slug: "", description: "", show_description: false, status: "operational", is_public: true, display_order: "0", group_id: "", subgroup_id: "", url: "", team_id: "" };

function toValues(c: Component): FormValues {
  return {
    name: c.name,
    slug: c.slug,
    description: c.description,
    show_description: c.show_description,
    status: c.status,
    is_public: c.is_public,
    display_order: String(c.display_order),
    group_id: c.group_id ?? "",
    subgroup_id: c.subgroup_id ?? "",
    url: c.url ?? "",
    team_id: c.team_id ?? "",
  };
}

function ComponentForm({
  initial,
  groups,
  teams,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: FormValues;
  groups: ComponentGroup[];
  teams: Team[];
  submitLabel: string;
  onSubmit: (input: ComponentCreate) => Promise<unknown>;
  onCancel?: () => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const set = <K extends keyof FormValues>(k: K, value: FormValues[K]) => setV((s) => ({ ...s, [k]: value }));
  const tree = groupTree(groups);
  const subgroups = tree.find((n) => n.group.id === v.group_id)?.subgroups ?? [];

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    const order = Number(v.display_order);
    if (!Number.isInteger(order) || order < 0 || order > 100000) errors.display_order = "Informe um inteiro entre 0 e 100000.";
    if (v.url.trim() && !/^https?:\/\/\S+$/i.test(v.url.trim())) errors.url = "Informe um endereço completo, começando com https:// ou http://.";
    if (Object.keys(errors).length) {
      submit.setFieldErrors(errors);
      return;
    }
    const input: ComponentCreate = {
      name: v.name.trim(),
      description: v.description.trim(),
      show_description: v.show_description,
      status: v.status,
      is_public: v.is_public,
      display_order: order,
      group_id: v.group_id || null,
      subgroup_id: v.subgroup_id || null,
      url: v.url.trim() || null,
      team_id: v.team_id || null,
    };
    if (v.slug.trim()) input.slug = v.slug.trim();
    const ok = await submit.run(() => onSubmit(input), "Componente salvo.");
    if (ok && initial === empty) setV(empty);
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate>
      <div className="form-grid form-grid-3">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field label="Slug" error={fe.slug} hint="Opcional na criação; derivado do nome.">
          {(p) => <input {...p} value={v.slug} maxLength={80} onChange={(e) => set("slug", e.target.value)} />}
        </Field>
        <Field label="Status declarado" error={fe.status}>
          {(p) => (
            <StatusSelect {...p} value={v.status} options={STATUSES} labels={componentStatusLabel} onChange={(x) => set("status", x as ComponentStatus)} />
          )}
        </Field>
        <Field label="Grupo" error={fe.group_id} hint="Opcional. Grupos aparecem recolhidos na página pública.">
          {(p) => (
            // A subgroup belongs to one group: changing the group clears it.
            <select {...p} value={v.group_id} onChange={(e) => setV((s) => ({ ...s, group_id: e.target.value, subgroup_id: "" }))}>
              <option value="">Sem grupo</option>
              {tree.map(({ group: g }) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </select>
          )}
        </Field>
        {subgroups.length > 0 && (
          <Field label="Subgrupo" error={fe.subgroup_id} hint="Opcional. Lista só os subgrupos do grupo escolhido.">
            {(p) => (
              <select {...p} value={v.subgroup_id} onChange={(e) => set("subgroup_id", e.target.value)}>
                <option value="">Nenhum (direto no grupo)</option>
                {subgroups.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.name}
                  </option>
                ))}
              </select>
            )}
          </Field>
        )}
        <TeamSelect
          teams={teams}
          value={v.team_id}
          onChange={(x) => set("team_id", x)}
          permission="components.manage"
          error={fe.team_id}
          hint="Opcional. Sem time, vale o time do subgrupo ou do grupo."
          emptyLabel={(() => {
            const sub = groups.find((x) => x.id === v.subgroup_id);
            const g = groups.find((x) => x.id === v.group_id);
            const from = sub?.team_id ? sub : g;
            const t = from?.team_id ? teams.find((x) => x.id === from.team_id) : undefined;
            return t ? `Herdar ${from === sub ? "do subgrupo" : "do grupo"} (${t.name})` : "Sem time responsável";
          })()}
        />
        <Field label="Ordem de exibição" error={fe.display_order}>
          {(p) => <input {...p} type="number" min={0} max={100000} value={v.display_order} onChange={(e) => set("display_order", e.target.value)} />}
        </Field>
      </div>
      <Field
        label="Link do componente"
        error={fe.url}
        hint="Opcional. Página, documentação ou endpoint público; aparece como um link discreto na página pública."
      >
        {(p) => (
          <input {...p} type="url" inputMode="url" placeholder="https://docs.exemplo.com/api" maxLength={500} value={v.url} onChange={(e) => set("url", e.target.value)} />
        )}
      </Field>
      <div className="field-group">
        <Field label="Descrição" error={fe.description}>
          {(p) => <textarea {...p} rows={2} maxLength={1000} value={v.description} onChange={(e) => set("description", e.target.value)} />}
        </Field>
        <label className="checkbox">
          <input type="checkbox" checked={v.show_description} onChange={(e) => set("show_description", e.target.checked)} />
          <span>Exibir a descrição na página pública</span>
        </label>
        <small className="field-hint">Fica num ícone “i” discreto ao lado do componente. Desmarcada, a descrição é só interna.</small>
      </div>
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

/** Status-only change, for roles with components.status but not components.manage. */
function StatusQuickChange({ component, onSaved }: { component: Component; onSaved: () => void }) {
  const [status, setStatus] = useState<ComponentStatus>(component.status);
  const submit = useSubmit();
  return (
    <div className="status-quick">
      <Field label={`Status de ${component.name}`}>
        {(p) => <StatusSelect {...p} value={status} options={STATUSES} labels={componentStatusLabel} onChange={(x) => setStatus(x as ComponentStatus)} />}
      </Field>
      <button
        type="button"
        className="secondary-button"
        disabled={submit.busy || status === component.status}
        onClick={async () => {
          if (await submit.run(() => adminApi.components.update(component.id, { status }), "Status atualizado.")) onSaved();
        }}
      >
        Salvar status
      </button>
      <FormStatus error={submit.formError} success={submit.success} />
    </div>
  );
}

export function ComponentsPage() {
  const [filter, setFilter] = useState<"active" | "inactive" | "all">("active");
  const [offset, setOffset] = useState(0);
  const [editing, setEditing] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [groupFilter, setGroupFilter] = useState("");
  const initialTeam = useLocation().search.get("team") ?? "";
  const [teamFilter, setTeamFilter] = useState(initialTeam);
  const isActive = filter === "all" ? undefined : filter === "active";
  const { state, reload } = useApi(
    (signal) =>
      adminApi.components.list(
        {
          is_active: isActive,
          // Filter values: a group id, or "sub:<id>" for a subgroup.
          group_id: groupFilter && !groupFilter.startsWith("sub:") ? groupFilter : undefined,
          subgroup_id: groupFilter.startsWith("sub:") ? groupFilter.slice(4) : undefined,
          team_id: teamFilter || undefined,
          limit: 50,
          offset,
        },
        { signal },
      ),
    [filter, groupFilter, teamFilter, offset],
  );
  const teams = useTeams();
  const teamName = (id: string | null) => teams.find((t) => t.id === id)?.name;
  const groupsState = useApi((signal) => adminApi.groups.all({ signal }), []).state;
  const groups = groupsState.status === "success" ? groupsState.data : [];
  const tree = groupTree(groups);
  // One request for all linked monitors; reloaded with the list.
  const monitorsState = useApi((signal) => adminApi.monitoring.monitors({}, { signal }), [state.status]).state;
  const monitorsOf = (cid: string) => (monitorsState.status === "success" ? monitorsState.data.data.filter((m) => m.component?.id === cid) : []);
  const { me, can, canFor, teamOnly } = useAuth();
  const canManage = can("components.manage");
  const canStatus = can("components.status");
  const scoped = teamOnly("components.manage") || (!canManage && teamOnly("components.status"));

  return (
    <div className="admin-content">
      <PageHeader title="Componentes">
        <Link className="secondary-button" to="/admin/component-groups">
          Grupos e subgrupos
        </Link>
        {canManage && (
          <button type="button" className="primary-button" onClick={() => setCreating((c) => !c)} aria-expanded={creating}>
            Novo componente
          </button>
        )}
      </PageHeader>
      {!canManage && <ReadOnlyNote what={canStatus ? "cadastrar ou editar componentes; você pode alterar o status" : "alterar componentes"} />}
      {scoped && (
        <p className="read-only-note" role="note">
          Seu papel vale só para os componentes dos seus times ({me.user.teams.map((t) => t.name).join(", ") || "nenhum time"}).
        </p>
      )}
      <p className="muted">
        O status de um componente é declarado manualmente. Incidentes ativos e manutenções em andamento podem agravar o status
        exibido publicamente, conforme a política documentada na API.
      </p>
      {creating && (
        <section className="panel" aria-label="Novo componente">
          <ComponentForm
            initial={empty}
            groups={groups}
            teams={teams}
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
      <div className="toolbar">
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
      {teams.length > 0 && (
        <label className="inline-filter">
          Time{" "}
          <select
            value={teamFilter}
            onChange={(e) => {
              setTeamFilter(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">Todos</option>
            {teams.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </label>
      )}
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
            {tree.map(({ group: g, subgroups }) => [
              <option key={g.id} value={g.id}>
                {g.name}
              </option>,
              ...subgroups.map((s) => (
                <option key={s.id} value={`sub:${s.id}`}>
                  {`\u00a0\u00a0· ${s.name}`}
                </option>
              )),
            ])}
          </select>
        </label>
      )}
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum componente encontrado" />
        ) : (
          <>
            <ul className="admin-list">
              {state.data.data.map((c) => {
                const manageThis = canFor("components.manage", c.owner_team_id);
                const statusThis = canFor("components.status", c.owner_team_id);
                return (
                <li key={c.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <strong>{c.name}</strong> <code>{c.slug}</code>
                      <div className="admin-item-tags">
                        {c.group_id && <span className="tag">Grupo: {groupPath(groups, c.group_id, c.subgroup_id) ?? "…"}</span>}
                        {c.owner_team_id && (
                          <span className="tag tag-team" title={c.team_id ? "Time responsável" : c.subgroup_id ? "Herdado do subgrupo ou do grupo" : "Herdado do grupo"}>
                            Time: {teamName(c.owner_team_id) ?? "…"}
                            {!c.team_id && (groups.find((g) => g.id === c.subgroup_id)?.team_id ? " (subgrupo)" : " (grupo)")}
                          </span>
                        )}
                        {!c.is_public && <span className="tag">Oculto</span>}
                        {!c.is_active && <span className="tag">Desativado</span>}
                        <span className="tag">Ordem {c.display_order}</span>
                        {c.url && <span className="tag">Com link</span>}
                        {c.description && <span className="tag">{c.show_description ? "Descrição pública" : "Descrição interna"}</span>}
                        <MonitorTags monitors={monitorsOf(c.id)} />
                      </div>
                    </div>
                    <Badge tone={toneForStatus[c.status]}>{componentStatusLabel[c.status]}</Badge>
                  </div>
                  {!manageThis && statusThis && c.is_active && <StatusQuickChange component={c} onSaved={reload} />}
                  {manageThis && (
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
                  )}
                  {editing === c.id && (
                    <ComponentForm
                      initial={toValues(c)}
                      groups={groups}
                      teams={teams}
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
                );
              })}
            </ul>
            <Pager pagination={state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}
