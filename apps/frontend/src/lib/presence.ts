import { useEffect, useRef } from "react";
import { ApiError } from "../api/client";
import type { PresenceArea } from "../api/types";

/** How often an open tab reports itself (the server counts it for 2 minutes). */
export const PRESENCE_INTERVAL_MS = 60_000;

const TAB_KEY = "dsp.presence.tab";
let memoryTabId: string | undefined;

function randomId(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  // base64url without padding: 22 characters.
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Random id of this browser tab, kept in sessionStorage (gone when the tab
 * closes, never sent as a cookie). It only lets the server count the tab once.
 */
export function tabId(): string {
  try {
    const stored = window.sessionStorage.getItem(TAB_KEY);
    if (stored && /^[A-Za-z0-9_-]{16,64}$/.test(stored)) return stored;
    const id = randomId();
    window.sessionStorage.setItem(TAB_KEY, id);
    return id;
  } catch {
    memoryTabId ??= randomId();
    return memoryTabId;
  }
}

/** Area of the public page reported in heartbeats. */
export function publicArea(pathname: string): PresenceArea {
  const p = pathname.replace(/\/$/, "") || "/";
  if (p === "/") return "status";
  if (p.startsWith("/incidents/")) return "incident";
  if (p.startsWith("/maintenances/")) return "maintenance";
  if (p === "/announcements" || p.startsWith("/announcements/")) return "announcements";
  if (p.startsWith("/subscriptions/")) return "subscriptions";
  return "other";
}

/**
 * Sends a heartbeat now, whenever `key` changes, when the tab becomes visible
 * again and every PRESENCE_INTERVAL_MS (plus once `resendAfterMs` after a
 * key change, for labels that depend on content still loading). Hidden tabs
 * are skipped unless `whileHidden` (admin tabs report "hidden" instead). Errors are ignored; a
 * 404 means presence is turned off on the server, so the hook stops.
 */
export function usePresenceBeacon(
  send: (hidden: boolean) => Promise<unknown>,
  key: string,
  { whileHidden = false, resendAfterMs }: { whileHidden?: boolean; resendAfterMs?: number } = {},
) {
  const sendRef = useRef(send);
  useEffect(() => {
    sendRef.current = send;
  });
  const disabled = useRef(false);

  useEffect(() => {
    if (disabled.current) return;
    let lastSent = 0;
    const beat = () => {
      if (disabled.current) return;
      const hidden = document.visibilityState === "hidden";
      if (hidden && !whileHidden) return;
      lastSent = Date.now();
      sendRef.current(hidden).catch((error: unknown) => {
        if (error instanceof ApiError && error.status === 404) disabled.current = true;
      });
    };
    beat();
    const resend = resendAfterMs === undefined ? undefined : window.setTimeout(beat, resendAfterMs);
    const timer = window.setInterval(beat, PRESENCE_INTERVAL_MS);
    const onVisibility = () => {
      // Report a tab coming back right away; a tab going to the background
      // is reported by admin tabs only (and not more than every few seconds).
      if (document.visibilityState === "visible" || Date.now() - lastSent > 5_000) beat();
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(timer);
      window.clearTimeout(resend);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [key, whileHidden, resendAfterMs]);
}
