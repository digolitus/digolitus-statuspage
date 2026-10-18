import { useCallback, useEffect, useRef, useState } from "react";
import { isAbort } from "../api/client";

export interface PollingState<T> {
  data?: T;
  /** Error of the initial load (no data yet). */
  error?: unknown;
  /** Error of the latest refresh while older data is still shown. */
  staleError?: unknown;
  /** When the shown data was received. */
  receivedAt?: Date;
  loading: boolean;
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
  const loadRef = useRef(load);
  loadRef.current = load;
  const controller = useRef<AbortController | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const failures = useRef(0);
  const lastRun = useRef(0);
  const mounted = useRef(true);

  const schedule = useCallback(
    (run: () => void) => {
      if (timer.current) clearTimeout(timer.current);
      const delay = Math.min(intervalMs * 2 ** failures.current, maxBackoffMs);
      timer.current = setTimeout(run, delay);
    },
    [intervalMs, maxBackoffMs],
  );

  const refresh = useCallback(async () => {
    if (timer.current) clearTimeout(timer.current);
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    lastRun.current = Date.now();
    setState((s) => ({ ...s, refreshing: s.data !== undefined }));
    try {
      const data = await loadRef.current(c.signal);
      if (c.signal.aborted || !mounted.current) return;
      failures.current = 0;
      setState({ data, receivedAt: new Date(), loading: false, refreshing: false });
    } catch (error) {
      if (c.signal.aborted || isAbort(error) || !mounted.current) return;
      failures.current += 1;
      setState((s) =>
        s.data !== undefined
          ? { ...s, staleError: error, refreshing: false, loading: false }
          : { loading: false, refreshing: false, error },
      );
    }
    if (mounted.current && !document.hidden) {
      schedule(() => void refresh());
    }
  }, [schedule]);

  useEffect(() => {
    mounted.current = true;
    void refresh();
    const onVisibility = () => {
      if (document.hidden) {
        if (timer.current) clearTimeout(timer.current);
      } else if (Date.now() - lastRun.current >= intervalMs) {
        void refresh();
      } else {
        schedule(() => void refresh());
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      mounted.current = false;
      document.removeEventListener("visibilitychange", onVisibility);
      if (timer.current) clearTimeout(timer.current);
      controller.current?.abort();
    };
  }, [refresh, schedule, intervalMs]);

  return { ...state, refresh };
}
