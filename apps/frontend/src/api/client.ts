import type { ErrorBody, FieldError } from "./types";

/** Default time budget for one API call. */
export const REQUEST_TIMEOUT_MS = 10_000;

/** An API failure with the contract's error code and field details. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: FieldError[];
  readonly requestId?: string;

  constructor(status: number, code: string, message: string, details: FieldError[] = [], requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.requestId = requestId;
  }

  /** Field errors keyed by field name. */
  fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const d of this.details) {
      out[d.field] ??= d.message;
    }
    return out;
  }
}

export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/** Human-readable message for any error thrown by request(). */
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message;
  }
  if (error instanceof Error && error.name === "TimeoutError") {
    return "A API não respondeu a tempo.";
  }
  return "Não foi possível se comunicar com a API.";
}

type Query = Record<string, string | number | boolean | undefined | null>;

export interface RequestOptions {
  query?: Query;
  body?: unknown;
  signal?: AbortSignal;
  timeoutMs?: number;
}

function buildUrl(path: string, query?: Query): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== "") {
      params.set(key, String(value));
    }
  }
  const qs = params.toString();
  return qs ? `${path}?${qs}` : path;
}

/**
 * Calls the API (same origin; the dev server and Nginx proxy /api). Applies a
 * timeout, honours caller cancellation and converts error envelopes to
 * ApiError. No retries here: callers decide (e.g. polling with backoff).
 */
export async function request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, options.timeoutMs ?? REQUEST_TIMEOUT_MS);
  const onAbort = () => controller.abort();
  options.signal?.addEventListener("abort", onAbort, { once: true });
  if (options.signal?.aborted) {
    controller.abort();
  }

  try {
    const headers: Record<string, string> = { Accept: "application/json" };
    let body: string | undefined;
    if (options.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(options.body);
    }
    let response: Response;
    try {
      response = await fetch(buildUrl(path, options.query), { method, headers, body, signal: controller.signal });
    } catch (error) {
      if (timedOut) {
        const timeout = new Error("timeout");
        timeout.name = "TimeoutError";
        throw timeout;
      }
      throw error;
    }

    const text = await response.text();
    let data: unknown = undefined;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = undefined;
      }
    }
    if (!response.ok) {
      const envelope = data as Partial<ErrorBody> | undefined;
      const err = envelope?.error;
      throw new ApiError(
        response.status,
        err?.code ?? "http_error",
        err?.message ?? `A API respondeu com HTTP ${response.status}.`,
        err?.details ?? [],
        err?.request_id,
      );
    }
    return data as T;
  } finally {
    clearTimeout(timer);
    options.signal?.removeEventListener("abort", onAbort);
  }
}
