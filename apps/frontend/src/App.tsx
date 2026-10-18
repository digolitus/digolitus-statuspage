import { useCallback, useEffect, useState } from "react";

type HelloResponse = {
  message: string;
  service: string;
};

type ApiState = "loading" | "operational" | "error";

type ServiceStatus = {
  name: string;
  description: string;
  endpoint: string;
};

const services: ServiceStatus[] = [
  {
    name: "Digolitus Status API",
    description: "API principal do Digolitus Status Page",
    endpoint: "GET /api/hello",
  },
];

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`A API respondeu com HTTP ${response.status}`);
  }

  return response.json() as Promise<T>;
}

function formatCheckTime(value: string | null): string {
  if (!value) {
    return "Ainda não verificado";
  }

  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "medium",
  }).format(new Date(value));
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
      const helloResponse = await fetchJson<HelloResponse>("/api/hello");
      await fetchJson<{ status: string }>("/healthz");

      setHello(helloResponse);
      setApiState("operational");
      setLastChecked(new Date().toISOString());
    } catch (requestError) {
      setHello(null);
      setApiState("error");
      setLastChecked(new Date().toISOString());
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Não foi possível consultar a API.",
      );
    }
  }, []);

  useEffect(() => {
    void checkApi();
  }, [checkApi]);

  const isOperational = apiState === "operational";
  const isLoading = apiState === "loading";

  const globalStatusLabel = isLoading
    ? "Verificando os sistemas"
    : isOperational
      ? "Todos os sistemas operacionais"
      : "Interrupção na verificação";

  const globalStatusDescription = isLoading
    ? "Estamos verificando a disponibilidade dos serviços."
    : isOperational
      ? "Nenhum problema conhecido está afetando os serviços."
      : "Não foi possível confirmar a disponibilidade da API.";

  return (
    <div className="app">
      <header className="site-header">
        <div className="container header-content">
          <a className="brand" href="/" aria-label="Digolitus Status Page">
            <span className="brand-mark" aria-hidden="true">
              D
            </span>

            <span className="brand-name">Digolitus Status Page</span>
          </a>

          <nav className="main-navigation" aria-label="Navegação principal">
            <a href="#components">Componentes</a>
            <a href="#incidents">Incidentes</a>
            <a href="#updates">Atualizações</a>
          </nav>
        </div>
      </header>

      <main>
        <section className="status-hero">
          <div className="container">
            <div className="status-hero-content">
              <p className="eyebrow">STATUS DA PLATAFORMA</p>

              <h1>Digolitus Status Page</h1>

              <p className="hero-description">
                Acompanhe a disponibilidade e a saúde dos serviços Digolitus.
              </p>

              <div
                className={`global-status global-status-${apiState}`}
                role="status"
                aria-live="polite"
              >
                <span className="status-icon" aria-hidden="true">
                  {isLoading ? "…" : isOperational ? "✓" : "!"}
                </span>

                <div>
                  <strong>{globalStatusLabel}</strong>
                  <span>{globalStatusDescription}</span>
                </div>
              </div>

              <div className="status-actions">
                <button
                  className="primary-button"
                  type="button"
                  onClick={() => void checkApi()}
                  disabled={isLoading}
                >
                  {isLoading ? "Verificando..." : "Verificar novamente"}
                </button>

                <span className="last-check">
                  Última verificação: {formatCheckTime(lastChecked)}
                </span>
              </div>
            </div>
          </div>
        </section>

        <section className="container page-section" id="components">
          <div className="section-heading">
            <div>
              <p className="eyebrow">SERVIÇOS</p>
              <h2>Componentes da plataforma</h2>
            </div>

            <span className="section-count">
              {services.length} componente monitorado
            </span>
          </div>

          <div className="component-list">
            {services.map((service) => (
              <article className="component-card" key={service.name}>
                <div className="component-main">
                  <span
                    className={`component-indicator component-indicator-${apiState}`}
                    aria-hidden="true"
                  />

                  <div>
                    <h3>{service.name}</h3>
                    <p>{service.description}</p>
                  </div>
                </div>

                <div className="component-meta">
                  <code>{service.endpoint}</code>

                  <span
                    className={`component-status component-status-${apiState}`}
                  >
                    {isLoading
                      ? "Verificando"
                      : isOperational
                        ? "Operacional"
                        : "Indisponível"}
                  </span>
                </div>
              </article>
            ))}
          </div>

          {hello && isOperational && (
            <div className="api-response">
              <div>
                <p className="eyebrow">RESPOSTA DA API</p>
                <p className="api-message">{hello.message}</p>
              </div>

              <code>{hello.service}</code>
            </div>
          )}

          {error && (
            <div className="error-panel" role="alert">
              <strong>Não foi possível consultar a API</strong>
              <span>{error}</span>
            </div>
          )}
        </section>

        <section className="container page-section uptime-section">
          <div className="section-heading">
            <div>
              <p className="eyebrow">DISPONIBILIDADE</p>
              <h2>Histórico de uptime</h2>
            </div>
          </div>

          <div className="empty-state">
            <span className="empty-state-icon" aria-hidden="true">
              —
            </span>

            <div>
              <strong>Histórico ainda não disponível</strong>
              <p>
                A coleta de disponibilidade histórica será adicionada em uma
                próxima versão da plataforma.
              </p>
            </div>
          </div>
        </section>

        <section className="container page-section" id="incidents">
          <div className="section-heading">
            <div>
              <p className="eyebrow">ACOMPANHAMENTO</p>
              <h2>Incidentes recentes</h2>
            </div>
          </div>

          <div className="incident-card">
            <span className="incident-icon" aria-hidden="true">
              ✓
            </span>

            <div>
              <strong>Nenhum incidente registrado</strong>
              <p>
                Ainda não existem incidentes registrados nesta versão do
                Digolitus Status Page.
              </p>
            </div>
          </div>
        </section>

        <section className="container page-section updates-section" id="updates">
          <div className="updates-card">
            <div>
              <p className="eyebrow">ATUALIZAÇÕES</p>
              <h2>Receba atualizações sobre os serviços</h2>
              <p>
                O mecanismo de assinaturas será disponibilizado em uma próxima
                etapa do projeto.
              </p>
            </div>

            <button
              className="secondary-button"
              type="button"
              disabled
              title="Assinaturas ainda não implementadas"
            >
              Assinar atualizações
            </button>
          </div>
        </section>
      </main>

      <footer className="site-footer">
        <div className="container footer-content">
          <span>Digolitus Status Page</span>
          <span>Monitoramento transparente para serviços confiáveis.</span>
        </div>
      </footer>
    </div>
  );
}
