import { vi } from "vitest";

export interface Call {
  method: string;
  path: string;
  query: URLSearchParams;
  body: unknown;
}

type Reply = { status?: number; body?: unknown };
type Route = Reply | ((call: Call) => Reply | Promise<Reply>);

/**
 * Mocks fetch with routes keyed by "METHOD /path" (query string ignored).
 * Unknown routes answer 404 with the API error envelope. Returns the
 * recorded calls so tests can assert on the payloads sent to the API.
 */
export function mockApi(routes: Record<string, Route>) {
  const calls: Call[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const method = (init?.method ?? "GET").toUpperCase();
    const call: Call = {
      method,
      path: url.pathname,
      query: url.searchParams,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    };
    calls.push(call);
    const route = routes[`${method} ${url.pathname}`];
    const reply: Reply = route === undefined
      ? { status: 404, body: { error: { code: "route_not_found", message: "Recurso não encontrado." } } }
      : typeof route === "function"
        ? await route(call)
        : route;
    // No body (null, not ""): responses such as 204 must not carry one.
    return new Response(reply.body === undefined ? null : JSON.stringify(reply.body), {
      status: reply.status ?? 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  return calls;
}

/** A 200 reply carrying a paginated list envelope. */
export const page = <T,>(data: T[]): Reply => ({
  body: { data, pagination: { limit: 20, offset: 0, total: data.length } },
});
