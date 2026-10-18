import { useId, useState, type KeyboardEvent } from "react";

/**
 * Free-form tags as removable chips. Enter, comma or Tab adds the typed
 * tag; Backspace on an empty input removes the last one. Duplicates are
 * ignored without regard to case, mirroring the API.
 */
export function TagInput({
  label,
  value,
  onChange,
  max = 20,
  maxLength = 30,
  hint,
  error,
  suggestions = [],
}: {
  label: string;
  value: string[];
  onChange: (tags: string[]) => void;
  max?: number;
  maxLength?: number;
  hint?: string;
  error?: string;
  suggestions?: string[];
}) {
  const [draft, setDraft] = useState("");
  const id = useId();
  const listId = useId();
  const add = (raw: string) => {
    const t = raw.replace(/\s+/g, " ").trim().slice(0, maxLength);
    if (!t || value.length >= max || value.some((v) => v.toLowerCase() === t.toLowerCase())) return;
    onChange([...value, t]);
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if ((e.key === "Enter" || e.key === "," || (e.key === "Tab" && draft.trim())) && !e.nativeEvent.isComposing) {
      e.preventDefault();
      add(draft);
      setDraft("");
    } else if (e.key === "Backspace" && !draft && value.length) {
      onChange(value.slice(0, -1));
    }
  };
  const free = suggestions.filter((s) => !value.some((v) => v.toLowerCase() === s.toLowerCase()));
  return (
    <div className={`field${error ? " field-invalid" : ""}`}>
      <label htmlFor={id}>{label}</label>
      <div className="tag-input" onClick={() => document.getElementById(id)?.focus()}>
        <ul className="tag-input-list" aria-label={`${label} adicionadas`}>
          {value.map((t) => (
            <li key={t} className="tag tag-team tag-removable">
              {t}
              <button type="button" aria-label={`Remover ${t}`} onClick={() => onChange(value.filter((x) => x !== t))}>
                ×
              </button>
            </li>
          ))}
        </ul>
        <input
          id={id}
          list={free.length ? listId : undefined}
          value={draft}
          maxLength={maxLength}
          disabled={value.length >= max}
          placeholder={value.length ? "" : "Digite e tecle Enter"}
          aria-describedby={hint ? `${id}-hint` : undefined}
          onChange={(e) => {
            const v = e.target.value;
            if (v.includes(",")) {
              v.split(",").slice(0, -1).forEach(add);
              setDraft(v.split(",").pop() ?? "");
            } else setDraft(v);
          }}
          onKeyDown={onKey}
          onBlur={() => {
            if (draft.trim()) {
              add(draft);
              setDraft("");
            }
          }}
        />
        {free.length > 0 && (
          <datalist id={listId}>
            {free.map((s) => (
              <option key={s} value={s} />
            ))}
          </datalist>
        )}
      </div>
      <small id={`${id}-hint`} className="field-hint">
        {hint ?? "Enter ou vírgula para adicionar."} {value.length}/{max}
      </small>
      {error && <small className="field-error">{error}</small>}
    </div>
  );
}
