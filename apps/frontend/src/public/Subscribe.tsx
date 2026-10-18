import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { ApiError, errorMessage } from "../api/client";
import { publicApi, subscriptionApi } from "../api/endpoints";
import type { PublicComponent, SubscriptionChannel, SubscriptionManageView } from "../api/types";
import { Loading, Notice } from "../components/ui";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";

type Tab = SubscriptionChannel | "feed";

const TABS: { id: Tab; label: string }[] = [
  { id: "email", label: "E-mail" },
  { id: "slack", label: "Slack" },
  { id: "webhook", label: "Webhook" },
  { id: "feed", label: "RSS / Atom" },
];

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Public components as checkboxes; nothing selected means "everything". */
function ComponentChoice({
  components,
  selected,
  onChange,
}: {
  components: PublicComponent[];
  selected: string[] | null;
  onChange: (ids: string[] | null) => void;
}) {
  const all = selected === null;
  const name = useId();
  return (
    <fieldset className="sub-components">
      <legend>O que acompanhar</legend>
      <label className="checkbox">
        <input type="radio" name={name} checked={all} onChange={() => onChange(null)} />
        <span>Todas as atualizações</span>
      </label>
      <label className="checkbox">
        <input type="radio" name={name} checked={!all} onChange={() => onChange([])} disabled={components.length === 0} />
        <span>Somente alguns componentes</span>
      </label>
      {!all && (
        <div className="sub-component-list">
          {components.map((c) => (
            <label key={c.id} className="checkbox">
              <input
                type="checkbox"
                checked={selected.includes(c.id)}
                onChange={(e) => onChange(e.target.checked ? [...selected, c.id] : selected.filter((x) => x !== c.id))}
              />
              <span>{c.name}</span>
            </label>
          ))}
        </div>
      )}
      {!all && <small className="field-hint">Avisos gerais (sem componente) chegam sempre.</small>}
    </fieldset>
  );
}

function SubscribeForm({ channel, components }: { channel: SubscriptionChannel; components: PublicComponent[] }) {
  const [email, setEmail] = useState("");
  const [url, setUrl] = useState("");
  const [selected, setSelected] = useState<string[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const emailId = useId();
  const urlId = useId();

  async function handle(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!EMAIL_RE.test(email.trim())) return setError("Informe um e-mail válido.");
    const u = url.trim();
    if (channel === "slack" && !u.startsWith("https://hooks.slack.com/services/"))
      return setError("Informe a URL de um incoming webhook do Slack (https://hooks.slack.com/services/…).");
    if (channel === "webhook" && !/^https:\/\/\S+$/i.test(u)) return setError("Informe um endereço https:// completo.");
    if (selected !== null && selected.length === 0) return setError("Escolha ao menos um componente.");
    setBusy(true);
    try {
      await subscriptionApi.subscribe({ channel, email: email.trim(), url: channel === "email" ? undefined : u, component_ids: selected ?? [] });
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError && err.details.length ? err.details.map((d) => d.message).join(" ") : errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <Notice tone="success">
        <strong>Confira seu e-mail.</strong> Enviamos um link para {email.trim()}; a inscrição só começa depois da confirmação.
      </Notice>
    );
  }
  return (
    <form className="sub-form" onSubmit={handle} noValidate>
      {channel !== "email" && (
        <div className="field">
          <label htmlFor={urlId}>{channel === "slack" ? "URL do incoming webhook do Slack" : "URL do webhook"}</label>
          <input
            id={urlId}
            type="url"
            inputMode="url"
            maxLength={500}
            placeholder={channel === "slack" ? "https://hooks.slack.com/services/…" : "https://exemplo.com/webhooks/status"}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          {channel === "webhook" && <small className="field-hint">Enviamos um POST JSON assinado (HMAC-SHA256) a cada atualização.</small>}
        </div>
      )}
      <div className="field">
        <label htmlFor={emailId}>{channel === "email" ? "E-mail" : "E-mail para confirmação"}</label>
        <input id={emailId} type="email" autoComplete="email" maxLength={254} placeholder="voce@exemplo.com" value={email} onChange={(e) => setEmail(e.target.value)} />
      </div>
      {components.length > 0 && <ComponentChoice components={components} selected={selected} onChange={setSelected} />}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      <button type="submit" className="primary-button" disabled={busy}>
        {busy ? "Enviando…" : "Inscrever-se"}
      </button>
      <small className="muted">Enviaremos um link de confirmação. Você pode cancelar a qualquer momento.</small>
    </form>
  );
}

function FeedLinks({ atom, rss }: { atom: string; rss: string }) {
  return (
    <div className="sub-form">
      <p className="small">Use um leitor de feeds ou integre a outras ferramentas. Não exige cadastro.</p>
      <div className="feed-links">
        <a className="secondary-button" href={atom} target="_blank" rel="noopener">
          Feed Atom
        </a>
        <a className="secondary-button" href={rss} target="_blank" rel="noopener">
          Feed RSS
        </a>
      </div>
    </div>
  );
}

/** Header button + popover with one tab per channel, like common status pages. */
export function SubscribeMenu() {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<Tab>("email");
  const ref = useRef<HTMLDivElement>(null);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    const onClick = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && setOpen(false);
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onClick);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onClick);
    };
  }, [open]);

  return (
    <div className="subscribe" ref={ref}>
      <button type="button" className="subscribe-button" aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((o) => !o)}>
        Inscrever-se
      </button>
      {open && <SubscribePanel id={panelId} tab={tab} setTab={setTab} />}
    </div>
  );
}

function SubscribePanel({ id, tab, setTab }: { id: string; tab: Tab; setTab: (t: Tab) => void }) {
  const { state } = useApi(async (signal) => {
    const [options, components] = await Promise.all([subscriptionApi.options({ signal }), publicApi.components({ signal })]);
    return { options, components: components.data };
  }, []);
  return (
    <div id={id} className="subscribe-panel" role="dialog" aria-label="Inscrever-se para receber atualizações">
      <div className="sub-tabs" role="tablist">
        {TABS.map((t) => (
          <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} className={tab === t.id ? "sub-tab sub-tab-active" : "sub-tab"} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      <div className="sub-body" role="tabpanel">
        {state.status === "loading" && <Loading />}
        {state.status === "error" && <p className="field-error">{errorMessage(state.error)}</p>}
        {state.status === "success" &&
          (tab === "feed" ? (
            <FeedLinks atom={state.data.options.atom_url} rss={state.data.options.rss_url} />
          ) : state.data.options[tab] ? (
            <SubscribeForm key={tab} channel={tab} components={state.data.components} />
          ) : (
            <div className="sub-form">
              <p className="small">
                Inscrições por {TABS.find((t) => t.id === tab)?.label.toLowerCase()} não estão habilitadas nesta instalação (o envio de
                e-mail não foi configurado). Use o feed RSS/Atom.
              </p>
              <button type="button" className="secondary-button" onClick={() => setTab("feed")}>
                Ver feeds
              </button>
            </div>
          ))}
      </div>
    </div>
  );
}

/** Reads the token from the URL once and removes it from the address bar and history. */
function useUrlToken(): string | null {
  const [token] = useState(() => new URLSearchParams(window.location.search).get("token"));
  useEffect(() => {
    if (token) window.history.replaceState(null, "", window.location.pathname);
  }, [token]);
  return token;
}

const channelText: Record<SubscriptionChannel, string> = { email: "e-mail", webhook: "webhook", slack: "Slack" };

function target(v: SubscriptionManageView) {
  return v.channel === "email" ? v.email : v.target_url;
}

function InvalidLink() {
  return (
    <div className="container narrow-page">
      <h1>Link inválido ou expirado</h1>
      <p>Este link não é mais válido. Se quiser, inscreva-se de novo pelo botão “Inscrever-se” no topo da página.</p>
      <Link to="/">Voltar à página de status</Link>
    </div>
  );
}

/** /subscriptions/confirm?token=… — confirmation needs a click (link scanners only GET). */
export function SubscriptionConfirmPage() {
  const token = useUrlToken();
  const [result, setResult] = useState<SubscriptionManageView | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  if (!token) return <InvalidLink />;
  if (error instanceof ApiError && error.status === 404) return <InvalidLink />;
  if (result) return <ManageView token={token} initial={result} confirmed />;
  return (
    <div className="container narrow-page">
      <h1>Confirmar inscrição</h1>
      <p>Clique para começar a receber as atualizações de status.</p>
      {error !== null && <p className="field-error">{errorMessage(error)}</p>}
      <button
        type="button"
        className="primary-button"
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          try {
            setResult(await subscriptionApi.confirm(token));
          } catch (e) {
            setError(e);
          } finally {
            setBusy(false);
          }
        }}
      >
        Confirmar inscrição
      </button>
    </div>
  );
}

/** /subscriptions/manage?token=… — choose components or unsubscribe. */
export function SubscriptionManagePage() {
  const token = useUrlToken();
  const { state } = useApi((signal) => (token ? subscriptionApi.manage(token, { signal }) : Promise.resolve(null)), [token]);
  if (!token) return <InvalidLink />;
  if (state.status === "loading") {
    return (
      <div className="container narrow-page">
        <Loading />
      </div>
    );
  }
  if (state.status === "error" && !(state.error instanceof ApiError && state.error.status === 404)) {
    return (
      <div className="container narrow-page">
        <p className="field-error">{errorMessage(state.error)}</p>
      </div>
    );
  }
  if (!state.data) return <InvalidLink />;
  return <ManageView token={token} initial={state.data} />;
}

function ManageView({ token, initial, confirmed = false }: { token: string; initial: SubscriptionManageView; confirmed?: boolean }) {
  const [view, setView] = useState(initial);
  const [selected, setSelected] = useState<string[] | null>(initial.all_components ? null : initial.components.map((c) => c.id));
  const [status, setStatus] = useState<string | null>(confirmed ? "Inscrição confirmada. Você receberá as próximas atualizações." : null);
  const [error, setError] = useState<string | null>(null);
  const [gone, setGone] = useState(false);
  const components = useApi((signal) => publicApi.components({ signal }), []).state;
  const list = components.status === "success" ? components.data.data : [];

  if (gone) {
    return (
      <div className="container narrow-page">
        <h1>Inscrição cancelada</h1>
        <p>Você não receberá mais atualizações em {target(view)}.</p>
        <Link to="/">Voltar à página de status</Link>
      </div>
    );
  }
  return (
    <div className="container narrow-page">
      <h1>Sua inscrição</h1>
      <p>
        Atualizações por {channelText[view.channel]} em <strong className="break">{target(view)}</strong>.
      </p>
      {status && <Notice tone="success">{status}</Notice>}
      {view.signing_secret && (
        <Notice tone="info">
          <div className="notice-body">
          <strong>Segredo de assinatura do webhook:</strong> <code className="secret">{view.signing_secret}</code>
          <span className="small">
            Valide <code>X-Digolitus-Signature</code> = <code>sha256=HMAC(segredo, X-Digolitus-Timestamp + "." + corpo)</code>.
          </span>
          </div>
        </Notice>
      )}
      <section className="panel">
        {components.status === "loading" ? <Loading /> : <ComponentChoice components={list} selected={selected} onChange={setSelected} />}
        {error && (
          <p className="field-error" role="alert">
            {error}
          </p>
        )}
        <div className="form-actions">
          <button
            type="button"
            className="primary-button"
            onClick={async () => {
              setError(null);
              if (selected !== null && selected.length === 0) return setError("Escolha ao menos um componente.");
              try {
                setView(await subscriptionApi.updatePreferences(token, selected ?? []));
                setStatus("Preferências salvas.");
              } catch (e) {
                setError(errorMessage(e));
              }
            }}
          >
            Salvar preferências
          </button>
          <button
            type="button"
            className="danger-button"
            onClick={async () => {
              try {
                await subscriptionApi.unsubscribe(token);
                setGone(true);
              } catch (e) {
                setError(errorMessage(e));
              }
            }}
          >
            Cancelar inscrição
          </button>
        </div>
      </section>
    </div>
  );
}
