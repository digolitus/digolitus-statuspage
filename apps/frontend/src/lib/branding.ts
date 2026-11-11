import { createContext, useContext, useLayoutEffect, useMemo, type RefObject } from "react";
import type { BrandingColors, BrandingFont, PublicBranding } from "../api/types";

/** Fonts bundled with the app (see main.tsx); same keys as the API. */
export const FONTS: { key: BrandingFont; label: string; stack: string }[] = [
  { key: "inter", label: "Inter", stack: '"Inter Variable", Inter' },
  { key: "ibm-plex-sans", label: "IBM Plex Sans", stack: '"IBM Plex Sans Variable", "IBM Plex Sans"' },
  { key: "source-sans-3", label: "Source Sans 3", stack: '"Source Sans 3 Variable", "Source Sans 3"' },
  { key: "nunito-sans", label: "Nunito Sans", stack: '"Nunito Sans Variable", "Nunito Sans"' },
  { key: "roboto", label: "Roboto", stack: '"Roboto Variable", Roboto' },
  { key: "merriweather-sans", label: "Merriweather Sans", stack: '"Merriweather Sans Variable", "Merriweather Sans"' },
  { key: "system", label: "Fonte do sistema", stack: "system-ui" },
];
const FALLBACK_STACK = 'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';

export function fontStack(font: BrandingFont): string {
  return `${FONTS.find((f) => f.key === font)?.stack ?? FONTS[0].stack}, ${FALLBACK_STACK}`;
}

/** The look before the API answers or when it fails (same as the backend defaults). */
export const DEFAULT_BRANDING: PublicBranding = {
  title: "Digolitus Status Page",
  subtitle: "",
  description: "",
  login_title: "Entrar na administração",
  login_message: "",
  footer_text: "Status informado pela equipe de operações.",
  footer_link: null,
  font: "inter",
  colors: { primary: "#197552", header_background: "#ffffff", header_text: "#17242c", page_background: "#f6f8f9" },
  show_title_with_logo: true,
  logo_url: null,
  favicon_url: null,
  updated_at: null,
};

// ---------- colors ----------

function rgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function toHex([r, g, b]: number[]): string {
  return (
    "#" +
    [r, g, b]
      .map((c) =>
        Math.round(Math.min(255, Math.max(0, c)))
          .toString(16)
          .padStart(2, "0"),
      )
      .join("")
  );
}

/** Mixes a with b; weight is the share of b (0..1). */
function mix(a: string, b: string, weight: number): string {
  const x = rgb(a);
  const y = rgb(b);
  return toHex(x.map((c, i) => c + (y[i] - c) * weight));
}

function luminance(hex: string): number {
  const [r, g, b] = rgb(hex).map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two #rrggbb colors (same formula as the API). */
export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

export const MIN_CONTRAST = 4.5;
/** Fixed texts the API checks the brand colors against. */
export const BUTTON_TEXT = "#ffffff";
export const MUTED_TEXT = "#5d6b74";

/** Contrast problems by field, as the API reports them. */
export function contrastProblems(c: BrandingColors): Record<string, string> {
  const valid = (v: string) => /^#[0-9a-f]{6}$/i.test(v);
  const out: Record<string, string> = {};
  const fmt = (r: number) => r.toFixed(1).replace(".", ",");
  if (valid(c.primary) && contrast(c.primary, BUTTON_TEXT) < MIN_CONTRAST) {
    out["colors.primary"] = `Contraste de ${fmt(contrast(c.primary, BUTTON_TEXT))}:1 com o texto branco dos botões; o mínimo é 4,5:1. Escureça a cor.`;
  }
  if (valid(c.header_text) && valid(c.header_background) && contrast(c.header_text, c.header_background) < MIN_CONTRAST) {
    out["colors.header_text"] = `Contraste de ${fmt(contrast(c.header_text, c.header_background))}:1 com o fundo do cabeçalho; o mínimo é 4,5:1.`;
  }
  if (valid(c.page_background) && contrast(c.page_background, MUTED_TEXT) < MIN_CONTRAST) {
    out["colors.page_background"] = `Contraste de ${fmt(contrast(c.page_background, MUTED_TEXT))}:1 com o texto secundário; use um fundo claro.`;
  }
  return out;
}

/** CSS custom properties for a branding (consumed by styles.css). */
export function brandingVars(b: Pick<PublicBranding, "colors" | "font">): Record<string, string> {
  const { primary, header_background, header_text, page_background } = b.colors;
  return {
    "--brand": primary,
    "--brand-strong": mix(primary, "#000000", 0.2),
    "--brand-soft": mix(primary, "#ffffff", 0.9),
    "--header-bg": header_background,
    "--header-text": header_text,
    "--page": page_background,
    "--brand-font": fontStack(b.font),
  };
}

/**
 * Applies the branding variables to an element. CSSOM writes are allowed by
 * the CSP (style-src 'self'), unlike style attributes in markup.
 */
export function useBrandingVars(ref: RefObject<HTMLElement | null>, b: Pick<PublicBranding, "colors" | "font">) {
  const vars = useMemo(() => brandingVars(b), [b]);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    for (const [k, v] of Object.entries(vars)) el.style.setProperty(k, v);
  }, [ref, vars]);
}

/** Favicon used when none is configured: the title's initial on the primary color. */
export function initialFavicon(title: string, color: string): string {
  const initial = (title.trim()[0] ?? "S").toUpperCase().replace(/[<>&"']/g, "");
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="${color}"/>` +
    `<text x="16" y="22.5" text-anchor="middle" font-family="system-ui,sans-serif" font-size="19" font-weight="700" fill="#fff">${initial}</text></svg>`;
  return "data:image/svg+xml," + encodeURIComponent(svg);
}

export function setHead(b: PublicBranding) {
  document.title = b.title;
  const setMeta = (name: string, content: string) => {
    let el = document.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`);
    if (!el) {
      el = document.createElement("meta");
      el.name = name;
      document.head.appendChild(el);
    }
    el.content = content;
  };
  setMeta("description", b.description || b.title);
  let icon = document.head.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!icon) {
    icon = document.createElement("link");
    icon.rel = "icon";
    document.head.appendChild(icon);
  }
  icon.href = b.favicon_url ?? initialFavicon(b.title, b.colors.primary);
}

// ---------- provider ----------

const CACHE_KEY = "branding:v1";

export function readCache(): PublicBranding | null {
  try {
    const raw = window.localStorage.getItem(CACHE_KEY);
    return raw ? { ...DEFAULT_BRANDING, ...(JSON.parse(raw) as PublicBranding) } : null;
  } catch {
    return null;
  }
}

export function writeCache(b: PublicBranding) {
  try {
    window.localStorage.setItem(CACHE_KEY, JSON.stringify(b));
  } catch {
    // Storage unavailable: the next load starts from the defaults.
  }
}

export interface BrandingState {
  branding: PublicBranding;
  /** Reloads after an admin change. */
  refresh: () => void;
}

export const BrandingContext = createContext<BrandingState>({ branding: DEFAULT_BRANDING, refresh: () => {} });

export function useBranding(): BrandingState {
  return useContext(BrandingContext);
}
