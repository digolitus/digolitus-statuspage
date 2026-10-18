import { useId, useState, type ReactNode } from "react";
import { ApiError, errorMessage } from "../api/client";
import { adminApi } from "../api/endpoints";
import type { Component } from "../api/types";
import { Loading, Notice } from "../components/ui";
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
}: {
  selected: string[];
  onChange: (ids: string[]) => void;
  keep?: { id: string; name: string }[];
  error?: string;
  legend?: string;
}) {
  const { state } = useApi((signal) => adminApi.components.list({ is_active: true, limit: 100 }, { signal }), []);
  let options: { id: string; name: string }[] = [];
  if (state.status === "success") {
    options = state.data.data.map((c: Component) => ({ id: c.id, name: c.name }));
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
      {state.status === "success" && options.length === 0 && <small className="field-hint">Nenhum componente ativo cadastrado.</small>}
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
