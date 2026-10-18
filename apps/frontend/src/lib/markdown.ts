// The announcement text format: a small, safe Markdown dialect.
//
// Blocks (separated by blank lines):
//   ## Título            ### Subtítulo           -# texto pequeno
//   - item / * item      1. item                 > citação
//   ---                  (paragraph: lines joined with line breaks)
// Inline:
//   **negrito**  *itálico* or _itálico_  ++sublinhado++  ~~tachado~~
//   ==destaque==  `monoespaçado`  [texto](https://...)  bare https:// links
//   \* escapes a marker character.
//
// One parser produces a tree used by every renderer: React on the public
// page (RichText), HTML for the visual editor, and back from the editor's
// DOM to Markdown. Text never becomes HTML without escaping, and only
// http(s) links are produced.

export type Inline =
  | { t: "text"; v: string }
  | { t: "strong" | "em" | "u" | "s" | "mark"; c: Inline[] }
  | { t: "code"; v: string }
  | { t: "link"; href: string; c: Inline[] }
  | { t: "br" };

export type Block =
  | { t: "p" | "h2" | "h3" | "small" | "quote"; c: Inline[] }
  | { t: "ul" | "ol"; items: Inline[][] }
  | { t: "hr" };

const ESCAPABLE = "\\*_~=+`[]#->.)";

// Order matters: code and links first (their content is not re-parsed for
// markers the same way), then the paired markers.
const INLINE =
  /\\([\\*_~=+`[\]#>.)-])|`([^`\n]+)`|\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|\*\*(.+?)\*\*|\+\+(.+?)\+\+|~~(.+?)~~|==(.+?)==|\*([^\s*](?:[^*]*?[^\s*])?)\*|(?<![\p{L}\p{N}_])_([^\s_](?:[^_]*?[^\s_])?)_(?![\p{L}\p{N}_])|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?])/gu;

export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let text = "";
  let last = 0;
  const flush = () => {
    if (text) out.push({ t: "text", v: text });
    text = "";
  };
  for (const m of src.matchAll(INLINE)) {
    text += src.slice(last, m.index);
    last = m.index + m[0].length;
    if (m[1] !== undefined) {
      text += m[1];
      continue;
    }
    flush();
    if (m[2] !== undefined) out.push({ t: "code", v: m[2] });
    else if (m[3] !== undefined) out.push({ t: "link", href: m[4], c: parseInline(m[3]) });
    else if (m[5] !== undefined) out.push({ t: "strong", c: parseInline(m[5]) });
    else if (m[6] !== undefined) out.push({ t: "u", c: parseInline(m[6]) });
    else if (m[7] !== undefined) out.push({ t: "s", c: parseInline(m[7]) });
    else if (m[8] !== undefined) out.push({ t: "mark", c: parseInline(m[8]) });
    else if (m[9] !== undefined) out.push({ t: "em", c: parseInline(m[9]) });
    else if (m[10] !== undefined) out.push({ t: "em", c: parseInline(m[10]) });
    else out.push({ t: "link", href: m[11], c: [{ t: "text", v: m[11] }] });
  }
  text += src.slice(last);
  flush();
  return out;
}

function lineInlines(lines: string[]): Inline[] {
  const out: Inline[] = [];
  lines.forEach((l, i) => {
    if (i > 0) out.push({ t: "br" });
    out.push(...parseInline(l));
  });
  return out;
}

const BULLET = /^\s*[-*]\s+/;
const ORDERED = /^\s*\d{1,3}[.)]\s+/;

export function parse(src: string): Block[] {
  const blocks: Block[] = [];
  const chunks = src.replace(/\r\n?/g, "\n").trim().split(/\n\s*\n/);
  for (const chunk of chunks) {
    if (!chunk.trim()) continue;
    // A chunk may mix single-line blocks (headings, rules) with text; split
    // those out so "## Título\ntexto" works as expected.
    let para: string[] = [];
    const flushPara = () => {
      if (para.length) blocks.push(classify(para));
      para = [];
    };
    for (const line of chunk.split("\n")) {
      const h = /^(#{1,3})\s+(.*)$/.exec(line);
      if (h) {
        flushPara();
        blocks.push({ t: h[1].length <= 2 ? "h2" : "h3", c: parseInline(h[2].trim()) });
      } else if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
        flushPara();
        blocks.push({ t: "hr" });
      } else {
        para.push(line);
      }
    }
    flushPara();
  }
  return blocks;
}

function classify(lines: string[]): Block {
  if (lines.every((l) => BULLET.test(l))) return { t: "ul", items: lines.map((l) => parseInline(l.replace(BULLET, ""))) };
  if (lines.every((l) => ORDERED.test(l))) return { t: "ol", items: lines.map((l) => parseInline(l.replace(ORDERED, ""))) };
  if (lines.every((l) => /^\s*>\s?/.test(l))) return { t: "quote", c: lineInlines(lines.map((l) => l.replace(/^\s*>\s?/, ""))) };
  if (lines.every((l) => /^-#\s+/.test(l))) return { t: "small", c: lineInlines(lines.map((l) => l.replace(/^-#\s+/, ""))) };
  return { t: "p", c: lineInlines(lines) };
}

// ---- HTML (visual editor) ----

const escapeHtml = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

const INLINE_TAG = { strong: "strong", em: "em", u: "u", s: "s", mark: "mark" } as const;

function inlineHtml(nodes: Inline[]): string {
  return nodes
    .map((n) => {
      switch (n.t) {
        case "text":
          return escapeHtml(n.v);
        case "br":
          return "<br>";
        case "code":
          return `<code>${escapeHtml(n.v)}</code>`;
        case "link":
          return `<a href="${escapeHtml(n.href)}">${inlineHtml(n.c)}</a>`;
        default:
          return `<${INLINE_TAG[n.t]}>${inlineHtml(n.c)}</${INLINE_TAG[n.t]}>`;
      }
    })
    .join("");
}

export function toHtml(src: string): string {
  const blocks = parse(src);
  if (!blocks.length) return "<p><br></p>";
  return blocks
    .map((b) => {
      switch (b.t) {
        case "hr":
          return "<hr>";
        case "ul":
        case "ol":
          return `<${b.t}>${b.items.map((i) => `<li>${inlineHtml(i) || "<br>"}</li>`).join("")}</${b.t}>`;
        case "quote":
          return `<blockquote>${inlineHtml(b.c)}</blockquote>`;
        case "small":
          return `<p class="text-small">${inlineHtml(b.c)}</p>`;
        default:
          return `<${b.t}>${inlineHtml(b.c) || "<br>"}</${b.t}>`;
      }
    })
    .join("");
}

// ---- DOM → Markdown (visual editor output) ----

/** Escapes characters that would otherwise start formatting. */
export function escapeText(s: string): string {
  return s.replace(/[\\*`[\]]|~~|==|\+\+/g, (m) => m.split("").map((c) => (ESCAPABLE.includes(c) ? `\\${c}` : c)).join(""));
}

function wrap(marker: string, inner: string, closing = marker): string {
  // Markers cannot hug whitespace; keep spaces outside of them.
  const m = /^(\s*)([\s\S]*?)(\s*)$/.exec(inner)!;
  return m[2] ? `${m[1]}${marker}${m[2]}${closing}${m[3]}` : inner;
}

function inlineMd(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return escapeText((node.textContent ?? "").replace(/ /g, " "));
  if (node.nodeType !== Node.ELEMENT_NODE) return "";
  const el = node as HTMLElement;
  const inner = () => Array.from(el.childNodes).map(inlineMd).join("");
  const style = el.getAttribute("style") ?? "";
  switch (el.tagName) {
    case "BR":
      return "\n";
    case "B":
    case "STRONG":
      return wrap("**", inner());
    case "I":
    case "EM":
      return wrap("*", inner());
    case "U":
    case "INS":
      return wrap("++", inner());
    case "S":
    case "STRIKE":
    case "DEL":
      return wrap("~~", inner());
    case "MARK":
      return wrap("==", inner());
    case "CODE":
    case "TT":
      return (el.textContent ?? "").trim() ? `\`${(el.textContent ?? "").replace(/`/g, "")}\`` : "";
    case "A": {
      const href = el.getAttribute("href") ?? "";
      const label = inner().trim();
      if (!/^https?:\/\/\S+$/i.test(href) || !label) return inner();
      const safe = href.replace(/\)/g, "%29");
      return label.replace(/\\/g, "") === href ? safe : `[${label}](${safe})`;
    }
    default: {
      // Inline styles from browser commands (styleWithCSS) or pasted text.
      let out = inner();
      if (/font-weight:\s*(bold|[6-9]00)/.test(style)) out = wrap("**", out);
      if (/font-style:\s*italic/.test(style)) out = wrap("*", out);
      if (/text-decoration[^;]*underline/.test(style)) out = wrap("++", out);
      if (/text-decoration[^;]*line-through/.test(style)) out = wrap("~~", out);
      return out;
    }
  }
}

const BLOCK_TAGS = new Set(["P", "DIV", "H1", "H2", "H3", "H4", "H5", "H6", "UL", "OL", "BLOCKQUOTE", "HR", "PRE", "LI"]);

function blockMd(el: HTMLElement): string[] {
  const text = (nodes: Node[]) =>
    nodes
      .map(inlineMd)
      .join("")
      .split("\n")
      .map((l) => l.replace(/\s+$/, ""))
      .join("\n")
      .trim();
  switch (el.tagName) {
    case "H1":
    case "H2": {
      const t = text(Array.from(el.childNodes)).replace(/\n+/g, " ");
      return t ? [`## ${t}`] : [];
    }
    case "H3":
    case "H4":
    case "H5":
    case "H6": {
      const t = text(Array.from(el.childNodes)).replace(/\n+/g, " ");
      return t ? [`### ${t}`] : [];
    }
    case "HR":
      return ["---"];
    case "UL":
    case "OL": {
      const items = Array.from(el.children)
        .filter((c) => c.tagName === "LI")
        .map((li) => text(Array.from(li.childNodes)).replace(/\n+/g, " "))
        .filter(Boolean);
      return items.length ? [items.map((t, i) => (el.tagName === "OL" ? `${i + 1}. ${t}` : `- ${t}`)).join("\n")] : [];
    }
    case "BLOCKQUOTE": {
      const inner = containerMd(el).join("\n\n");
      return inner ? [inner.split("\n").map((l) => `> ${l}`.trimEnd()).join("\n")] : [];
    }
    case "PRE": {
      const t = (el.textContent ?? "").trim();
      return t ? [t.split("\n").map((l) => (l.trim() ? `\`${l.replace(/`/g, "")}\`` : "")).join("\n")] : [];
    }
    default: {
      // P / DIV: a div may itself contain blocks (pasted or nested markup).
      if (Array.from(el.children).some((c) => BLOCK_TAGS.has(c.tagName))) return containerMd(el);
      const t = text(Array.from(el.childNodes));
      if (!t) return [];
      if (el.classList.contains("text-small")) return [t.split("\n").map((l) => `-# ${l}`).join("\n")];
      return [protectBlockStart(t)];
    }
  }
}

/** Keeps a paragraph that starts like a block marker from becoming one. */
function protectBlockStart(t: string): string {
  return t
    .split("\n")
    .map((l) => {
      const o = /^(\s*\d{1,3})([.)]\s.*)$/.exec(l);
      if (o) return `${o[1]}\\${o[2]}`;
      return /^\s*([-*]\s|>|#{1,3}\s|-#\s|(-{3,}|\*{3,}|_{3,})\s*$)/.test(l) ? `\\${l.trimStart()}` : l;
    })
    .join("\n");
}

function containerMd(root: HTMLElement): string[] {
  const out: string[] = [];
  let inline: Node[] = [];
  const flushInline = () => {
    if (inline.length) {
      const p = document.createElement("p");
      inline.forEach((n) => p.appendChild(n.cloneNode(true)));
      out.push(...blockMd(p));
    }
    inline = [];
  };
  for (const node of Array.from(root.childNodes)) {
    if (node.nodeType === Node.ELEMENT_NODE && BLOCK_TAGS.has((node as HTMLElement).tagName)) {
      flushInline();
      out.push(...blockMd(node as HTMLElement));
    } else {
      inline.push(node);
    }
  }
  flushInline();
  return out;
}

/** Converts the visual editor's DOM back to the Markdown dialect. */
export function fromDom(root: HTMLElement): string {
  return containerMd(root).join("\n\n");
}

/** Plain text without formatting markers, for excerpts and previews. */
export function plainText(src: string): string {
  const inline = (nodes: Inline[]): string =>
    nodes.map((n) => (n.t === "text" || n.t === "code" ? n.v : n.t === "br" ? " " : inline(n.c))).join("");
  // List markers inside mixed paragraphs read better as bullets too.
  return parse(src.replace(/^\s*[-*]\s+(?=\S)/gm, (m, off: number) => (off === 0 || src[off - 1] === "\n" ? "• " : m)))
    .map((b) => {
      if (b.t === "hr") return "";
      if ("items" in b) return b.items.map((i) => `• ${inline(i)}`).join(" ");
      return inline(b.c);
    })
    .filter(Boolean)
    .join(" ")
    .replace(/\s+/g, " ")
    .trim();
}
