import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { ApiError, errorMessage } from "../api/client";
import { publicApi, reporterGoogleUrl, suspicionApi } from "../api/endpoints";
import type { MySuspicion, PublicSuspicion, Reporter, ReportCreate } from "../api/types";
import { InfoTip, WithInfo } from "../components/InfoTip";
import { Badge, EmptyState, ErrorPanel, Field, Loading, Notice } from "../components/ui";
import { useFeatures } from "../lib/features";
import { formatDateTime, formatRelative, plural } from "../lib/format";
import { Link, navigate, useLocation } from "../lib/router";
import {
  choiceLabel,
  closeReasonLabel,
  componentName,
  FREQUENCIES,
  frequencyLabel,
  NOTICED,
  noticedLabel,
  publicStatusLabel,
  REACHES,
  reachLabel,
  suspicionNumber,
  suspicionTone,
  SYMPTOMS,
  symptomLabel,
} from "../lib/suspicions";
import { useApi } from "../lib/useApi";

// "Reporte uma suspeita" (ADR 0019): public list without sign-in; reporting
// and "Minhas suspeitas" need a reporter account (e-mail link or Google).

/** Where to return after signing in (kept for the e-mail link round trip). */
const NEXT_KEY = "dsp.suspicions.next";

function rememberNext(path: string) {
  try {
    window.sessionStorage.setItem(NEXT_KEY, path);
  } catch {
    // Storage unavailable: fall back to "Minhas suspeitas".
  }
}

function takeNext(): string {
  try {
    const v = window.sessionStorage.getItem(NEXT_KEY);
    window.sessionStorage.removeItem(NEXT_KEY);
    if (v && v.startsWith("/suspeitas")) return v;
  } catch {
    // ignore
  }
  return "/suspeitas/minhas";
}

type ReporterState = { status: "loading" } | { status: "out" } | { status: "in"; reporter: Reporter };

function useReporter() {
  const { state, reload } = useApi(async (signal) => {
    try {
      return await suspicionApi.me({ signal });
    } catch (e) {
      if (e instanceof ApiError && (e.status === 401 || e.status === 403)) return null;
      throw e;
    }
  }, []);
  let reporter: ReporterState = { status: "loading" };
  if (state.status === "success") reporter = state.data ? { status: "in", reporter: state.data } : { status: "out" };
  if (state.status === "error") reporter = { status: "out" };
  return { reporter, reload };
}

function Shell({ tab, reporter, onLogout, children }: { tab: "public" | "mine" | "new" | "login"; reporter: ReporterState; onLogout: () => void; children: ReactNode }) {
  const features = useFeatures();
  return (
    <div className="container suspicions-page">
      <div className="suspicions-head">
        <WithInfo
          block
          label="Sobre Reporte uma suspeita"
          info="Percebeu algo errado num serviço? Reporte aqui. O time responsável analisa e, se confirmar, abre um incidente nesta página. Seu nome e e-mail nunca aparecem publicamente."
        >
          <h1>Reporte uma suspeita</h1>
        </WithInfo>
        <div className="suspicions-account">
          {reporter.status === "in" && (
            <>
              <span className="muted small">{reporter.reporter.name}</span>
              <button type="button" className="link-button" onClick={onLogout}>
                Sair
              </button>
            </>
          )}
          {tab !== "new" && features.suspicions && (
            <Link className="primary-button" to="/suspeitas/nova">
              Abrir suspeita
            </Link>
          )}
        </div>
      </div>
      {tab !== "new" && tab !== "login" && (
        <nav className="suspicions-tabs" aria-label="Suspeitas">
          <Link to="/suspeitas" aria-current={tab === "public" ? "page" : undefined}>
            Suspeitas públicas em aberto
          </Link>
          <Link to="/suspeitas/minhas" aria-current={tab === "mine" ? "page" : undefined}>
            Minhas suspeitas
            {reporter.status !== "in" && <span className="visually-hidden"> (requer login)</span>}
          </Link>
        </nav>
      )}
      {children}
    </div>
  );
}

function Disabled() {
  return (
    <div className="container narrow-page">
      <h1>Reportar suspeita indisponível</h1>
      <p>Esta funcionalidade não está ligada no momento.</p>
      <Link to="/">Voltar à página de status</Link>
    </div>
  );
}

/** Routes under /suspeitas. */
export function SuspicionsRoutes({ pathname }: { pathname: string }) {
  const { search } = useLocation();
  const { reporter, reload } = useReporter();
  // The public switch can lag a few seconds; the API answers 404 when off,
  // so a disabled page also shows when the first call fails that way.
  const probe = useApi((signal) => suspicionApi.providers({ signal }), []);
  if (probe.state.status === "loading") return <Loading />;
  if (probe.state.status === "error") {
    const e = probe.state.error;
    if (e instanceof ApiError && e.code === "feature_disabled") return <Disabled />;
    return (
      <div className="container">
        <ErrorPanel error={e} onRetry={probe.reload} />
      </div>
    );
  }
  const providers = probe.state.data;
  const logout = async () => {
    try {
      await suspicionApi.logout();
    } finally {
      reload();
      navigate("/suspeitas");
    }
  };
  const p = pathname.replace(/\/$/, "");
  const tab = p === "/suspeitas/minhas" ? "mine" : p === "/suspeitas/nova" ? "new" : p === "/suspeitas/entrar" ? "login" : "public";
  const needsLogin = (tab === "mine" || tab === "new") && reporter.status === "out";
  let body: ReactNode;
  if (tab === "login") {
    body = <LoginView providers={providers} token={search.get("token")} loginError={search.get("login_error")} onSignedIn={reload} />;
  } else if (reporter.status === "loading" && tab !== "public") {
    body = <Loading />;
  } else if (needsLogin) {
    body = <LoginPanel providers={providers} next={p + (search.toString() ? `?${search.toString()}` : "")} />;
  } else if (tab === "mine") {
    body = <MySuspicions />;
  } else if (tab === "new") {
    body = <NewSuspicion componentId={search.get("component") ?? ""} />;
  } else {
    body = <PublicSuspicions signedIn={reporter.status === "in"} />;
  }
  return (
    <Shell tab={tab} reporter={reporter} onLogout={() => void logout()}>
      {body}
    </Shell>
  );
}

function Bars({ values }: { values: number[] }) {
  const max = Math.max(1, ...values);
  return (
    <span className="suspicion-bars" role="img" aria-label={`Relatos por 10 minutos na última hora: ${values.join(", ")}`}>
      {values.map((v, i) => (
        <i key={i} style={{ height: `${Math.max(v ? 15 : 4, (v / max) * 100)}%` }} />
      ))}
    </span>
  );
}

function PublicSuspicions({ signedIn }: { signedIn: boolean }) {
  const list = useApi((signal) => suspicionApi.list({ signal }), []);
  if (list.state.status === "loading") return <Loading />;
  if (list.state.status === "error") return <ErrorPanel error={list.state.error} onRetry={list.reload} />;
  const items = list.state.data.data;
  if (items.length === 0) {
    return <EmptyState title="Nenhuma suspeita pública em aberto">Se algo não está funcionando para você, abra uma suspeita.</EmptyState>;
  }
  return (
    <ul className="suspicion-list">
      {items.map((s) => (
        <PublicCard key={s.id} s={s} signedIn={signedIn} />
      ))}
    </ul>
  );
}

function PublicCard({ s, signedIn }: { s: PublicSuspicion; signedIn: boolean }) {
  const join = `/suspeitas/nova?component=${encodeURIComponent(s.component.id)}`;
  return (
    <li className="panel suspicion-card">
      <div className="suspicion-card-head">
        <div>
          <h2>{componentName(s.component)}</h2>
          <span className="muted small">
            Desde {formatDateTime(s.opened_at)} · atualizado {formatRelative(s.last_report_at)}
          </span>
        </div>
        <Badge tone={suspicionTone[s.status]}>{publicStatusLabel[s.status]}</Badge>
      </div>
      <div className="admin-item-tags">
        {[s.summary.symptom, s.summary.frequency, s.summary.reach].filter(Boolean).map((v) => (
          <span key={v} className="tag">
            {choiceLabel(v)}
          </span>
        ))}
      </div>
      <div className="suspicion-reports">
        <strong>{s.recent_count}</strong>
        <span className="muted small">
          {plural(s.recent_count, "relato", "relatos")} na última hora · {s.report_count} no total
        </span>
        <Bars values={s.recent} />
      </div>
      <div className="form-actions">
        <Link className="secondary-button" to={join} onClick={() => !signedIn && rememberNext(join)}>
          Também estou com esse problema
        </Link>
      </div>
    </li>
  );
}

function LoginPanel({ providers, next }: { providers: { email: boolean; google: boolean }; next: string }) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState<string | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  async function send(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    rememberNext(next);
    try {
      await suspicionApi.sendLink(email.trim(), name.trim());
      setSent(email.trim());
    } catch (err) {
      setError(err instanceof ApiError ? err : new ApiError(0, "network", errorMessage(err)));
    } finally {
      setBusy(false);
    }
  }
  const fe = error?.fieldErrors() ?? {};
  return (
    <section className="panel reporter-login" aria-labelledby="reporter-login-title">
      <div className="field-label">
        <h2 id="reporter-login-title">Entre para reportar</h2>
        <InfoTip
          label="Por que entrar"
          text="Pedimos login só para saber quem abriu a suspeita e poder responder. Seu nome e e-mail nunca aparecem na página pública. Não há senha."
        />
      </div>
      {providers.google && (
        <a className="secondary-button reporter-google" href={reporterGoogleUrl} onClick={() => rememberNext(next)}>
          <span className="google-mark" aria-hidden="true" />
          Continuar com Google
        </a>
      )}
      {providers.email && providers.google && <div className="or-divider">ou</div>}
      {providers.email &&
        (sent ? (
          <Notice tone="success">Link enviado para {sent}. Ele vale por 15 minutos.</Notice>
        ) : (
          <form className="form" onSubmit={send} noValidate>
            <Field label="E-mail" error={fe.email}>
              {(p) => <input {...p} type="email" autoComplete="email" required value={email} maxLength={254} onChange={(e) => setEmail(e.target.value)} />}
            </Field>
            <Field label="Nome (opcional)" error={fe.name}>
              {(p) => <input {...p} autoComplete="name" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />}
            </Field>
            {error && error.details.length === 0 && (
              <p className="field-error" role="alert">
                {error.message}
              </p>
            )}
            <button type="submit" className="primary-button" disabled={busy || !email.trim()}>
              {busy ? "Enviando…" : "Enviar link de acesso"}
            </button>
          </form>
        ))}
      {!providers.email && !providers.google && <Notice tone="warn">Nenhuma forma de login está configurada. Fale com a equipe responsável.</Notice>}
    </section>
  );
}

const loginErrors: Record<string, string> = {
  cancelled: "Login cancelado.",
  blocked: "Sua conta foi bloqueada para reportar suspeitas.",
  failed: "Não foi possível entrar com o Google. Tente de novo.",
};

function LoginView({
  providers,
  token,
  loginError,
  onSignedIn,
}: {
  providers: { email: boolean; google: boolean };
  token: string | null;
  loginError: string | null;
  onSignedIn: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const started = useRef(false);
  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    suspicionApi.redeem(token).then(
      () => {
        onSignedIn();
        navigate(takeNext(), true);
      },
      (e: unknown) => setError(errorMessage(e)),
    );
  }, [token, onSignedIn]);
  if (token && !error) return <Loading label="Entrando…" />;
  return (
    <>
      {(error || loginError) && (
        <div className="error-panel" role="alert">
          <div>
            <strong>Não foi possível entrar</strong>
            <span>{error ?? loginErrors[loginError ?? ""] ?? loginErrors.failed}</span>
          </div>
        </div>
      )}
      <LoginPanel providers={providers} next={takeNextPeek()} />
    </>
  );
}

function takeNextPeek(): string {
  try {
    return window.sessionStorage.getItem(NEXT_KEY) ?? "/suspeitas/minhas";
  } catch {
    return "/suspeitas/minhas";
  }
}

function MySuspicions() {
  const list = useApi((signal) => suspicionApi.mine({ signal }), []);
  if (list.state.status === "loading") return <Loading />;
  if (list.state.status === "error") return <ErrorPanel error={list.state.error} onRetry={list.reload} />;
  const items = list.state.data.data;
  if (items.length === 0) return <EmptyState title="Você ainda não reportou nenhuma suspeita" />;
  return (
    <ul className="suspicion-list">
      {items.map((s) => (
        <MyCard key={s.id} initial={s} />
      ))}
    </ul>
  );
}

function MyCard({ initial }: { initial: MySuspicion }) {
  const [s, setS] = useState(initial);
  const [reply, setReply] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function send(e: FormEvent) {
    e.preventDefault();
    if (!reply.trim()) return;
    setBusy(true);
    setError(null);
    try {
      setS(await suspicionApi.reply(s.id, reply.trim()));
      setReply("");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <li className="panel suspicion-card">
      <div className="suspicion-card-head">
        <div>
          <h2>{componentName(s.component)}</h2>
          <span className="muted small">
            Reportada em {formatDateTime(s.reported_at)} · {suspicionNumber(s.number)} · {s.report_count} {plural(s.report_count, "relato", "relatos")}
          </span>
        </div>
        <Badge tone={suspicionTone[s.status]}>{publicStatusLabel[s.status]}</Badge>
      </div>
      {s.incident && (
        <p className="small">
          Incidente “{s.incident.title}”{s.incident.resolved_at ? ` resolvido em ${formatDateTime(s.incident.resolved_at)}` : " em andamento"}.{" "}
          <Link to={`/incidents/${s.incident.id}`}>Ver incidente</Link>
        </p>
      )}
      {s.status === "closed" && s.close_reason && (
        <p className="small">
          <strong>{closeReasonLabel[s.close_reason]}.</strong> {s.close_message}
        </p>
      )}
      {s.messages.length > 0 && (
        <ol className="conversation" aria-label="Conversa com o time">
          {s.messages.map((m) => (
            <li key={m.id} className={m.kind === "from_reporter" ? "msg msg-me" : "msg msg-team"}>
              <small>
                {m.kind === "from_reporter" ? "Você" : m.author_name} · {formatDateTime(m.created_at)}
              </small>
              <p>{m.body}</p>
            </li>
          ))}
        </ol>
      )}
      {s.status !== "closed" && (
        <form className="form reply-form" onSubmit={send} noValidate>
          <Field label="Responder ao time">
            {(p) => <textarea {...p} rows={2} maxLength={2000} value={reply} placeholder="Escreva para o time" onChange={(e) => setReply(e.target.value)} />}
          </Field>
          {error && (
            <p className="field-error" role="alert">
              {error}
            </p>
          )}
          <div className="form-actions">
            <button type="submit" className="primary-button" disabled={busy || !reply.trim()}>
              {busy ? "Enviando…" : "Enviar"}
            </button>
          </div>
        </form>
      )}
    </li>
  );
}

function Choice<T extends string>({
  name,
  options,
  labels,
  value,
  onChange,
}: {
  name: string;
  options: readonly T[];
  labels: Record<T, string>;
  value: T | "";
  onChange: (v: T) => void;
}) {
  return (
    <div className="choice-group">
      {options.map((o) => (
        <label key={o} className={value === o ? "choice choice-active" : "choice"}>
          <input type="radio" name={name} value={o} checked={value === o} onChange={() => onChange(o)} />
          {labels[o]}
        </label>
      ))}
    </div>
  );
}

function ChoiceField({ label, info, error, children }: { label: string; info?: string; error?: string; children: ReactNode }) {
  return (
    <fieldset className={`field choice-field${error ? " field-invalid" : ""}`}>
      <legend className="field-label">
        {label}
        {info && <InfoTip label={`Sobre ${label}`} text={info} />}
      </legend>
      {children}
      {error && <small className="field-error">{error}</small>}
    </fieldset>
  );
}

function NewSuspicion({ componentId }: { componentId: string }) {
  const status = useApi((signal) => publicApi.status({ signal }), []);
  const open = useApi((signal) => suspicionApi.list({ signal }), []);
  const [v, setV] = useState({
    component_id: componentId,
    symptom: "" as ReportCreate["symptom"] | "",
    frequency: "" as ReportCreate["frequency"] | "",
    reach: "" as ReportCreate["reach"] | "",
    noticed: "" as ReportCreate["noticed"] | "",
    noticed_at: "",
    location: "",
    error_ref: "",
    description: "",
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const [done, setDone] = useState<MySuspicion | null>(null);

  if (done) {
    return (
      <section className="panel narrow-panel">
        <Notice tone="success">
          Suspeita enviada ({suspicionNumber(done.number)}). {done.report_count > 1 ? `Você e mais ${done.report_count - 1} ${plural(done.report_count - 1, "pessoa", "pessoas")} reportaram este componente. ` : ""}
          O time responsável vai analisar.
        </Notice>
        <div className="form-actions">
          <Link className="primary-button" to="/suspeitas/minhas">
            Acompanhar em Minhas suspeitas
          </Link>
          <Link className="secondary-button" to="/">
            Voltar à página de status
          </Link>
        </div>
      </section>
    );
  }
  if (status.state.status === "loading") return <Loading />;
  if (status.state.status === "error") return <ErrorPanel error={status.state.error} onRetry={status.reload} />;
  const groups = new Map(status.state.data.groups.map((g) => [g.id, g.name]));
  const components = status.state.data.components
    .map((c) => ({ id: c.id, label: c.group_id && groups.get(c.group_id) ? `${groups.get(c.group_id)} · ${c.name}` : c.name }))
    .sort((a, b) => a.label.localeCompare(b.label, "pt-BR"));
  const similar = open.state.status === "success" ? open.state.data.data.find((s) => s.component.id === v.component_id) : undefined;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body: ReportCreate = {
      component_id: v.component_id,
      symptom: v.symptom as ReportCreate["symptom"],
      frequency: v.frequency as ReportCreate["frequency"],
      reach: v.reach as ReportCreate["reach"],
      noticed: v.noticed as ReportCreate["noticed"],
      location: v.location.trim(),
      error_ref: v.error_ref.trim(),
      description: v.description.trim(),
    };
    if (v.noticed === "other" && v.noticed_at) body.noticed_at = new Date(v.noticed_at).toISOString();
    try {
      setDone(await suspicionApi.report(body));
    } catch (err) {
      setError(err instanceof ApiError ? err : new ApiError(0, "network", errorMessage(err)));
    } finally {
      setBusy(false);
    }
  }
  const fe = error?.fieldErrors() ?? {};
  return (
    <form className="form panel suspicion-form" onSubmit={submit} noValidate aria-label="Abrir suspeita">
      <h2>Abrir suspeita</h2>
      <Field label="Componente" error={fe.component_id}>
        {(p) => (
          <select {...p} value={v.component_id} onChange={(e) => setV({ ...v, component_id: e.target.value })}>
            <option value="">Escolha o componente</option>
            {components.map((c) => (
              <option key={c.id} value={c.id}>
                {c.label}
              </option>
            ))}
          </select>
        )}
      </Field>
      {similar && (
        <div className="similar-note" role="note">
          <strong>
            {similar.report_count} {plural(similar.report_count, "pessoa já reportou", "pessoas já reportaram")}
          </strong>{" "}
          problema neste componente. Seu relato será somado a essa suspeita.
        </div>
      )}
      <ChoiceField label="O que você percebeu" info="Escolha o sintoma que mais se aproxima. Detalhes vão na descrição." error={fe.symptom}>
        <Choice name="symptom" options={SYMPTOMS} labels={symptomLabel} value={v.symptom} onChange={(x) => setV({ ...v, symptom: x })} />
      </ChoiceField>
      <div className="form-grid">
        <ChoiceField label="Frequência" error={fe.frequency}>
          <Choice name="frequency" options={FREQUENCIES} labels={frequencyLabel} value={v.frequency} onChange={(x) => setV({ ...v, frequency: x })} />
        </ChoiceField>
        <ChoiceField label="Abrangência percebida" info="Até onde você sabe, quem mais está sendo afetado." error={fe.reach}>
          <Choice name="reach" options={REACHES} labels={reachLabel} value={v.reach} onChange={(x) => setV({ ...v, reach: x })} />
        </ChoiceField>
      </div>
      <ChoiceField label="Quando percebeu" error={fe.noticed ?? fe.noticed_at}>
        <Choice name="noticed" options={NOTICED} labels={noticedLabel} value={v.noticed} onChange={(x) => setV({ ...v, noticed: x })} />
        {v.noticed === "other" && (
          <input
            type="datetime-local"
            aria-label="Data e hora em que percebeu"
            value={v.noticed_at}
            onChange={(e) => setV({ ...v, noticed_at: e.target.value })}
          />
        )}
      </ChoiceField>
      <div className="form-grid">
        <Field label="Local ou rede (opcional)" error={fe.location}>
          {(p) => <input {...p} value={v.location} maxLength={120} placeholder="Ex.: Escritório SP, VPN" onChange={(e) => setV({ ...v, location: e.target.value })} />}
        </Field>
        <Field label="Código de erro ou URL (opcional)" error={fe.error_ref}>
          {(p) => <input {...p} value={v.error_ref} maxLength={300} onChange={(e) => setV({ ...v, error_ref: e.target.value })} />}
        </Field>
      </div>
      <Field label="Descrição" error={fe.description} hint={`${v.description.length}/1000`}>
        {(p) => <textarea {...p} rows={4} maxLength={1000} value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} />}
      </Field>
      {error && error.details.length === 0 && (
        <div className="error-panel" role="alert">
          <div>
            <strong>Não foi possível enviar</strong>
            <span>{error.message}</span>
          </div>
        </div>
      )}
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={busy}>
          {busy ? "Enviando…" : "Enviar suspeita"}
        </button>
        <Link className="secondary-button" to="/suspeitas">
          Cancelar
        </Link>
      </div>
    </form>
  );
}
