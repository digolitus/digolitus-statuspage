import { useId, useState, type ReactNode } from "react";
import { ApiError, errorMessage } from "../api/client";
import { adminApi } from "../api/endpoints";
import type { Component, PermissionKey, Team } from "../api/types";
import { Field, Loading, Notice } from "../components/ui";
import { useAuth } from "./authContext";
import { useApi } from "../lib/useApi";

/** Tracks submission state and maps API errors to fields. */
export function useSubmit() {
  const [busy, setBusy] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  async function run<T>(action: () => Promise<T>, successMessage?: string): Promise<T | undefined> {
    setBusy(true);
    setFormError(null);
    setSuccess(null);
    setFieldErrors({});
    try {
      const result = await action();
      if (successMessage) setSuccess(successMessage);
      return result;
    } catch (error) {
      if (error instanceof ApiError && error.details.length > 0) {
        setFieldErrors(error.fieldErrors());
      }
      setFormError(errorMessage(error));
      return undefined;
    } finally {
      setBusy(false);
    }
  }

  return { busy, fieldErrors, formError, success, run, setFieldErrors, setSuccess };
}

/** Result banner for a form. */
export function FormStatus({ error, success }: { error: string | null; success: string | null }) {
  return (
    <>
      {error && (
        <div className="error-panel" role="alert">
          <div>
            <strong>Não foi possível salvar</strong>
            <span>{error}</span>
          </div>
        </div>
      )}
      {success && <Notice tone="success">{success}</Notice>}
    </>
  );
}

/**
 * Checkbox list of components. Only active components can be added; ones
 * already associated stay listed (and selectable) even if inactive.
 */
export function ComponentPicker({
  selected,
  onChange,
  keep = [],
  error,
  legend = "Componentes afetados",
  permission,
}: {
  selected: string[];
  onChange: (ids: string[]) => void;
  keep?: { id: string; name: string }[];
  error?: string;
  legend?: string;
  /** With a team-scoped grant, only the user's components are offered. */
  permission?: PermissionKey;
}) {
  const { canFor, teamOnly } = useAuth();
  const { state } = useApi((signal) => adminApi.components.list({ is_active: true, limit: 100 }, { signal }), []);
  let options: { id: string; name: string }[] = [];
  if (state.status === "success") {
    options = state.data.data
      .filter((c: Component) => !permission || selected.includes(c.id) || canFor(permission, c.owner_team_id))
      .map((c: Component) => ({ id: c.id, name: c.name }));
    for (const k of keep) {
      if (!options.some((o) => o.id === k.id)) options.push(k);
    }
  }
  const toggle = (id: string) => onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  return (
    <fieldset className={`field picker${error ? " field-invalid" : ""}`}>
      <legend>{legend}</legend>
      {state.status === "loading" && <Loading label="Carregando componentes…" />}
      {state.status === "error" && <small className="field-error">{errorMessage(state.error)}</small>}
      {state.status === "success" && options.length === 0 && (
        <small className="field-hint">{permission && teamOnly(permission) ? "Nenhum componente dos seus times." : "Nenhum componente ativo cadastrado."}</small>
      )}
      {permission && teamOnly(permission) && options.length > 0 && (
        <small className="field-hint">Seu papel vale só para os componentes dos seus times.</small>
      )}
      <div className="picker-options">
        {options.map((o) => (
          <label key={o.id} className="checkbox">
            <input type="checkbox" checked={selected.includes(o.id)} onChange={() => toggle(o.id)} />
            {o.name}
          </label>
        ))}
      </div>
      {error && <small className="field-error">{error}</small>}
    </fieldset>
  );
}

/** Select that applies a template to the form; nothing is created by it. */
export function TemplateSelect<T extends { id: string; name: string; guidance: string }>({
  templates,
  onApply,
  label = "Usar template",
}: {
  templates: T[];
  onApply: (template: T) => void;
  label?: string;
}) {
  const [chosen, setChosen] = useState<T | null>(null);
  const id = useId();
  return (
    <div className="template-select">
      <label htmlFor={id}>{label}</label>
      <select
          id={id}
          value={chosen?.id ?? ""}
          onChange={(e) => {
            const t = templates.find((x) => x.id === e.target.value) ?? null;
            setChosen(t);
            if (t) onApply(t);
          }}
        >
          <option value="">Sem template</option>
          {templates.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
      </select>
      {chosen?.guidance && (
        <p className="guidance">
          <strong>Orientação:</strong> {chosen.guidance}
        </p>
      )}
      {chosen && <small className="field-hint">Os campos foram preenchidos pelo template. Revise antes de publicar.</small>}
    </div>
  );
}

/** Shown on pages the user may read but not change (no permission). */
export function ReadOnlyNote({ what }: { what: string }) {
  return (
    <p className="read-only-note" role="note">
      Somente leitura: seu papel não permite {what}.
    </p>
  );
}

export function PageHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="admin-page-header">
      <h1>{title}</h1>
      {children && <div className="admin-page-actions">{children}</div>}
    </div>
  );
}

/** Minimal client-side checks; the API remains the authority. */
export function requireText(errors: Record<string, string>, field: string, value: string, max: number, label: string) {
  const v = value.trim();
  if (!v) errors[field] = `${label} é obrigatório.`;
  else if (v.length > max) errors[field] = `${label} deve ter no máximo ${max} caracteres.`;
}

/** Teams, for selects and labels (any signed-in user may list them). */
export function useTeams(): Team[] {
  const { state } = useApi((signal) => adminApi.teams.list({}, { signal }), []);
  return state.status === "success" ? state.data.data : [];
}

/**
 * Optional responsible team. Users whose permission is team-scoped only
 * see their own teams (plus the current one, so it stays visible).
 */
export function TeamSelect({
  teams,
  value,
  onChange,
  permission,
  error,
  hint,
  emptyLabel = "Sem time responsável",
}: {
  teams: Team[];
  value: string;
  onChange: (id: string) => void;
  permission: Parameters<ReturnType<typeof useAuth>["can"]>[0];
  error?: string;
  hint?: string;
  emptyLabel?: string;
}) {
  const { me, teamOnly } = useAuth();
  const mine = new Set(me.user.teams.map((t) => t.id));
  const scoped = teamOnly(permission);
  const options = teams.filter((t) => t.id === value || (t.is_active && (!scoped || mine.has(t.id))));
  return (
    <Field label="Time responsável" error={error} hint={hint ?? (scoped ? "Seu papel vale só para os seus times." : "Opcional.")}>
      {(p) => (
        <select {...p} value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">{emptyLabel}</option>
          {options.map((t) => (
            <option key={t.id} value={t.id}>
              {t.kind ? `${t.name} (${t.kind})` : t.name}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}

/**
 * Whether `permission` covers an item affecting `componentIds` (incident,
 * maintenance). Team-scoped grants need at least one component and all of
 * them owned by the user's teams; the API enforces the same rule.
 */
export function useCanForComponents(permission: PermissionKey, componentIds: string[]): boolean {
  const { can, canFor, teamOnly } = useAuth();
  const scoped = teamOnly(permission);
  const { state } = useApi(
    (signal) => (scoped ? adminApi.components.list({ limit: 100 }, { signal }) : Promise.resolve(null)),
    [scoped],
  );
  if (!can(permission)) return false;
  if (!scoped) return true;
  if (state.status !== "success" || !state.data || componentIds.length === 0) return false;
  const owners = new Map(state.data.data.map((c) => [c.id, c.owner_team_id]));
  return componentIds.every((id) => canFor(permission, owners.get(id)));
}
