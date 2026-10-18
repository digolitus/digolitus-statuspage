import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import type { ComponentStatus } from "../api/types";
import { componentStatusLabel } from "../lib/format";

/** Icon for a status: shape + colour, so colour is never the only cue. */
export function StatusIcon({ status }: { status: ComponentStatus }) {
  const paths: Record<ComponentStatus, string> = {
    operational: "M5 8.2 7 10.2 11 6",
    degraded_performance: "M4.8 8h6.4",
    partial_outage: "M8 4.6v4.2M8 11.2v.2",
    major_outage: "M5.6 5.6l4.8 4.8M10.4 5.6l-4.8 4.8",
    maintenance: "M10.2 4.3a2.3 2.3 0 0 0-2.9 2.9L4.4 10.1l1.5 1.5 2.9-2.9a2.3 2.3 0 0 0 2.9-2.9l-1.3 1.3-1.2-.3-.3-1.2z",
  };
  return (
    <svg className={`status-icon status-icon-${status}`} viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="8" />
      <path d={paths[status]} />
    </svg>
  );
}

/**
 * Select-only combobox (WAI-ARIA APG pattern) that shows the status icon of
 * each option. The trigger is a <button>, so a <label htmlFor> names it and
 * its text is the current value. Keyboard: arrows, Home/End, Enter/Space,
 * Escape; typing a letter jumps to the first matching option.
 */
export function StatusSelect<T extends ComponentStatus>({
  id,
  value,
  options,
  onChange,
  labels = componentStatusLabel,
  ...aria
}: {
  id?: string;
  value: T;
  options: readonly T[];
  onChange: (value: T) => void;
  labels?: Record<T, string>;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  const autoId = useId();
  const baseId = id ?? autoId;
  const listId = `${baseId}-listbox`;
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(() => Math.max(0, options.indexOf(value)));
  const rootRef = useRef<HTMLDivElement>(null);
  // Hover only moves the highlight on real pointer movement; a list that
  // opens under a resting pointer must not override keyboard navigation.
  const pointer = useRef({ x: -1, y: -1 });

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);

  const show = () => {
    setActive(Math.max(0, options.indexOf(value)));
    setOpen(true);
  };
  const choose = (i: number) => {
    onChange(options[i]);
    setOpen(false);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    const last = options.length - 1;
    const move = (i: number) => {
      e.preventDefault();
      if (!open) show();
      setActive(Math.min(last, Math.max(0, i)));
    };
    switch (e.key) {
      case "ArrowDown":
        return move(open ? active + 1 : options.indexOf(value));
      case "ArrowUp":
        return move(open ? active - 1 : options.indexOf(value));
      case "Home":
        return move(0);
      case "End":
        return move(last);
      case "Enter":
      case " ":
        e.preventDefault();
        if (open) choose(active);
        else show();
        return;
      case "Escape":
        if (open) {
          e.preventDefault();
          setOpen(false);
        }
        return;
      case "Tab":
        setOpen(false);
        return;
      default:
        if (e.key.length === 1 && /\S/.test(e.key)) {
          const i = options.findIndex((o) => labels[o].toLowerCase().startsWith(e.key.toLowerCase()));
          if (i >= 0) move(i);
        }
    }
  };

  return (
    <div className={`status-select${open ? " status-select-open" : ""}`} ref={rootRef}>
      <button
        type="button"
        role="combobox"
        id={baseId}
        className="status-select-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        aria-activedescendant={open ? `${baseId}-opt-${active}` : undefined}
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={onKeyDown}
        {...aria}
      >
        <StatusIcon status={value} />
        <span className="status-select-value">{labels[value]}</span>
        <span className="status-select-chevron" aria-hidden="true">
          ▾
        </span>
      </button>
      {open && (
        <ul className="status-select-list" role="listbox" id={listId} tabIndex={-1}>
          {options.map((o, i) => (
            <li
              key={o}
              id={`${baseId}-opt-${i}`}
              role="option"
              aria-selected={o === value}
              className={`status-select-option${i === active ? " is-active" : ""}`}
              onMouseMove={(e) => {
                if (e.clientX === pointer.current.x && e.clientY === pointer.current.y) return;
                pointer.current = { x: e.clientX, y: e.clientY };
                if (i !== active) setActive(i);
              }}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => choose(i)}
            >
              <StatusIcon status={o} />
              {labels[o]}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
