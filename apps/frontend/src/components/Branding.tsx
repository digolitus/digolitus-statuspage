import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { publicApi } from "../api/endpoints";
import type { PublicBranding } from "../api/types";
import { BrandingContext, DEFAULT_BRANDING, readCache, setHead, writeCache } from "../lib/branding";

/**
 * Loads the public branding once. The last known value (localStorage) shows
 * immediately so a returning visitor does not see the default look flash.
 */
export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<PublicBranding>(() => readCache() ?? DEFAULT_BRANDING);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    publicApi.branding({ signal: controller.signal }).then(
      (b) => {
        setBranding(b);
        writeCache(b);
      },
      () => {
        // Keep the cached or default look; the page itself still works.
      },
    );
    return () => controller.abort();
  }, [version]);
  useEffect(() => setHead(branding), [branding]);
  const refresh = useCallback(() => setVersion((v) => v + 1), []);
  const value = useMemo(() => ({ branding, refresh }), [branding, refresh]);
  return <BrandingContext.Provider value={value}>{children}</BrandingContext.Provider>;
}

/** Logo (or the initial mark) plus the title, as in the public header. */
export function BrandMark({ branding, subtitle = true }: { branding: PublicBranding; subtitle?: boolean }) {
  const showTitle = !branding.logo_url || branding.show_title_with_logo;
  return (
    <>
      {branding.logo_url ? (
        <img className="brand-logo" src={branding.logo_url} alt={showTitle ? "" : branding.title} />
      ) : (
        <span className="brand-mark" aria-hidden="true">
          {(branding.title.trim()[0] ?? "S").toUpperCase()}
        </span>
      )}
      {showTitle && (
        <span className="brand-text">
          <span className="brand-name">{branding.title}</span>
          {subtitle && branding.subtitle && <span className="brand-subtitle">{branding.subtitle}</span>}
        </span>
      )}
    </>
  );
}
