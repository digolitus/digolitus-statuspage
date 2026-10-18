import type { ReactNode } from "react";

/**
 * Renders the lightweight formatting used in announcements:
 *  - blank line → new paragraph; single line break kept;
 *  - lines starting with "- " or "* " → bullet list;
 *  - **bold**, [text](https://...) and bare https:// links.
 *
 * Everything is rendered as React text nodes (no HTML injection), and only
 * http(s) links become anchors.
 */
export function RichText({ text, className = "rich-text" }: { text: string; className?: string }) {
  const blocks = text.replace(/\r\n?/g, "\n").trim().split(/\n{2,}/);
  return (
    <div className={className}>
      {blocks.map((block, i) => {
        const lines = block.split("\n");
        if (lines.every((l) => /^\s*[-*]\s+/.test(l))) {
          return (
            <ul key={i}>
              {lines.map((l, j) => (
                <li key={j}>{inline(l.replace(/^\s*[-*]\s+/, ""))}</li>
              ))}
            </ul>
          );
        }
        return (
          <p key={i}>
            {lines.map((l, j) => (
              <span key={j}>
                {j > 0 && <br />}
                {inline(l)}
              </span>
            ))}
          </p>
        );
      })}
    </div>
  );
}

const TOKEN = /\*\*(.+?)\*\*|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?])/g;

function inline(line: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let k = 0;
  for (const m of line.matchAll(TOKEN)) {
    if (m.index > last) out.push(line.slice(last, m.index));
    if (m[1] !== undefined) {
      out.push(<strong key={k++}>{m[1]}</strong>);
    } else {
      const href = m[3] ?? m[4];
      out.push(
        <a key={k++} href={href} target="_blank" rel="noopener noreferrer nofollow">
          {m[2] ?? m[4]}
        </a>,
      );
    }
    last = m.index + m[0].length;
  }
  if (last < line.length) out.push(line.slice(last));
  return out;
}

/** Plain-text excerpt (formatting markers removed) for previews. */
export function excerpt(text: string, max = 280): string {
  const plain = text
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/^\s*[-*]\s+/gm, "• ")
    .replace(/\s+/g, " ")
    .trim();
  return plain.length > max ? `${plain.slice(0, max - 1).trimEnd()}…` : plain;
}
