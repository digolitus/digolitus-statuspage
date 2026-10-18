import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

describe("App", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the loading state", () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      () => new Promise(() => undefined)
    );

    render(<App />);

    expect(screen.getByText("Verificando API...")).toBeInTheDocument();
    expect(screen.getByText("Carregando resposta do backend...")).toBeInTheDocument();
  });

  it("renders the backend response", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);

      if (url.includes("/api/hello")) {
        return new Response(
          JSON.stringify({
            message: "Hello World from Digolitus Status Page",
            service: "digolitus-statuspage-backend"
          }),
          { status: 200 }
        );
      }

      return new Response(JSON.stringify({ status: "ok" }), { status: 200 });
    });

    render(<App />);

    expect(
      await screen.findByText("Hello World from Digolitus Status Page")
    ).toBeInTheDocument();
    expect(screen.getByText("Todos os sistemas operacionais")).toBeInTheDocument();
  });

  it("renders an error when the API fails", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("network failure"));

    render(<App />);

    expect(await screen.findByText("Falha na comunicação")).toBeInTheDocument();
    expect(screen.getByText("network failure")).toBeInTheDocument();
  });

  it("allows retrying the API request", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockImplementation(async (input) => {
        const url = String(input);

        if (url.includes("/api/hello")) {
          return new Response(
            JSON.stringify({
              message: "Hello World from Digolitus Status Page",
              service: "digolitus-statuspage-backend"
            }),
            { status: 200 }
          );
        }

        return new Response(JSON.stringify({ status: "ok" }), { status: 200 });
      });

    render(<App />);

    const retryButton = await screen.findByRole("button", {
      name: "Verificar novamente"
    });

    fireEvent.click(retryButton);

    await waitFor(() => {
      expect(
        screen.getByText("Todos os sistemas operacionais")
      ).toBeInTheDocument();
    });

    expect(fetchMock).toHaveBeenCalledTimes(3);
  });
});
