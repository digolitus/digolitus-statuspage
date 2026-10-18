import { useState, type FormEvent } from "react";
import { authApi, adminApi } from "../api/endpoints";
import type { PermissionInfo, PermissionKey, Role, User } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { formatDateTime, isFuture } from "../lib/format";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, requireText, useSubmit } from "./shared";

const MIN_PASSWORD = 12;

function checkPassword(errors: Record<string, string>, field: string, value: string) {
  if (value.length < MIN_PASSWORD) errors[field] = `Use pelo menos ${MIN_PASSWORD} caracteres.`;
  else if (value.length > 128) errors[field] = "Use no máximo 128 caracteres.";
}

/** The signed-in user's own account: profile summary and password change. */
export function AccountPage() {
  const { me } = useAuth();
  const [v, setV] = useState({ current: "", next: "", confirm: "" });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    if (!v.current) errors.current_password = "Informe a senha atual.";
    checkPassword(errors, "new_password", v.next);
    if (v.next !== v.confirm) errors.confirm = "As senhas não conferem.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const ok = await submit.run(async () => {
      await authApi.changePassword(v.current, v.next);
      return true;
    }, "Senha alterada. Suas outras sessões foram encerradas.");
    if (ok) setV({ current: "", next: "", confirm: "" });
  }
  const fe = submit.fieldErrors;
  return (
    <div className="admin-content">
      <PageHeader title="Minha conta" />
      <section className="panel">
        <p>
          <strong>{me.user.name}</strong> · {me.user.email}
        </p>
        <p className="muted small">
          Papéis: {me.user.roles.map((r) => r.name).join(", ") || "nenhum"}
          {me.is_admin && " · acesso total"}
        </p>
      </section>
      <section className="panel" aria-labelledby="pw-title">
        <h2 id="pw-title">Trocar senha</h2>
        <form className="form" onSubmit={handle} noValidate aria-label="Trocar senha">
          <Field label="Senha atual" error={fe.current_password}>
            {(p) => <input {...p} type="password" autoComplete="current-password" value={v.current} onChange={(e) => setV({ ...v, current: e.target.value })} />}
          </Field>
          <div className="form-grid">
            <Field label="Nova senha" error={fe.new_password} hint="12 a 128 caracteres.">
              {(p) => <input {...p} type="password" autoComplete="new-password" value={v.next} maxLength={128} onChange={(e) => setV({ ...v, next: e.target.value })} />}
            </Field>
            <Field label="Confirmar nova senha" error={fe.confirm}>
              {(p) => <input {...p} type="password" autoComplete="new-password" value={v.confirm} maxLength={128} onChange={(e) => setV({ ...v, confirm: e.target.value })} />}
            </Field>
          </div>
          <FormStatus error={submit.formError} success={submit.success} />
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={submit.busy}>
              Trocar senha
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

/** Roles the current user may grant: admins any; others only roles whose permissions they hold. */
function grantable(roles: Role[], can: (p: PermissionKey) => boolean, isAdmin: boolean): Set<string> {
  return new Set(roles.filter((r) => isAdmin || (!r.is_system && r.permissions.every((p) => can(p)))).map((r) => r.id));
}

function RolePicker({
  roles,
  selected,
  allowed,
  onChange,
  error,
}: {
  roles: Role[];
  selected: string[];
  allowed: Set<string>;
  onChange: (ids: string[]) => void;
  error?: string;
}) {
  return (
    <fieldset className={`field picker${error ? " field-invalid" : ""}`}>
      <legend>Papéis</legend>
      <div className="picker-options role-options">
        {roles.map((r) => (
          <label key={r.id} className="checkbox" title={allowed.has(r.id) ? r.description : "Você não pode conceder este papel."}>
            <input
              type="checkbox"
              checked={selected.includes(r.id)}
              disabled={!allowed.has(r.id)}
              onChange={(e) => onChange(e.target.checked ? [...selected, r.id] : selected.filter((x) => x !== r.id))}
            />
            {r.name}
            {r.is_system && <span className="tag">acesso total</span>}
          </label>
        ))}
      </div>
      {error && <small className="field-error">{error}</small>}
    </fieldset>
  );
}

function NewUserForm({ roles, allowed, onCreated }: { roles: Role[]; allowed: Set<string>; onCreated: () => void }) {
  const [v, setV] = useState({ name: "", email: "", password: "", role_ids: [] as string[] });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 120, "Nome");
    requireText(errors, "email", v.email, 254, "E-mail");
    checkPassword(errors, "password", v.password);
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const ok = await submit.run(
      () => adminApi.users.create({ name: v.name.trim(), email: v.email.trim(), password: v.password, role_ids: v.role_ids }),
      "Usuário criado. Informe a senha inicial por um canal seguro; ela será trocada no primeiro acesso.",
    );
    if (ok) {
      setV({ name: "", email: "", password: "", role_ids: [] });
      onCreated();
    }
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Novo usuário">
      <div className="form-grid">
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={120} onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        <Field label="E-mail" error={fe.email}>
          {(p) => <input {...p} type="email" autoComplete="off" value={v.email} maxLength={254} onChange={(e) => setV({ ...v, email: e.target.value })} />}
        </Field>
        <Field label="Senha inicial" error={fe.password} hint="12 a 128 caracteres.">
          {(p) => <input {...p} type="password" autoComplete="new-password" value={v.password} maxLength={128} onChange={(e) => setV({ ...v, password: e.target.value })} />}
        </Field>
      </div>
      <RolePicker roles={roles} selected={v.role_ids} allowed={allowed} onChange={(ids) => setV({ ...v, role_ids: ids })} error={fe.role_ids} />
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          Criar usuário
        </button>
      </div>
    </form>
  );
}

function UserItem({ u, roles, allowed, onChanged }: { u: User; roles: Role[]; allowed: Set<string>; onChanged: () => void }) {
  const { me } = useAuth();
  const [editing, setEditing] = useState(false);
  const [roleIds, setRoleIds] = useState(u.roles.map((r) => r.id));
  const [password, setPassword] = useState("");
  const submit = useSubmit();
  const self = u.id === me.user.id;
  const locked = isFuture(u.locked_until);
  return (
    <li className="admin-item">
      <div className="admin-item-head">
        <div>
          <strong>{u.name}</strong> <span className="muted">{u.email}</span>
          <div className="admin-item-tags">
            {u.roles.length === 0 ? <span className="tag">Sem papel (somente leitura)</span> : u.roles.map((r) => <span key={r.id} className="tag">{r.name}</span>)}
            {self && <span className="tag">Você</span>}
            {u.must_change_password && <span className="tag">Senha provisória</span>}
            {locked && <span className="tag tag-warn">Bloqueado até {formatDateTime(u.locked_until)}</span>}
          </div>
          <p className="muted small">Último acesso: {formatDateTime(u.last_login_at)}</p>
        </div>
        <Badge tone={u.is_active ? "ok" : "neutral"}>{u.is_active ? "Ativo" : "Desativado"}</Badge>
      </div>
      <div className="form-actions">
        <button type="button" className="secondary-button" aria-expanded={editing} onClick={() => setEditing((e) => !e)}>
          Editar acesso
        </button>
        {locked && (
          <button type="button" className="secondary-button" onClick={() => submit.run(() => adminApi.users.update(u.id, { unlock: true }), "Usuário desbloqueado.").then(onChanged)}>
            Desbloquear
          </button>
        )}
        {!self && (
          <ConfirmButton
            variant={u.is_active ? "danger" : "secondary"}
            label={u.is_active ? "Desativar" : "Reativar"}
            confirmLabel="Confirmar"
            question={u.is_active ? `Desativar ${u.name}? As sessões abertas serão encerradas.` : `Reativar ${u.name}?`}
            onConfirm={async () => {
              await adminApi.users.update(u.id, { is_active: !u.is_active });
              onChanged();
            }}
          />
        )}
      </div>
      {editing && (
        <div className="form">
          <RolePicker roles={roles} selected={roleIds} allowed={allowed} onChange={setRoleIds} error={submit.fieldErrors.role_ids} />
          <div className="form-actions">
            <button
              type="button"
              className="primary-button"
              disabled={submit.busy}
              onClick={() => submit.run(() => adminApi.users.update(u.id, { role_ids: roleIds }), "Papéis atualizados.").then((r) => r && onChanged())}
            >
              Salvar papéis
            </button>
          </div>
          {!self && (
            <div className="inline-form">
              <Field label="Nova senha" error={submit.fieldErrors.password} hint="Redefine a senha e encerra as sessões do usuário.">
                {(p) => <input {...p} type="password" autoComplete="new-password" value={password} maxLength={128} onChange={(e) => setPassword(e.target.value)} />}
              </Field>
              <button
                type="button"
                className="secondary-button"
                disabled={submit.busy}
                onClick={async () => {
                  const errors: Record<string, string> = {};
                  checkPassword(errors, "password", password);
                  if (Object.keys(errors).length) return submit.setFieldErrors(errors);
                  const ok = await submit.run(async () => {
                    await adminApi.users.resetPassword(u.id, password);
                    return true;
                  }, "Senha redefinida. O usuário deverá trocá-la no próximo acesso.");
                  if (ok) setPassword("");
                }}
              >
                Redefinir senha
              </button>
            </div>
          )}
          <FormStatus error={submit.formError} success={submit.success} />
        </div>
      )}
    </li>
  );
}

export function UsersPage() {
  const { me, can } = useAuth();
  const data = useApi(async (signal) => {
    const [users, roles] = await Promise.all([adminApi.users.list({ signal }), adminApi.roles.list({ signal })]);
    return { users: users.data, roles: roles.data };
  }, []);
  return (
    <div className="admin-content">
      <PageHeader title="Usuários" />
      <p className="muted">
        Cada usuário recebe um ou mais papéis; as permissões são a soma deles. Sem papel, o usuário só consulta os dados.
        Você só pode conceder papéis cujas permissões também possui.
      </p>
      {data.state.status === "loading" && <Loading />}
      {data.state.status === "error" && <ErrorPanel error={data.state.error} onRetry={data.reload} />}
      {data.state.status === "success" && (
        <>
          <section className="panel" aria-label="Novo usuário">
            <h2>Novo usuário</h2>
            <NewUserForm roles={data.state.data.roles} allowed={grantable(data.state.data.roles, can, me.is_admin)} onCreated={data.reload} />
          </section>
          {data.state.data.users.length === 0 ? (
            <EmptyState title="Nenhum usuário" />
          ) : (
            <ul className="admin-list">
              {data.state.data.users.map((u) => (
                <UserItem
                  key={`${u.id}-${u.updated_at}`}
                  u={u}
                  roles={data.state.data!.roles}
                  allowed={grantable(data.state.data!.roles, can, me.is_admin)}
                  onChanged={data.reload}
                />
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  );
}

function PermissionPicker({
  catalogue,
  selected,
  onChange,
  disabled,
}: {
  catalogue: PermissionInfo[];
  selected: PermissionKey[];
  onChange: (p: PermissionKey[]) => void;
  disabled: (p: PermissionKey) => boolean;
}) {
  const groups = [...new Set(catalogue.map((c) => c.group))];
  return (
    <div className="permission-groups">
      {groups.map((g) => (
        <fieldset key={g} className="picker">
          <legend>{g}</legend>
          {catalogue
            .filter((c) => c.group === g)
            .map((c) => (
              <label key={c.key} className="permission-option">
                <input
                  type="checkbox"
                  checked={selected.includes(c.key)}
                  disabled={disabled(c.key)}
                  onChange={(e) => onChange(e.target.checked ? [...selected, c.key] : selected.filter((x) => x !== c.key))}
                />
                <span>
                  <strong>{c.label}</strong>
                  {c.sensitive && <span className="tag tag-warn">sensível</span>}
                  <small className="muted">{c.description}</small>
                </span>
              </label>
            ))}
        </fieldset>
      ))}
    </div>
  );
}

function RoleForm({ catalogue, role, onSaved }: { catalogue: PermissionInfo[]; role?: Role; onSaved: () => void }) {
  const { can } = useAuth();
  const [v, setV] = useState({ name: role?.name ?? "", description: role?.description ?? "", permissions: (role?.permissions ?? []) as PermissionKey[] });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "name", v.name, 60, "Nome");
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const body = { name: v.name.trim(), description: v.description.trim(), permissions: v.permissions };
    const ok = await submit.run(() => (role ? adminApi.roles.update(role.id, body) : adminApi.roles.create(body)), role ? "Papel atualizado." : "Papel criado.");
    if (ok) {
      if (!role) setV({ name: "", description: "", permissions: [] });
      onSaved();
    }
  }
  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label={role ? `Editar papel ${role.name}` : "Novo papel"}>
      <div className="form-grid">
        <Field label="Nome do papel" error={fe.name}>
          {(p) => <input {...p} value={v.name} maxLength={60} placeholder="Ex.: Comunicação" onChange={(e) => setV({ ...v, name: e.target.value })} />}
        </Field>
        <Field label="Descrição" error={fe.description}>
          {(p) => <input {...p} value={v.description} maxLength={500} onChange={(e) => setV({ ...v, description: e.target.value })} />}
        </Field>
      </div>
      <PermissionPicker catalogue={catalogue} selected={v.permissions} onChange={(permissions) => setV({ ...v, permissions })} disabled={(p) => !can(p)} />
      {fe.permissions && <p className="field-error">{fe.permissions}</p>}
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {role ? "Salvar papel" : "Criar papel"}
        </button>
      </div>
    </form>
  );
}

export function RolesPage() {
  const { can, refresh } = useAuth();
  const [editing, setEditing] = useState<string | null>(null);
  const data = useApi(async (signal) => {
    const [roles, perms] = await Promise.all([adminApi.roles.list({ signal }), adminApi.permissions({ signal })]);
    return { roles: roles.data, catalogue: perms.data };
  }, []);
  const manage = can("roles.manage");
  const saved = () => {
    data.reload();
    // The signed-in user's own permissions may have changed.
    refresh();
  };
  return (
    <div className="admin-content">
      <PageHeader title="Papéis e permissões" />
      <p className="muted">
        Papéis agrupam permissões. Mudanças valem imediatamente para os usuários que têm o papel. O papel Administrador é
        fixo e tem acesso total. Permissões marcadas como sensíveis permitem mudar quem pode fazer o quê.
      </p>
      {data.state.status === "loading" && <Loading />}
      {data.state.status === "error" && <ErrorPanel error={data.state.error} onRetry={data.reload} />}
      {data.state.status === "success" && (
        <>
          {manage && (
            <section className="panel" aria-label="Novo papel">
              <h2>Novo papel</h2>
              <RoleForm catalogue={data.state.data.catalogue} onSaved={data.reload} />
            </section>
          )}
          <ul className="admin-list">
            {data.state.data.roles.map((r) => (
              <li key={r.id} className="admin-item">
                <div className="admin-item-head">
                  <div>
                    <strong>{r.name}</strong>
                    <div className="admin-item-tags">
                      <span className="tag">{r.user_count} usuário(s)</span>
                      {r.is_system ? <span className="tag">Acesso total · fixo</span> : <span className="tag">{r.permissions.length} permissão(ões)</span>}
                    </div>
                    {r.description && <p className="muted small">{r.description}</p>}
                    {!r.is_system && (
                      <p className="small">
                        {r.permissions.length === 0
                          ? "Nenhuma permissão (somente leitura)."
                          : r.permissions.map((p) => data.state.data!.catalogue.find((c) => c.key === p)?.label ?? p).join(" · ")}
                      </p>
                    )}
                  </div>
                </div>
                {manage && !r.is_system && (
                  <div className="form-actions">
                    <button type="button" className="secondary-button" aria-expanded={editing === r.id} onClick={() => setEditing(editing === r.id ? null : r.id)}>
                      Editar
                    </button>
                    {r.user_count === 0 && (
                      <ConfirmButton
                        variant="danger"
                        label="Excluir"
                        confirmLabel="Confirmar exclusão"
                        question={`Excluir o papel "${r.name}"?`}
                        onConfirm={async () => {
                          await adminApi.roles.remove(r.id);
                          data.reload();
                        }}
                      />
                    )}
                  </div>
                )}
                {editing === r.id && <RoleForm key={r.updated_at} catalogue={data.state.data!.catalogue} role={r} onSaved={saved} />}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
