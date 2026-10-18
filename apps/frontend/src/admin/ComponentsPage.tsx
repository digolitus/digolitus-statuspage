import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Component, ComponentCreate, ComponentGroup, ComponentStatus, Team } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { StatusSelect } from "../components/StatusSelect";
import { componentStatusLabel, toneForStatus } from "../lib/format";
import { Link, useLocation } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, requireText, TeamSelect, useSubmit, useTeams } from "./shared";

const STATUSES = Object.keys(componentStatusLabel) as ComponentStatus[];

type FormValues = {
  name: string;
  slug: string;
  description: string;
  status: ComponentStatus;
  is_public: boolean;
  display_order: string;
  group_id: string;
  url: string;
  team_id: string;
};

const empty: FormValues = { name: "", slug: "", description: "", status: "operational", is_public: true, display_order: "0", group_id: "", url: "", team_id: "" };

function toValues(c: Component): FormValues {
  return {
    name: c.name,
    slug: c.slug,
    description: c.description,
    status: c.status,
    is_public: c.is_public,
    display_order: String(c.display_order),
    group_id: c.group_id ?? "",
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
      status: v.status,
      is_public: v.is_public,
      display_order: order,
      group_id: v.group_id || null,
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
        <TeamSelect
          teams={teams}
          value={v.team_id}
          onChange={(x) => set("team_id", x)}
          permission="components.manage"
          error={fe.team_id}
          hint="Opcional. Sem time, vale o time do grupo."
          emptyLabel={(() => {
            const g = groups.find((x) => x.id === v.group_id);
            const t = g?.team_id ? teams.find((x) => x.id === g.team_id) : undefined;
            return t ? `Herdar do grupo (${t.name})` : "Sem time responsável";
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
        { is_active: isActive, group_id: groupFilter || undefined, team_id: teamFilter || undefined, limit: 50, offset },
        { signal },
      ),
    [filter, groupFilter, teamFilter, offset],
  );
  const teams = useTeams();
  const teamName = (id: string | null) => teams.find((t) => t.id === id)?.name;
  const groupsState = useApi((signal) => adminApi.groups.list({ limit: 100 }, { signal }), []).state;
  const groups = groupsState.status === "success" ? groupsState.data.data : [];
  const groupName = (id: string | null) => groups.find((g) => g.id === id)?.name;
  const { me, can, canFor, teamOnly } = useAuth();
  const canManage = can("components.manage");
  const canStatus = can("components.status");
  const scoped = teamOnly("components.manage") || (!canManage && teamOnly("components.status"));

  return (
    <div className="admin-content">
      <PageHeader title="Componentes">
        <Link className="secondary-button" to="/admin/component-groups">
          Gerenciar grupos
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
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
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
                        {c.group_id && <span className="tag">Grupo: {groupName(c.group_id) ?? "…"}</span>}
                        {c.owner_team_id && (
                          <span className="tag tag-team" title={c.team_id ? "Time responsável" : "Herdado do grupo"}>
                            Time: {teamName(c.owner_team_id) ?? "…"}
                            {!c.team_id && " (grupo)"}
                          </span>
                        )}
                        {!c.is_public && <span className="tag">Oculto</span>}
                        {!c.is_active && <span className="tag">Desativado</span>}
                        <span className="tag">Ordem {c.display_order}</span>
                        {c.url && <span className="tag">Com link</span>}
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
