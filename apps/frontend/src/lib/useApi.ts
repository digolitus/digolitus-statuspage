import { useCallback, useEffect, useEffectEvent, useState } from "react";
import { isAbort } from "../api/client";

export type LoadState<T> =
  | { status: "loading"; data?: undefined; error?: undefined }
  | { status: "success"; data: T; error?: undefined }
  | { status: "error"; data?: undefined; error: unknown };

type Settled<T> = { key: string; state: LoadState<T> };

/**
 * Loads data and keeps it in sync with `deps` (compared by value). A new
 * load, or unmount, aborts the previous request, so responses never arrive
 * out of order. `reload()` refetches while still showing the current data.
 *
 * State is only set from async callbacks and event handlers (never
 * synchronously in an effect), as required by the React hooks lint rules.
 */
export function useApi<T>(load: (signal: AbortSignal) => Promise<T>, deps: unknown[]) {
  const key = JSON.stringify(deps);
  const [nonce, setNonce] = useState(0);
  const [settled, setSettled] = useState<Settled<T> | null>(null);
  const run = useEffectEvent((signal: AbortSignal) => load(signal));

  useEffect(() => {
    const controller = new AbortController();
    run(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) setSettled({ key, state: { status: "success", data } });
      },
      (error: unknown) => {
        if (!controller.signal.aborted && !isAbort(error)) setSettled({ key, state: { status: "error", error } });
      },
    );
    return () => controller.abort();
  }, [key, nonce]);

  // Different deps than the settled result: show loading, not stale data
  // from another query. Same deps (a reload): keep showing current data.
  const state: LoadState<T> = settled && settled.key === key ? settled.state : { status: "loading" };

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  const setData = useCallback((data: T) => setSettled({ key, state: { status: "success", data } }), [key]);

  return { state, reload, setData };
}
