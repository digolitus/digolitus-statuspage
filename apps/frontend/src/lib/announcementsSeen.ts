import { useEffect, useState } from "react";
import { publicApi } from "../api/endpoints";
import { usePolling } from "./usePolling";

// "New announcement" tracking is a per-viewer convenience: the newest
// publication date the viewer has seen is kept in this browser only.
// Storage may be unavailable (private mode, blocked site data), so every
// access is guarded and the page works without it.

const KEY = "digolitus.announcements.lastSeen";
const EVENT = "app:announcements-seen";
/** Without any record, announcements from the last 14 days count as new. */
export const NEW_WINDOW_MS = 14 * 24 * 60 * 60 * 1000;

export function readLastSeen(): string | null {
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

/** Records that the viewer has seen announcements up to `publishedAt`. */
export function markSeen(publishedAt: string): void {
  const current = readLastSeen();
  if (current && new Date(current) >= new Date(publishedAt)) return;
  try {
    window.localStorage.setItem(KEY, publishedAt);
  } catch {
    // Not persisted; the badge is still cleared for this page view below.
  }
  window.dispatchEvent(new CustomEvent(EVENT, { detail: publishedAt }));
}

export function countUnseen(publishedDates: string[], lastSeen: string | null, now = Date.now()): number {
  const threshold = lastSeen ? new Date(lastSeen).getTime() : now - NEW_WINDOW_MS;
  return publishedDates.filter((d) => new Date(d).getTime() > threshold).length;
}

/** Number of announcements published since the viewer last opened the page. */
export function useUnseenAnnouncements(): number {
  const poll = usePolling((signal) => publicApi.announcements({ limit: 20 }, { signal }), {
    intervalMs: 300_000,
    maxBackoffMs: 900_000,
  });
  const [lastSeen, setLastSeen] = useState(readLastSeen);
  useEffect(() => {
    const update = (e: Event) => setLastSeen((e as CustomEvent<string>).detail ?? readLastSeen());
    window.addEventListener(EVENT, update);
    return () => window.removeEventListener(EVENT, update);
  }, []);
  if (!poll.data) return 0;
  return countUnseen(
    poll.data.data.map((a) => a.published_at),
    lastSeen,
  );
}
