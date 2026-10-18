import { useCallback, useEffect, useRef, useState } from "react";
import { isAbort } from "../api/client";

export interface PollingState<T> {
  data?: T;
  /** Error of the initial load (no data yet). */
  error?: unknown;
  /** Error of the latest refresh while older data is still shown. */
  staleError?: unknown;
  loading: boolean;
  /** A manual refresh is in flight. */
  refreshing: boolean;
}

export interface PollingOptions {
  /** Normal refresh interval. */
  intervalMs: number;
  /** Upper bound for the exponential backoff after failures. */
  maxBackoffMs: number;
}

/**
 * Periodic refresh for public data. One request at a time, aborted on
 * unmount; paused while the tab is hidden; doubles the delay after each
 * failure (capped) and resets on success. Failures after a success keep the
 * previous data and flag it as stale instead of replacing it with an error.
 */
export function usePolling<T>(load: (signal: AbortSignal) => Promise<T>, { intervalMs, maxBackoffMs }: PollingOptions) {
  const [state, setState] = useState<PollingState<T>>({ loading: true, refreshing: false });
  // Latest loader, updated after each render (never during render) so the
  // timer always calls the current function.
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  });
  const controller = useRef<AbortController | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const failures = useRef(0);
  const lastRun = useRef(0);
  const active = useRef(true);

  const fetchOnce = useCallback(async () => {
    if (timer.current) clearTimeout(timer.current);
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    lastRun.current = Date.now();
    try {
      const data = await loadRef.current(c.signal);
      if (c.signal.aborted || !active.current) return;
      failures.current = 0;
      setState({ data, loading: false, refreshing: false });
    } catch (error) {
      if (c.signal.aborted || isAbort(error) || !active.current) return;
      failures.current += 1;
      setState((s) =>
        s.data !== undefined
          ? { ...s, staleError: error, refreshing: false, loading: false }
          : { loading: false, refreshing: false, error },
      );
    }
    if (active.current && !document.hidden) {
      const delay = Math.min(intervalMs * 2 ** failures.current, maxBackoffMs);
      timer.current = setTimeout(() => void fetchOnce(), delay);
    }
  }, [intervalMs, maxBackoffMs]);

  useEffect(() => {
    active.current = true;
    void fetchOnce();
    const onVisibility = () => {
      if (document.hidden) {
        if (timer.current) clearTimeout(timer.current);
      } else if (Date.now() - lastRun.current >= intervalMs) {
        void fetchOnce();
      } else {
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => void fetchOnce(), intervalMs - (Date.now() - lastRun.current));
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      active.current = false;
      document.removeEventListener("visibilitychange", onVisibility);
      if (timer.current) clearTimeout(timer.current);
      controller.current?.abort();
    };
  }, [fetchOnce, intervalMs]);

  /** Manual refresh (event handler): shows a refreshing indicator. */
  const refresh = useCallback(() => {
    setState((s) => ({ ...s, refreshing: s.data !== undefined, loading: s.data === undefined }));
    void fetchOnce();
  }, [fetchOnce]);

  return { ...state, refresh };
}
