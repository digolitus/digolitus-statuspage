import { ApiError } from "../api/client";
import { publicApi } from "../api/endpoints";
import { EmptyState, ErrorPanel, Loading } from "../components/ui";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";
import { IncidentCard, MaintenanceCard } from "./views";

function NotFoundOr({ error, onRetry, what }: { error: unknown; onRetry: () => void; what: string }) {
  if (error instanceof ApiError && error.status === 404) {
    return <EmptyState title={`${what} não encontrado(a)`}>Verifique o endereço ou volte para a página inicial.</EmptyState>;
  }
  return <ErrorPanel error={error} onRetry={onRetry} />;
}

export function IncidentPage({ id }: { id: string }) {
  const { state, reload } = useApi((signal) => publicApi.incident(id, { signal }), [id]);
  return (
    <section className="container page-section">
      <p>
        <Link to="/">← Voltar ao status</Link>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <NotFoundOr error={state.error} onRetry={reload} what="Incidente" />}
      {state.status === "success" && <IncidentCard incident={state.data} full />}
    </section>
  );
}

export function MaintenancePage({ id }: { id: string }) {
  const { state, reload } = useApi((signal) => publicApi.maintenance(id, { signal }), [id]);
  return (
    <section className="container page-section">
      <p>
        <Link to="/">← Voltar ao status</Link>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <NotFoundOr error={state.error} onRetry={reload} what="Manutenção" />}
      {state.status === "success" && <MaintenanceCard maintenance={state.data} full />}
    </section>
  );
}
