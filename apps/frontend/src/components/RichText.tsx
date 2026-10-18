import type { ReactNode } from "react";
import { parse, plainText, type Block, type Inline } from "../lib/markdown";

/**
 * Renders the announcement text format (see lib/markdown.ts): headings,
 * small text, bullet and numbered lists, quotes, rules, and inline bold,
 * italic, underline, strikethrough, highlight, monospace and links.
 *
 * Everything is rendered as React nodes (no HTML injection), and only
 * http(s) links become anchors. Headings start at h3 so they never compete
 * with the page or announcement title.
 */
export function RichText({ text, className = "rich-text" }: { text: string; className?: string }) {
  return <div className={className}>{parse(text).map(block)}</div>;
}

function block(b: Block, i: number): ReactNode {
  switch (b.t) {
    case "h2":
      return <h3 key={i}>{inline(b.c)}</h3>;
    case "h3":
      return <h4 key={i}>{inline(b.c)}</h4>;
    case "small":
      return (
        <p key={i} className="text-small">
          {inline(b.c)}
        </p>
      );
    case "quote":
      return <blockquote key={i}>{inline(b.c)}</blockquote>;
    case "hr":
      return <hr key={i} />;
    case "ul":
    case "ol": {
      const List = b.t;
      return (
        <List key={i}>
          {b.items.map((item, j) => (
            <li key={j}>{inline(item)}</li>
          ))}
        </List>
      );
    }
    default:
      return <p key={i}>{inline(b.c)}</p>;
  }
}

function inline(nodes: Inline[]): ReactNode[] {
  return nodes.map((n, k) => {
    switch (n.t) {
      case "text":
        return n.v;
      case "br":
        return <br key={k} />;
      case "code":
        return <code key={k}>{n.v}</code>;
      case "link":
        return (
          <a key={k} href={n.href} target="_blank" rel="noopener noreferrer nofollow">
            {inline(n.c)}
          </a>
        );
      case "strong":
        return <strong key={k}>{inline(n.c)}</strong>;
      case "em":
        return <em key={k}>{inline(n.c)}</em>;
      case "u":
        return <u key={k}>{inline(n.c)}</u>;
      case "s":
        return <s key={k}>{inline(n.c)}</s>;
      case "mark":
        return <mark key={k}>{inline(n.c)}</mark>;
    }
  });
}

/** Plain-text excerpt (formatting markers removed) for previews. */
export function excerpt(text: string, max = 280): string {
  const plain = plainText(text);
  return plain.length > max ? `${plain.slice(0, max - 1).trimEnd()}…` : plain;
}
