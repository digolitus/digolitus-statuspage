import { useCallback, useEffect, useState } from "react";

type HelloResponse = {
  message: string;
  service: string;
};

type ApiState = "loading" | "success" | "error";

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`A API respondeu com HTTP ${response.status}`);
  }

  return response.json() as Promise<T>;
}

export default function App() {
  const [apiState, setApiState] = useState<ApiState>("loading");
  const [hello, setHello] = useState<HelloResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastChecked, setLastChecked] = useState<string | null>(null);

  const checkApi = useCallback(async () => {
    setApiState("loading");
    setError(null);

    try {
      const response = await fetchJson<HelloResponse>("/api/hello");
      await fetchJson<{ status: string }>("/healthz");

      setHello(response);
      setApiState("success");
      setLastChecked(new Date().toLocaleTimeString("pt-BR"));
    } catch (requestError) {
      setApiState("error");
      setHello(null);
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Não foi possível consultar a API."
      );
    }
  }, []);

  useEffect(() => {
    void checkApi();
  }, [checkApi]);

  const isOperational = apiState === "success";

  return (
    <main className="page-shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Página inicial">
          <span className="brand-mark" aria-hidden="true">
            D
          </span>
          <span>Digolitus Status Page</span>
        </a>
        <span className="version-label">Initial release</span>
      </header>

      <section className="hero" aria-labelledby="page-title">
        <p className="eyebrow">SERVICE STATUS</p>
        <h1 id="page-title">Tudo sobre o seu ambiente, em um só lugar.</h1>
        <p className="hero-description">
          Uma base simples e confiável para acompanhar a disponibilidade dos
          serviços Digolitus.
        </p>
      </section>

      <section className="status-card" aria-live="polite">
        <div className={`status-indicator ${isOperational ? "up" : "down"}`}>
          <span className="status-dot" aria-hidden="true" />
          <div>
            <strong>
              {apiState === "loading"
                ? "Verificando API..."
                : isOperational
                  ? "Todos os sistemas operacionais"
                  : "Não foi possível verificar a API"}
            </strong>
            <p>
              {apiState === "loading"
                ? "Aguarde enquanto consultamos o backend."
                : isOperational
                  ? "A API está respondendo normalmente."
                  : "Verifique a conexão e tente novamente."}
            </p>
          </div>
        </div>

        <button
          className="retry-button"
          type="button"
          onClick={() => void checkApi()}
          disabled={apiState === "loading"}
        >
          {apiState === "loading" ? "Verificando..." : "Verificar novamente"}
        </button>
      </section>

      <section className="content-grid">
        <article className="panel hello-panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">API RESPONSE</p>
              <h2>Hello World</h2>
            </div>
            <span className="endpoint">GET /api/hello</span>
          </div>

          {apiState === "loading" && (
            <div className="loading-block" role="status">
              <span className="spinner" />
              Carregando resposta do backend...
            </div>
          )}

          {apiState === "error" && (
            <div className="error-block" role="alert">
              <strong>Falha na comunicação</strong>
              <span>{error}</span>
            </div>
          )}

          {apiState === "success" && hello && (
            <div className="response-block">
              <p>{hello.message}</p>
              <code>{hello.service}</code>
            </div>
          )}
        </article>

        <article className="panel">
          <p className="eyebrow">SERVICE INFORMATION</p>
          <h2>Informações básicas</h2>
          <dl className="info-list">
            <div>
              <dt>Serviço</dt>
              <dd>Backend API</dd>
            </div>
            <div>
              <dt>Estado</dt>
              <dd>{isOperational ? "Operacional" : "Indisponível"}</dd>
            </div>
            <div>
              <dt>Última verificação</dt>
              <dd>{lastChecked ?? "Ainda não verificado"}</dd>
            </div>
          </dl>
        </article>
      </section>

      <footer className="footer">
        <span>Digolitus Status Page</span>
        <span>Disponibilidade transparente para evoluir com confiança.</span>
      </footer>
    </main>
  );
}
