import { useState, type FormEvent } from "react";
import { errorMessage } from "../api/client";
import { adminApi } from "../api/endpoints";
import type {
  Component,
  Monitor,
  MonitorDownStatus,
  MonitoringEvent,
  MonitoringIntegration,
  MonitoringIntegrationWithToken,
  MonitoringProvider,
} from "../api/types";
import { InfoTip, WithInfo } from "../components/InfoTip";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Notice } from "../components/ui";
import { componentStatusLabel, formatDateTime, plural, type Tone } from "../lib/format";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, useSubmit } from "./shared";

const providerLabel: Record<MonitoringProvider, string> = {
  uptimerobot: "UptimeRobot",
  healthchecks: "Healthchecks.io",
};

const DOWN_STATUSES: MonitorDownStatus[] = ["degraded_performance", "partial_outage", "major_outage"];

const stateLabel: Record<Monitor["state"], string> = { unknown: "Sem alertas ainda", up: "No ar", down: "Fora do ar" };
const stateTone: Record<Monitor["state"], Tone> = { unknown: "neutral", up: "ok", down: "critical" };

const eventLabel: Record<MonitoringEvent["kind"], string> = { down: "Caiu", up: "Voltou", ignored: "Ignorado", invalid: "Ilegível" };
const eventTone: Record<MonitoringEvent["kind"], Tone> = { down: "critical", up: "ok", ignored: "neutral", invalid: "bad" };

/** Template the tool must send, per provider (copied into the tool's settings). */
const BODY_TEMPLATE: Record<MonitoringProvider, string> = {
  uptimerobot:
    '{"monitorID":"*monitorID*","monitorFriendlyName":"*monitorFriendlyName*","alertType":"*alertType*","alertDetails":"*alertDetails*"}',
  healthchecks: '{"check_id":"$CODE","name":"$NAME","status":"$STATUS"}',
};

const STEPS: Record<MonitoringProvider, string[]> = {
  uptimerobot: [
    "No UptimeRobot, abra Integrations e crie um Webhook com o endereço acima.",
    "Em “POST value (JSON)”, cole o modelo abaixo e marque “Send as JSON”.",
    "Associe o webhook aos monitores que devem avisar (alertas de down e up).",
  ],
  healthchecks: [
    "No Healthchecks.io, abra Integrations e adicione um Webhook.",
    "Use o endereço acima, método POST, nos dois eventos (down e up).",
    "No corpo da requisição dos dois eventos, cole o modelo abaixo.",
  ],
};

function webhookUrl(token: string): string {
  return `${window.location.origin}/api/v1/hooks/monitoring?token=${encodeURIComponent(token)}`;
}

function CopyField({ label, value, multiline }: { label: string; value: string; multiline?: boolean }) {
  const [copied, setCopied] = useState(false);
  return (
    <Field label={label}>
      {(p) => (
        <div className="copy-field">
          {multiline ? (
            <textarea {...p} readOnly value={value} rows={2} onFocus={(e) => e.currentTarget.select()} />
          ) : (
            <input {...p} readOnly value={value} onFocus={(e) => e.currentTarget.select()} />
          )}
          <button
            type="button"
            className="secondary-button"
            onClick={() => {
              navigator.clipboard?.writeText(value).then(
                () => setCopied(true),
                () => setCopied(false),
              );
            }}
          >
            {copied ? "Copiado" : "Copiar"}
          </button>
        </div>
      )}
    </Field>
  );
}

/** Shown once, right after the token is created or replaced. */
function TokenPanel({ integration, onDone }: { integration: MonitoringIntegrationWithToken; onDone: () => void }) {
  return (
    <section className="panel token-panel" aria-label="Conectar na ferramenta">
      <h2>Conectar {integration.name}</h2>
      <Notice tone="warn">
        <span>
          Exibido só agora{" "}
          <InfoTip label="Sobre o endereço" text="Funciona como senha: quem tiver o endereço consegue mudar o status. Se vazar, use “Trocar endereço”." />
        </span>
      </Notice>
      <CopyField label="Endereço do webhook" value={webhookUrl(integration.webhook_token)} />
      <ol className="steps">
        {STEPS[integration.provider].map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ol>
      <CopyField label="Modelo do corpo" value={BODY_TEMPLATE[integration.provider]} multiline />
      <div className="form-actions">
        <button type="button" className="primary-button" onClick={onDone}>
          Já configurei
        </button>
      </div>
    </section>
  );
}

function AddIntegrationForm({ onCreated }: { onCreated: (i: MonitoringIntegrationWithToken) => void }) {
  const [provider, setProvider] = useState<MonitoringProvider>("uptimerobot");
  const [name, setName] = useState("");
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) return submit.setFieldErrors({ name: "Informe um nome." });
    const created = await submit.run(() => adminApi.monitoring.create({ name: name.trim(), provider }));
    if (created) {
      setName("");
      onCreated(created);
    }
  }
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Nova integração">
      <fieldset className="provider-picker">
        <legend>Ferramenta</legend>
        {(Object.keys(providerLabel) as MonitoringProvider[]).map((p) => (
          <label key={p} className={`provider-option${p === provider ? " selected" : ""}`}>
            <input type="radio" name="provider" value={p} checked={p === provider} onChange={() => setProvider(p)} />
            <strong>{providerLabel[p]}</strong>
            <span className="muted small">{p === "uptimerobot" ? "Monitores HTTP, ping e porta" : "Cron jobs e heartbeats"}</span>
          </label>
        ))}
      </fieldset>
      <Field label="Nome" error={submit.fieldErrors.name} hint="Ex.: UptimeRobot produção">
        {(p) => <input {...p} value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />}
      </Field>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Criando…" : "Criar integração"}
        </button>
      </div>
    </form>
  );
}

function MonitorRow({ monitor: m, components, canManage, onChanged }: { monitor: Monitor; components: Component[]; canManage: boolean; onChanged: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  async function save(body: Parameters<typeof adminApi.monitoring.updateMonitor>[1]) {
    setBusy(true);
    setError(null);
    try {
      await adminApi.monitoring.updateMonitor(m.id, body);
      onChanged();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <tr>
      <td>
        <strong>{m.name || m.external_id}</strong>
        <div className="muted small mono">id {m.external_id}</div>
      </td>
      <td>
        <Badge tone={stateTone[m.state]}>{stateLabel[m.state]}</Badge>
        <div className="muted small">{m.last_signal_at ? `último alerta ${formatDateTime(m.last_signal_at)}` : "nenhum alerta"}</div>
      </td>
      <td>
        <select
          aria-label={`Componente de ${m.name || m.external_id}`}
          value={m.component?.id ?? ""}
          disabled={!canManage || busy}
          onChange={(e) => save({ component_id: e.target.value || null })}
        >
          <option value="">Não ligar (ignorar)</option>
          {m.component && !components.some((c) => c.id === m.component?.id) && <option value={m.component.id}>{m.component.name}</option>}
          {components.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </td>
      <td>
        <select
          aria-label={`Status quando ${m.name || m.external_id} cair`}
          value={m.down_status}
          disabled={!canManage || busy || !m.component}
          onChange={(e) => save({ down_status: e.target.value as MonitorDownStatus })}
        >
          {DOWN_STATUSES.map((s) => (
            <option key={s} value={s}>
              {componentStatusLabel[s]}
            </option>
          ))}
        </select>
        {error && (
          <div className="field-error" role="alert">
            {error}
          </div>
        )}
      </td>
      <td>
        {canManage && (
          <ConfirmButton
            variant="secondary"
            label="Remover"
            confirmLabel="Remover"
            question={`Remover o monitor "${m.name || m.external_id}"?`}
            onConfirm={async () => {
              await adminApi.monitoring.removeMonitor(m.id);
              onChanged();
            }}
          />
        )}
      </td>
    </tr>
  );
}

function AddMonitorForm({ integrationId, onAdded }: { integrationId: string; onAdded: () => void }) {
  const [externalId, setExternalId] = useState("");
  const submit = useSubmit();
  return (
    <form
      className="inline-form"
      noValidate
      aria-label="Cadastrar monitor manualmente"
      onSubmit={async (e) => {
        e.preventDefault();
        if (!externalId.trim()) return submit.setFieldErrors({ external_id: "Informe o identificador." });
        const ok = await submit.run(() => adminApi.monitoring.addMonitor(integrationId, { external_id: externalId.trim() }));
        if (ok) {
          setExternalId("");
          onAdded();
        }
      }}
    >
      <Field label="Cadastrar monitor pelo ID" error={submit.fieldErrors.external_id ?? submit.formError ?? undefined}>
        {(p) => <input {...p} value={externalId} maxLength={128} onChange={(e) => setExternalId(e.target.value)} />}
      </Field>
      <button type="submit" className="secondary-button" disabled={submit.busy}>
        Adicionar
      </button>
    </form>
  );
}

function IntegrationDetail({ integration, canManage, onChanged }: { integration: MonitoringIntegration; canManage: boolean; onChanged: () => void }) {
  const monitors = useApi((signal) => adminApi.monitoring.monitors({ integration_id: integration.id }, { signal }), [integration.id]);
  const events = useApi((signal) => adminApi.monitoring.events(integration.id, { signal }), [integration.id]);
  const comps = useApi((signal) => adminApi.components.list({ is_active: true, limit: 100 }, { signal }), []);
  const reload = () => {
    monitors.reload();
    events.reload();
    onChanged();
  };
  const components = comps.state.status === "success" ? comps.state.data.data : [];
  return (
    <div className="integration-detail">
      <h3>
        <WithInfo
          label="Sobre monitores"
          info="Os monitores aparecem depois do primeiro alerta (ou do teste da ferramenta). Também dá para cadastrar pelo ID. Ligue cada um a um componente e escolha o status enquanto estiver fora do ar."
        >
          Monitores
        </WithInfo>
      </h3>
      {monitors.state.status === "loading" && <Loading />}
      {monitors.state.status === "error" && <ErrorPanel error={monitors.state.error} onRetry={monitors.reload} />}
      {monitors.state.status === "success" &&
        (monitors.state.data.data.length === 0 ? (
          <p className="muted">Nenhum monitor ainda.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Monitor na ferramenta</th>
                  <th>Situação</th>
                  <th>Componente</th>
                  <th>Quando cair, marcar como</th>
                  <th>
                    <span className="visually-hidden">Ações</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {monitors.state.data.data.map((m) => (
                  <MonitorRow key={m.id} monitor={m} components={components} canManage={canManage} onChanged={reload} />
                ))}
              </tbody>
            </table>
          </div>
        ))}
      {canManage && <AddMonitorForm integrationId={integration.id} onAdded={reload} />}

      <h3>Últimos alertas recebidos</h3>
      {events.state.status === "success" &&
        (events.state.data.data.length === 0 ? (
          <p className="muted">Nenhum alerta recebido.</p>
        ) : (
          <ul className="event-log">
            {events.state.data.data.slice(0, 15).map((e) => (
              <li key={e.id}>
                <Badge tone={eventTone[e.kind]}>{eventLabel[e.kind]}</Badge>
                <span>
                  <strong>{e.monitor_name || "Monitor sem nome"}</strong>
                  {e.detail && <span className="muted"> · {e.detail}</span>}
                  {e.applied && <span className="muted"> · status do componente atualizado</span>}
                </span>
                <span className="muted small">{formatDateTime(e.received_at)}</span>
              </li>
            ))}
          </ul>
        ))}
    </div>
  );
}

function situation(i: MonitoringIntegration): { tone: Tone; label: string } {
  if (!i.is_active) return { tone: "neutral", label: "Desativada" };
  if (i.down_count > 0) return { tone: "warn", label: plural(i.down_count, "monitor fora do ar", "monitores fora do ar") };
  if (!i.last_received_at) return { tone: "neutral", label: "Aguardando 1º alerta" };
  return { tone: "ok", label: "Recebendo" };
}

function IntegrationItem({
  integration: i,
  startOpen,
  onChanged,
  onToken,
}: {
  integration: MonitoringIntegration;
  startOpen: boolean;
  onChanged: () => void;
  onToken: (i: MonitoringIntegrationWithToken) => void;
}) {
  const canManage = useAuth().can("monitoring.manage");
  const [open, setOpen] = useState(startOpen);
  const s = situation(i);
  return (
    <li className="admin-item">
      <div className="admin-item-head">
        <div>
          <strong>{i.name}</strong> <span className="muted">· {providerLabel[i.provider]}</span>
          <div className="admin-item-tags">
            <Badge tone={s.tone}>{s.label}</Badge>
            <span className="tag">
              {plural(i.monitor_count, "monitor", "monitores")} · {i.linked_count} ligados a componentes
            </span>
          </div>
          <p className="muted small">
            Último alerta: {formatDateTime(i.last_received_at)} · endereço terminado em …{i.token_hint}
          </p>
        </div>
      </div>
      <div className="form-actions">
        <button type="button" className="secondary-button" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? "Fechar" : "Monitores e alertas"}
        </button>
        {canManage && (
          <>
            <button
              type="button"
              className="secondary-button"
              onClick={async () => {
                await adminApi.monitoring.update(i.id, { is_active: !i.is_active });
                onChanged();
              }}
            >
              {i.is_active ? "Desativar" : "Ativar"}
            </button>
            <ConfirmButton
              variant="secondary"
              label="Trocar endereço"
              confirmLabel="Gerar novo endereço"
              question="O endereço atual para de funcionar na hora. Continuar?"
              onConfirm={async () => onToken(await adminApi.monitoring.rotateToken(i.id))}
            />
            <ConfirmButton
              variant="danger"
              label="Remover"
              confirmLabel="Confirmar remoção"
              question={`Remover "${i.name}", seus monitores e alertas?`}
              onConfirm={async () => {
                await adminApi.monitoring.remove(i.id);
                onChanged();
              }}
            />
          </>
        )}
      </div>
      {open && <IntegrationDetail integration={i} canManage={canManage} onChanged={onChanged} />}
    </li>
  );
}

/** Banner listing linked monitors that are down right now. */
function ActiveAlerts({ version }: { version: number }) {
  const { state } = useApi((signal) => adminApi.monitoring.monitors({ state: "down" }, { signal }), [version]);
  if (state.status !== "success") return null;
  const linked = state.data.data.filter((m) => m.component);
  if (linked.length === 0) return null;
  return (
    <Notice tone="warn">
      <strong>{plural(linked.length, "alerta ativo", "alertas ativos")}:</strong>{" "}
      {linked.map((m, k) => (
        <span key={m.id}>
          {k > 0 && "; "}“{m.name || m.external_id}” ({providerLabel[m.provider]}) está marcando <strong>{m.component?.name}</strong> como{" "}
          {componentStatusLabel[m.down_status].toLowerCase()} desde {formatDateTime(m.state_changed_at)}
        </span>
      ))}
    </Notice>
  );
}

export function MonitoringPage() {
  const { state, reload } = useApi((signal) => adminApi.monitoring.list({ signal }), []);
  const [token, setToken] = useState<MonitoringIntegrationWithToken | null>(null);
  const [version, setVersion] = useState(0);
  const canManage = useAuth().can("monitoring.manage");
  const refresh = () => {
    reload();
    setVersion((v) => v + 1);
  };
  return (
    <div className="admin-content">
      <PageHeader
        title="Monitoramento"
        info="Ferramentas externas avisam quando algo cai. O alerta muda o status do componente ligado ao monitor (vale o mais grave entre status declarado, incidentes, manutenções e monitores) e volta sozinho quando a ferramenta avisa que normalizou."
      />
      <p className="muted">UptimeRobot e Healthchecks.io</p>
      <ActiveAlerts version={version} />
      {token && (
        <TokenPanel
          integration={token}
          onDone={() => {
            setToken(null);
            refresh();
          }}
        />
      )}
      {canManage ? (
        !token && (
          <section className="panel" aria-label="Nova integração">
            <AddIntegrationForm
              onCreated={(i) => {
                setToken(i);
                reload();
              }}
            />
          </section>
        )
      ) : (
        <ReadOnlyNote what="cadastrar integrações ou ligar monitores" />
      )}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhuma integração cadastrada" />
        ) : (
          <ul className="admin-list">
            {state.data.data.map((i) => (
              <IntegrationItem key={i.id} integration={i} startOpen={i.id === token?.id} onChanged={refresh} onToken={setToken} />
            ))}
          </ul>
        ))}
    </div>
  );
}
