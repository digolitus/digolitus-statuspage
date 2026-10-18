import { useId, useState, type ReactNode } from "react";
import { errorMessage, ApiError } from "../api/client";
import type { Pagination } from "../api/types";
import { toneIcon, type Tone } from "../lib/format";

/** Status indicator: icon + text + colour, never colour alone. */
export function Badge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className={`badge badge-${tone}`}>
      <span className="badge-icon" aria-hidden="true">
        {toneIcon[tone]}
      </span>
      {children}
    </span>
  );
}

export function Loading({ label = "Carregando…" }: { label?: string }) {
  return (
    <p className="loading" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" />
      {label}
    </p>
  );
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty-state">
      <span className="empty-state-icon" aria-hidden="true">
        —
      </span>
      <div>
        <strong>{title}</strong>
        {children && <p>{children}</p>}
      </div>
    </div>
  );
}

export function ErrorPanel({ error, onRetry, title = "Não foi possível carregar os dados" }: { error: unknown; onRetry?: () => void; title?: string }) {
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  return (
    <div className="error-panel" role="alert">
      <div>
        <strong>{title}</strong>
        <span>{errorMessage(error)}</span>
        {requestId && <small>ID da requisição: {requestId}</small>}
      </div>
      {onRetry && (
        <button type="button" className="secondary-button" onClick={onRetry}>
          Tentar novamente
        </button>
      )}
    </div>
  );
}

export function Notice({ tone = "info", children }: { tone?: "info" | "warn" | "success"; children: ReactNode }) {
  return (
    <div className={`notice notice-${tone}`} role={tone === "success" ? "status" : undefined}>
      {children}
    </div>
  );
}

export function Pager({ pagination, onChange }: { pagination: Pagination; onChange: (offset: number) => void }) {
  const { limit, offset, total } = pagination;
  if (total <= limit) {
    return null;
  }
  const page = Math.floor(offset / limit) + 1;
  const pages = Math.ceil(total / limit);
  return (
    <nav className="pager" aria-label="Paginação">
      <button type="button" className="secondary-button" disabled={offset === 0} onClick={() => onChange(Math.max(0, offset - limit))}>
        Anterior
      </button>
      <span>
        Página {page} de {pages}
      </span>
      <button type="button" className="secondary-button" disabled={offset + limit >= total} onClick={() => onChange(offset + limit)}>
        Próxima
      </button>
    </nav>
  );
}

/**
 * A button that asks for explicit confirmation before running an
 * important action. Keeps focus flow simple and testable (no window.confirm).
 */
export function ConfirmButton({
  label,
  confirmLabel,
  question,
  onConfirm,
  disabled,
  variant = "primary",
}: {
  label: string;
  confirmLabel: string;
  question: string;
  onConfirm: () => void | Promise<void>;
  disabled?: boolean;
  variant?: "primary" | "danger" | "secondary";
}) {
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!asking) {
    return (
      <span className="confirm-wrap">
        <button type="button" className={`${variant}-button`} disabled={disabled} onClick={() => setAsking(true)}>
          {label}
        </button>
        {error && (
          <span className="field-error" role="alert">
            {error}
          </span>
        )}
      </span>
    );
  }
  return (
    <div className="confirm" role="group" aria-label={question}>
      <span>{question}</span>
      <button
        type="button"
        className={`${variant}-button`}
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          setError(null);
          try {
            await onConfirm();
          } catch (e) {
            setError(errorMessage(e));
          } finally {
            setBusy(false);
            setAsking(false);
          }
        }}
      >
        {busy ? "Aguarde…" : confirmLabel}
      </button>
      <button type="button" className="secondary-button" disabled={busy} onClick={() => setAsking(false)}>
        Cancelar
      </button>
    </div>
  );
}

/** Labelled form field with an accessible error message. */
export function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  children: (props: { id: string; "aria-invalid"?: boolean; "aria-describedby"?: string }) => ReactNode;
}) {
  const id = useId();
  const describedBy = [error ? `${id}-error` : "", hint ? `${id}-hint` : ""].filter(Boolean).join(" ") || undefined;
  return (
    <div className={`field${error ? " field-invalid" : ""}`}>
      <label htmlFor={id}>{label}</label>
      {children({ id, "aria-invalid": error ? true : undefined, "aria-describedby": describedBy })}
      {hint && (
        <small id={`${id}-hint`} className="field-hint">
          {hint}
        </small>
      )}
      {error && (
        <small id={`${id}-error`} className="field-error">
          {error}
        </small>
      )}
    </div>
  );
}

export interface TimelineEntry {
  id: string;
  label: string;
  tone: Tone;
  message: string | null;
  at: string;
  extra?: ReactNode;
}

/** Ordered list of published updates, newest first. */
export function Timeline({ entries, formatAt }: { entries: TimelineEntry[]; formatAt: (iso: string) => string }) {
  if (entries.length === 0) {
    return <p className="muted">Nenhuma atualização publicada.</p>;
  }
  return (
    <ol className="timeline">
      {entries.map((e) => (
        <li key={e.id} className="timeline-entry">
          <div className="timeline-head">
            <Badge tone={e.tone}>{e.label}</Badge>
            <time dateTime={e.at}>{formatAt(e.at)}</time>
          </div>
          {e.message && <p className="timeline-message">{e.message}</p>}
          {e.extra}
        </li>
      ))}
    </ol>
  );
}
