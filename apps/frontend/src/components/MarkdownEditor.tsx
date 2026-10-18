import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { fromDom, toHtml } from "../lib/markdown";
import { RichText } from "./RichText";

// Rich text editor for announcements. The stored value is always the
// Markdown dialect of lib/markdown.ts; the visual mode edits an HTML view of
// it and converts back on every change, so both modes and the preview stay
// in sync and nothing but that dialect ever reaches the API.

export type EditorMode = "visual" | "markdown" | "preview";

const MODE_KEY = "dsp.editor.mode";

function savedMode(): EditorMode {
  try {
    const m = window.localStorage.getItem(MODE_KEY);
    if (m === "visual" || m === "markdown") return m;
  } catch {
    // Storage unavailable: use the default.
  }
  return "visual";
}

type Inline = "bold" | "italic" | "underline" | "strike" | "mark" | "code";
type BlockKind = "p" | "h2" | "h3" | "small" | "quote" | "ul" | "ol";

const MD_MARK: Record<Inline, string> = { bold: "**", italic: "*", underline: "++", strike: "~~", mark: "==", code: "`" };
const MD_PREFIX: Record<Exclude<BlockKind, "p">, (i: number) => string> = {
  h2: () => "## ",
  h3: () => "### ",
  small: () => "-# ",
  quote: () => "> ",
  ul: () => "- ",
  ol: (i) => `${i + 1}. `,
};
const ANY_PREFIX = /^(#{1,3}\s+|-#\s+|>\s?|[-*]\s+|\d{1,3}[.)]\s+)/;

interface Props {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  maxLength?: number;
  invalid?: boolean;
  describedBy?: string;
  rows?: number;
}

export function MarkdownEditor({ id, label, value, onChange, maxLength = 10000, invalid, describedBy, rows = 12 }: Props) {
  const [mode, setModeState] = useState<EditorMode>(savedMode);
  const [linkOpen, setLinkOpen] = useState(false);
  const visualRef = useRef<HTMLDivElement>(null);
  const textRef = useRef<HTMLTextAreaElement>(null);
  const emitted = useRef<string | null>(null);
  const savedRange = useRef<Range | null>(null);
  const labelId = useId();

  const setMode = (m: EditorMode) => {
    setModeState(m);
    setLinkOpen(false);
    if (m !== "preview") {
      try {
        window.localStorage.setItem(MODE_KEY, m);
      } catch {
        // Not persisted; fine.
      }
    }
  };

  // Load the value into the visual editor when entering it or when the value
  // changes from outside (template applied, form reset), not while typing.
  useEffect(() => {
    const el = visualRef.current;
    if (mode !== "visual" || !el || value === emitted.current) return;
    el.innerHTML = toHtml(value);
    emitted.current = value;
  }, [mode, value]);

  const emitVisual = () => {
    const el = visualRef.current;
    if (!el) return;
    const md = fromDom(el);
    emitted.current = md;
    onChange(md);
  };

  // ---- visual mode helpers ----

  const editorRange = (): Range | null => {
    const sel = window.getSelection();
    const el = visualRef.current;
    if (!sel || !sel.rangeCount || !el) return null;
    const r = sel.getRangeAt(0);
    return el.contains(r.commonAncestorContainer) ? r : null;
  };

  const closestIn = (node: Node | null, pred: (e: HTMLElement) => boolean): HTMLElement | null => {
    const root = visualRef.current;
    for (let n = node; n && n !== root; n = n.parentNode) {
      if (n.nodeType === Node.ELEMENT_NODE && pred(n as HTMLElement)) return n as HTMLElement;
    }
    return null;
  };

  const unwrap = (el: HTMLElement) => {
    const parent = el.parentNode!;
    while (el.firstChild) parent.insertBefore(el.firstChild, el);
    parent.removeChild(el);
  };

  const exec = (cmd: string, arg?: string) => {
    visualRef.current?.focus();
    // Not available in every environment (e.g. test DOMs).
    if (typeof document.execCommand !== "function") return;
    document.execCommand("styleWithCSS", false, "false");
    document.execCommand(cmd, false, arg);
  };

  const visualInline = (kind: Inline) => {
    const native: Partial<Record<Inline, string>> = { bold: "bold", italic: "italic", underline: "underline", strike: "strikeThrough" };
    if (native[kind]) return exec(native[kind]!);
    // <mark> and <code> have no browser command: wrap or unwrap the range.
    const range = editorRange();
    if (!range) return;
    const tag = kind === "mark" ? "MARK" : "CODE";
    const existing = closestIn(range.commonAncestorContainer, (e) => e.tagName === tag);
    if (existing) return unwrap(existing);
    if (range.collapsed) return;
    const el = document.createElement(tag.toLowerCase());
    if (kind === "code") el.textContent = range.toString();
    else el.appendChild(range.extractContents());
    if (kind === "code") range.deleteContents();
    range.insertNode(el);
    window.getSelection()?.selectAllChildren(el);
  };

  const visualBlock = (kind: BlockKind) => {
    const range = editorRange();
    const current = range && closestIn(range.startContainer, (e) => e.parentNode === visualRef.current);
    if (kind === "ul") return exec("insertUnorderedList");
    if (kind === "ol") return exec("insertOrderedList");
    if (kind === "small") {
      if (current?.classList.contains("text-small")) return current.classList.remove("text-small");
      exec("formatBlock", "<p>");
      const block = closestIn(editorRange()?.startContainer ?? null, (e) => e.parentNode === visualRef.current);
      block?.classList.add("text-small");
      return;
    }
    const tag = kind === "quote" ? "blockquote" : kind;
    exec("formatBlock", current?.tagName.toLowerCase() === tag ? "<p>" : `<${tag}>`);
    const block = closestIn(editorRange()?.startContainer ?? null, (e) => e.parentNode === visualRef.current);
    block?.classList.remove("text-small");
  };

  // ---- markdown mode helpers ----

  const replaceText = (start: number, end: number, insert: string, select: [number, number]) => {
    const el = textRef.current;
    if (!el) return;
    onChange(el.value.slice(0, start) + insert + el.value.slice(end));
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(select[0], select[1]);
    });
  };

  const mdInline = (kind: Inline) => {
    const el = textRef.current;
    if (!el) return;
    const m = MD_MARK[kind];
    const { selectionStart: s, selectionEnd: e, value: v } = el;
    // Toggle off when the selection is already wrapped.
    if (v.slice(s - m.length, s) === m && v.slice(e, e + m.length) === m) {
      return replaceText(s - m.length, e + m.length, v.slice(s, e), [s - m.length, e - m.length]);
    }
    const text = v.slice(s, e) || "texto";
    replaceText(s, e, m + text + m, [s + m.length, s + m.length + text.length]);
  };

  const mdBlock = (kind: BlockKind) => {
    const el = textRef.current;
    if (!el) return;
    const v = el.value;
    const start = v.lastIndexOf("\n", el.selectionStart - 1) + 1;
    let end = v.indexOf("\n", el.selectionEnd);
    if (end < 0) end = v.length;
    const lines = v.slice(start, end).split("\n");
    const prefix = kind === "p" ? null : MD_PREFIX[kind];
    const already = prefix && lines.every((l, i) => l.startsWith(prefix(i)));
    const next = lines.map((l, i) => {
      const bare = l.replace(ANY_PREFIX, "");
      return prefix && !already ? prefix(i) + bare : bare;
    });
    const out = next.join("\n");
    replaceText(start, end, out, [start, start + out.length]);
  };

  // ---- shared toolbar actions ----

  const inlineAction = (kind: Inline) => (mode === "visual" ? (visualInline(kind), emitVisual()) : mdInline(kind));
  const blockAction = (kind: BlockKind) => (mode === "visual" ? (visualBlock(kind), emitVisual()) : mdBlock(kind));

  const rule = () => {
    if (mode === "visual") {
      exec("insertHorizontalRule");
      emitVisual();
      return;
    }
    const el = textRef.current!;
    const insert = `\n\n---\n\n`;
    replaceText(el.selectionStart, el.selectionEnd, insert, [el.selectionStart + insert.length, el.selectionStart + insert.length]);
  };

  const clear = () => {
    if (mode === "visual") {
      exec("removeFormat");
      exec("formatBlock", "<p>");
      const range = editorRange();
      for (const tag of ["MARK", "CODE"]) {
        const el = closestIn(range?.commonAncestorContainer ?? null, (e) => e.tagName === tag);
        if (el) unwrap(el);
      }
      closestIn(editorRange()?.startContainer ?? null, (e) => e.parentNode === visualRef.current)?.classList.remove("text-small");
      emitVisual();
      return;
    }
    const el = textRef.current!;
    const { selectionStart: s, selectionEnd: e, value: v } = el;
    const plain = v
      .slice(s, e)
      .replace(/\*\*|\+\+|~~|==|`/g, "")
      .replace(/(^|[^*])\*([^*]+)\*/g, "$1$2")
      .replace(/^(#{1,3}\s+|-#\s+|>\s?)/gm, "");
    replaceText(s, e, plain, [s, s + plain.length]);
  };

  const openLink = () => {
    savedRange.current = mode === "visual" ? (editorRange()?.cloneRange() ?? null) : null;
    setLinkOpen(true);
  };

  const applyLink = (url: string) => {
    setLinkOpen(false);
    if (mode === "visual") {
      const sel = window.getSelection();
      visualRef.current?.focus();
      if (savedRange.current && sel) {
        sel.removeAllRanges();
        sel.addRange(savedRange.current);
      }
      if (savedRange.current && !savedRange.current.collapsed) exec("createLink", url);
      else exec("insertHTML", `<a href="${url.replace(/"/g, "%22")}">${url.replace(/&/g, "&amp;").replace(/</g, "&lt;")}</a>`);
      emitVisual();
      return;
    }
    const el = textRef.current!;
    const { selectionStart: s, selectionEnd: e, value: v } = el;
    const text = v.slice(s, e) || "texto do link";
    const insert = `[${text}](${url})`;
    replaceText(s, e, insert, [s + 1, s + 1 + text.length]);
  };

  const onMarkdownKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!(e.ctrlKey || e.metaKey)) return;
    const k = e.key.toLowerCase();
    const map: Record<string, Inline> = { b: "bold", i: "italic", u: "underline" };
    if (map[k]) {
      e.preventDefault();
      mdInline(map[k]);
    } else if (k === "k") {
      e.preventDefault();
      openLink();
    }
  };

  const onVisualKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      openLink();
    }
  };

  const over = value.length > maxLength;
  const editing = mode !== "preview";

  return (
    <div className={`md-editor${invalid || over ? " md-editor-invalid" : ""}`}>
      <div className="md-editor-head">
        <span id={labelId} className="md-editor-label">
          {label}
        </span>
        <div className="segmented segmented-sm" role="tablist" aria-label="Modo do editor">
          {(
            [
              ["visual", "Visual"],
              ["markdown", "Markdown"],
              ["preview", "Pré-visualizar"],
            ] as const
          ).map(([m, text]) => (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={mode === m}
              className={mode === m ? "segmented-option segmented-option-active" : "segmented-option"}
              onClick={() => setMode(m)}
            >
              {text}
            </button>
          ))}
        </div>
      </div>
      {editing && (
        <div className="md-toolbar" role="toolbar" aria-label="Formatação do texto" aria-controls={id}>
          <select
            aria-label="Tamanho do texto"
            className="md-select"
            value=""
            onMouseDown={() => (savedRange.current = mode === "visual" ? (editorRange()?.cloneRange() ?? null) : null)}
            onChange={(e) => {
              if (mode === "visual" && savedRange.current) {
                const sel = window.getSelection();
                sel?.removeAllRanges();
                sel?.addRange(savedRange.current);
              }
              blockAction(e.target.value as BlockKind);
            }}
          >
            <option value="" disabled>
              Tamanho
            </option>
            <option value="h2">Título</option>
            <option value="h3">Subtítulo</option>
            <option value="p">Normal</option>
            <option value="small">Pequeno</option>
          </select>
          <select
            aria-label="Fonte"
            className="md-select"
            value=""
            onMouseDown={() => (savedRange.current = mode === "visual" ? (editorRange()?.cloneRange() ?? null) : null)}
            onChange={(e) => {
              if (mode === "visual" && savedRange.current) {
                const sel = window.getSelection();
                sel?.removeAllRanges();
                sel?.addRange(savedRange.current);
              }
              if (e.target.value === "mono") inlineAction("code");
              else clearMono();
            }}
          >
            <option value="" disabled>
              Fonte
            </option>
            <option value="default">Padrão</option>
            <option value="mono">Monoespaçada</option>
          </select>
          <span className="md-sep" aria-hidden="true" />
          <ToolButton label="Negrito (Ctrl+B)" onClick={() => inlineAction("bold")}>
            <b>B</b>
          </ToolButton>
          <ToolButton label="Itálico (Ctrl+I)" onClick={() => inlineAction("italic")}>
            <i>I</i>
          </ToolButton>
          <ToolButton label="Sublinhado (Ctrl+U)" onClick={() => inlineAction("underline")}>
            <u>U</u>
          </ToolButton>
          <ToolButton label="Tachado" onClick={() => inlineAction("strike")}>
            <s>S</s>
          </ToolButton>
          <ToolButton label="Destacar" onClick={() => inlineAction("mark")}>
            <mark className="md-mark-icon">A</mark>
          </ToolButton>
          <span className="md-sep" aria-hidden="true" />
          <ToolButton label="Lista com marcadores" onClick={() => blockAction("ul")}>
            <ListIcon />
          </ToolButton>
          <ToolButton label="Lista numerada" onClick={() => blockAction("ol")}>
            <ListIcon numbered />
          </ToolButton>
          <ToolButton label="Citação" onClick={() => blockAction("quote")}>
            <span className="md-quote-icon">”</span>
          </ToolButton>
          <ToolButton label="Link (Ctrl+K)" onClick={openLink}>
            <LinkIcon />
          </ToolButton>
          <ToolButton label="Linha divisória" onClick={rule}>
            <span className="md-rule-icon">—</span>
          </ToolButton>
          <span className="md-sep" aria-hidden="true" />
          <ToolButton label="Limpar formatação" onClick={clear}>
            <span className="md-clear-icon">T<sub>×</sub></span>
          </ToolButton>
        </div>
      )}
      {linkOpen && <LinkBar onApply={applyLink} onCancel={() => setLinkOpen(false)} />}
      {mode === "visual" && (
        <div
          id={id}
          ref={visualRef}
          className="md-visual rich-text"
          contentEditable
          suppressContentEditableWarning
          role="textbox"
          aria-multiline="true"
          aria-labelledby={labelId}
          aria-invalid={invalid || over ? true : undefined}
          aria-describedby={describedBy}
          style={{ minHeight: `${rows * 1.5}em` }}
          onInput={emitVisual}
          onBlur={emitVisual}
          onKeyDown={onVisualKey}
          onPaste={(e) => {
            // Pasted content enters as plain text: no foreign markup or styles.
            e.preventDefault();
            exec("insertText", e.clipboardData.getData("text/plain"));
          }}
          onDrop={(e) => e.preventDefault()}
        />
      )}
      {mode === "markdown" && (
        <textarea
          id={id}
          ref={textRef}
          className="md-textarea"
          rows={rows}
          value={value}
          aria-labelledby={labelId}
          aria-invalid={invalid || over ? true : undefined}
          aria-describedby={describedBy}
          spellCheck
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onMarkdownKey}
        />
      )}
      {mode === "preview" && (
        <div className="md-preview" aria-label="Pré-visualização">
          {value.trim() ? <RichText text={value} /> : <p className="muted">Nada para pré-visualizar.</p>}
        </div>
      )}
      <div className="md-editor-foot">
        <span className="field-hint">
          {mode === "markdown" ? (
            <>
              <code>**negrito**</code> <code>*itálico*</code> <code>++sublinhado++</code> <code>~~tachado~~</code> <code>==destaque==</code>{" "}
              <code>`mono`</code> <code>## título</code> <code>- lista</code> <code>1. lista</code> <code>&gt; citação</code>{" "}
              <code>[texto](https://…)</code>
            </>
          ) : (
            "Colar insere texto sem formatação. Use a barra para formatar."
          )}
        </span>
        <span className={over ? "field-error" : "field-hint"}>
          {value.length}/{maxLength}
        </span>
      </div>
    </div>
  );

  function clearMono() {
    if (mode === "visual") {
      const el = closestIn(editorRange()?.commonAncestorContainer ?? null, (e) => e.tagName === "CODE");
      if (el) unwrap(el);
      emitVisual();
      return;
    }
    const el = textRef.current!;
    const { selectionStart: s, selectionEnd: e, value: v } = el;
    if (v[s - 1] === "`" && v[e] === "`") replaceText(s - 1, e + 1, v.slice(s, e), [s - 1, e - 1]);
  }
}

function ToolButton({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      className="md-tool"
      aria-label={label}
      title={label}
      // Keep the selection in the editor while clicking the toolbar.
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function LinkBar({ onApply, onCancel }: { onApply: (url: string) => void; onCancel: () => void }) {
  const [url, setUrl] = useState("https://");
  const [error, setError] = useState<string | null>(null);
  const inputId = useId();
  const apply = () => {
    const u = url.trim();
    if (!/^https?:\/\/[^\s/]+\.[^\s]+$/i.test(u)) return setError("Informe um endereço completo, começando com https://.");
    onApply(u);
  };
  return (
    <div className="md-linkbar" role="group" aria-label="Inserir link">
      <label htmlFor={inputId}>URL do link</label>
      <input
        id={inputId}
        type="url"
        value={url}
        autoFocus
        onChange={(e) => setUrl(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            apply();
          } else if (e.key === "Escape") {
            onCancel();
          }
        }}
      />
      <button type="button" className="secondary-button" onClick={apply}>
        Aplicar
      </button>
      <button type="button" className="link-button" onClick={onCancel}>
        Cancelar
      </button>
      {error && <small className="field-error">{error}</small>}
    </div>
  );
}

function ListIcon({ numbered = false }: { numbered?: boolean }) {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
      {numbered ? (
        <g fill="currentColor" fontSize="5" fontWeight="700" fontFamily="sans-serif">
          <text x="0" y="5">1</text>
          <text x="0" y="10">2</text>
          <text x="0" y="15">3</text>
        </g>
      ) : (
        <g fill="currentColor">
          <circle cx="2" cy="3.5" r="1.4" />
          <circle cx="2" cy="8" r="1.4" />
          <circle cx="2" cy="12.5" r="1.4" />
        </g>
      )}
      <g stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
        <line x1="6" y1="3.5" x2="15" y2="3.5" />
        <line x1="6" y1="8" x2="15" y2="8" />
        <line x1="6" y1="12.5" x2="15" y2="12.5" />
      </g>
    </svg>
  );
}

function LinkIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
      <path d="M6.5 9.5l3-3" />
      <path d="M7 4.5l1.2-1.2a2.8 2.8 0 014 4L11 8.5" />
      <path d="M9 11.5l-1.2 1.2a2.8 2.8 0 01-4-4L5 7.5" />
    </svg>
  );
}
