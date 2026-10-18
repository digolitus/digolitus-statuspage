import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, errorMessage, request } from "./client";

function respond(status: number, body: unknown) {
  return vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(body), { status }));
}

describe("request", () => {
  afterEach(() => vi.restoreAllMocks());

  it("sends JSON and builds the query string without empty values", async () => {
    const spy = respond(200, { ok: true });
    await request("POST", "/api/x", { body: { a: 1 }, query: { limit: 10, state: undefined, q: "" } });
    const [url, init] = spy.mock.calls[0];
    expect(url).toBe("/api/x?limit=10");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe('{"a":1}');
    expect((init?.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
  });

  it("turns the error envelope into an ApiError with field errors", async () => {
    respond(422, {
      error: { code: "validation_failed", message: "Os dados enviados são inválidos.", details: [{ field: "title", message: "Obrigatório." }], request_id: "r1" },
    });
    const error = await request("POST", "/api/x", { body: {} }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.status).toBe(422);
    expect(apiError.code).toBe("validation_failed");
    expect(apiError.requestId).toBe("r1");
    expect(apiError.fieldErrors()).toEqual({ title: "Obrigatório." });
  });

  it("handles non-JSON error bodies", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("Bad Gateway", { status: 502 }));
    const error = (await request("GET", "/api/x").catch((e: unknown) => e)) as ApiError;
    expect(error.code).toBe("http_error");
    expect(error.message).toContain("502");
  });

  it("reports network failures in plain language", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Failed to fetch"));
    const error = await request("GET", "/api/x").catch((e: unknown) => e);
    expect(errorMessage(error)).toBe("Não foi possível se comunicar com a API.");
  });

  it("times out slow requests", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        }),
    );
    const error = await request("GET", "/api/x", { timeoutMs: 10 }).catch((e: unknown) => e);
    expect(errorMessage(error)).toBe("A API não respondeu a tempo.");
  });
});
