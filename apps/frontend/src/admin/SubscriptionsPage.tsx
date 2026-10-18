import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Subscription, SubscriptionChannel, SubscriptionCreated, SubscriptionStatus } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Notice, Pager } from "../components/ui";
import { formatDateTime } from "../lib/format";
import { useApi } from "../lib/useApi";
import { ComponentPicker, FormStatus, PageHeader, useSubmit } from "./shared";

const channelLabel: Record<SubscriptionChannel, string> = { email: "E-mail", webhook: "Webhook", slack: "Slack" };
const statusLabel: Record<SubscriptionStatus, string> = { pending: "Aguardando confirmação", active: "Ativa", disabled: "Desativada" };
const statusTone = { pending: "warn", active: "ok", disabled: "neutral" } as const;

function NewIntegration({ onCreated }: { onCreated: () => void }) {
  const [channel, setChannel] = useState<"webhook" | "slack">("webhook");
  const [url, setUrl] = useState("");
  const [contact, setContact] = useState("");
  const [all, setAll] = useState(true);
  const [componentIds, setComponentIds] = useState<string[]>([]);
  const [created, setCreated] = useState<SubscriptionCreated | null>(null);
  const submit = useSubmit();

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    const u = url.trim();
    if (!/^https:\/\/\S+$/i.test(u)) errors.url = "Informe um endereço https:// completo.";
    else if (channel === "slack" && !u.startsWith("https://hooks.slack.com/services/"))
      errors.url = "Use a URL de um incoming webhook do Slack (https://hooks.slack.com/services/…).";
    if (!all && componentIds.length === 0) errors.component_ids = "Escolha ao menos um componente ou receba todas as atualizações.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const result = await submit.run(
      () => adminApi.subscriptions.create({ channel, url: u, email: contact.trim() || null, component_ids: all ? [] : componentIds }),
      "Integração criada e ativa.",
    );
    if (result) {
      setCreated(result);
      setUrl("");
      setContact("");
      onCreated();
    }
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Nova integração">
      <div className="form-grid">
        <Field label="Canal" error={fe.channel}>
          {(p) => (
            <select {...p} value={channel} onChange={(e) => setChannel(e.target.value as "webhook" | "slack")}>
              <option value="webhook">Webhook (JSON assinado)</option>
              <option value="slack">Slack (incoming webhook)</option>
            </select>
          )}
        </Field>
        <Field label="URL" error={fe.url}>
          {(p) => (
            <input
              {...p}
              type="url"
              inputMode="url"
              maxLength={500}
              placeholder={channel === "slack" ? "https://hooks.slack.com/services/…" : "https://exemplo.com/webhooks/status"}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          )}
        </Field>
        <Field label="Contato (opcional)" error={fe.email} hint="E-mail de quem responde pela integração.">
          {(p) => <input {...p} type="email" maxLength={254} value={contact} onChange={(e) => setContact(e.target.value)} />}
        </Field>
      </div>
      <label className="checkbox">
        <input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} />
        <span>Todas as atualizações</span>
      </label>
      {!all && <ComponentPicker legend="Somente estes componentes" selected={componentIds} onChange={setComponentIds} error={fe.component_ids} />}
      <FormStatus error={submit.formError} success={submit.success} />
      {created?.signing_secret && (
        <Notice tone="warn">
          <div className="notice-body">
          <strong>Segredo de assinatura (exibido uma única vez):</strong>
          <code className="secret">{created.signing_secret}</code>
          <span className="small">
            Valide <code>X-Digolitus-Signature: sha256=HMAC(segredo, timestamp + "." + corpo)</code>, com o timestamp de{" "}
            <code>X-Digolitus-Timestamp</code>.
          </span>
          </div>
        </Notice>
      )}
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Criando…" : "Criar integração"}
        </button>
      </div>
    </form>
  );
}

function SubscriptionItem({ s, onChanged }: { s: Subscription; onChanged: () => void }) {
  const submit = useSubmit();
  const target = s.channel === "email" ? s.email : s.target_url;
  return (
    <li className="admin-item">
      <div className="admin-item-head">
        <div className="min-w-0">
          <strong className="break">{target}</strong>
          <div className="admin-item-tags">
            <span className="tag">{channelLabel[s.channel]}</span>
            <span className="tag">{s.all_components ? "Todas as atualizações" : s.components.map((c) => c.name).join(", ")}</span>
            <span className="tag">{s.created_by === "admin" ? "Criada por administrador" : "Autoinscrição"}</span>
            {s.channel !== "email" && s.email && <span className="tag">Contato: {s.email}</span>}
            {s.failure_count > 0 && <span className="tag tag-warn">{s.failure_count} falha(s) seguidas</span>}
          </div>
          <p className="muted small">
            Criada em {formatDateTime(s.created_at)}
            {s.last_delivery_at && ` · última entrega ${formatDateTime(s.last_delivery_at)}`}
          </p>
          {(s.disabled_reason || s.last_error) && <p className="small field-error">{s.disabled_reason ?? s.last_error}</p>}
        </div>
        <Badge tone={statusTone[s.status]}>{statusLabel[s.status]}</Badge>
      </div>
      <div className="form-actions">
        {s.status !== "pending" && (
          <button
            type="button"
            className="secondary-button"
            disabled={submit.busy}
            onClick={() => submit.run(() => adminApi.subscriptions.update(s.id, { enabled: s.status !== "active" }), "Inscrição atualizada.").then((r) => r && onChanged())}
          >
            {s.status === "active" ? "Desativar" : "Reativar"}
          </button>
        )}
        {s.status === "active" && (
          <button type="button" className="secondary-button" disabled={submit.busy} onClick={() => submit.run(() => adminApi.subscriptions.test(s.id), "Mensagem de teste na fila.")}>
            Enviar teste
          </button>
        )}
        <ConfirmButton
          variant="danger"
          label="Excluir"
          confirmLabel="Confirmar exclusão"
          question={`Excluir a inscrição de ${target}? Nada mais será enviado.`}
          onConfirm={async () => {
            await adminApi.subscriptions.remove(s.id);
            onChanged();
          }}
        />
      </div>
      <FormStatus error={submit.formError} success={submit.success} />
    </li>
  );
}

export function SubscriptionsPage() {
  const [channel, setChannel] = useState<SubscriptionChannel | "">("");
  const [status, setStatus] = useState<SubscriptionStatus | "">("");
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const { state, reload } = useApi(
    (signal) =>
      adminApi.subscriptions.list({ channel: channel || undefined, status: status || undefined, q: q.trim() || undefined, limit: 25, offset }, { signal }),
    [channel, status, q, offset],
  );

  return (
    <div className="admin-content">
      <PageHeader title="Inscrições">
        <button type="button" className="primary-button" aria-expanded={creating} onClick={() => setCreating((c) => !c)}>
          Nova integração
        </button>
      </PageHeader>
      <p className="muted">
        Pessoas se inscrevem pela página pública (e-mail, webhook ou Slack, com confirmação por e-mail). Aqui você acompanha as
        inscrições e cria integrações internas de webhook ou Slack, ativas sem confirmação. Entregas que falham são repetidas; após
        15 falhas seguidas a inscrição é desativada.
      </p>
      {creating && (
        <section className="panel" aria-label="Nova integração">
          <NewIntegration onCreated={reload} />
        </section>
      )}
      {state.status === "success" && (
        <div className="stat-row" aria-label="Totais">
          <span className="stat">
            <strong>{state.data.counts.active}</strong> ativas
          </span>
          <span className="stat">
            <strong>{state.data.counts.pending}</strong> aguardando confirmação
          </span>
          <span className="stat">
            <strong>{state.data.counts.disabled}</strong> desativadas
          </span>
        </div>
      )}
      <div className="toolbar">
        <label className="inline-filter">
          Canal{" "}
          <select
            value={channel}
            onChange={(e) => {
              setChannel(e.target.value as SubscriptionChannel | "");
              setOffset(0);
            }}
          >
            <option value="">Todos</option>
            {(Object.keys(channelLabel) as SubscriptionChannel[]).map((c) => (
              <option key={c} value={c}>
                {channelLabel[c]}
              </option>
            ))}
          </select>
        </label>
        <label className="inline-filter">
          Status{" "}
          <select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as SubscriptionStatus | "");
              setOffset(0);
            }}
          >
            <option value="">Todos</option>
            {(Object.keys(statusLabel) as SubscriptionStatus[]).map((x) => (
              <option key={x} value={x}>
                {statusLabel[x]}
              </option>
            ))}
          </select>
        </label>
        <label className="inline-filter">
          Buscar{" "}
          <input
            type="search"
            value={q}
            maxLength={100}
            placeholder="e-mail ou URL"
            onChange={(e) => {
              setQ(e.target.value);
              setOffset(0);
            }}
          />
        </label>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhuma inscrição encontrada" />
        ) : (
          <>
            <ul className="admin-list">
              {state.data.data.map((s) => (
                <SubscriptionItem key={`${s.id}-${s.updated_at}`} s={s} onChanged={reload} />
              ))}
            </ul>
            <Pager pagination={state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}
