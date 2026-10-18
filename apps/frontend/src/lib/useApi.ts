import { useCallback, useEffect, useRef, useState } from "react";
import { isAbort } from "../api/client";

export type LoadState<T> =
  | { status: "loading"; data?: undefined; error?: undefined }
  | { status: "success"; data: T; error?: undefined }
  | { status: "error"; data?: undefined; error: unknown };

/**
 * Loads data with cancellation: a new load (or unmount) aborts the previous
 * request, so responses never arrive out of order. deps re-trigger loading.
 */
export function useApi<T>(load: (signal: AbortSignal) => Promise<T>, deps: unknown[]) {
  const [state, setState] = useState<LoadState<T>>({ status: "loading" });
  const controller = useRef<AbortController | null>(null);
  const loadRef = useRef(load);
  loadRef.current = load;

  const reload = useCallback(async () => {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    setState((prev) => (prev.status === "success" ? prev : { status: "loading" }));
    try {
      const data = await loadRef.current(c.signal);
      if (!c.signal.aborted) {
        setState({ status: "success", data });
      }
    } catch (error) {
      if (!c.signal.aborted && !isAbort(error)) {
        setState({ status: "error", error });
      }
    }
  }, []);

  useEffect(() => {
    setState({ status: "loading" });
    void reload();
    return () => controller.current?.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { state, reload, setData: (data: T) => setState({ status: "success", data }) };
}
