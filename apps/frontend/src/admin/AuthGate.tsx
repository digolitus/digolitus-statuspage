import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { ApiError, isAbort, SESSION_EXPIRED_EVENT } from "../api/client";
import { authApi, socialLoginUrl } from "../api/endpoints";
import type { LoginProvider, Me, PermissionKey } from "../api/types";
import { ErrorPanel, Field, Loading, Notice } from "../components/ui";
import { navigate } from "../lib/router";
import { AuthContext, useCan } from "./authContext";
import { FormStatus, useSubmit } from "./shared";

type Gate =
  | { kind: "loading" }
  | { kind: "error"; error: unknown }
  | { kind: "signed-out"; setupRequired: boolean; expired: boolean }
  | { kind: "signed-in"; me: Me };

/**
 * Protects the admin area: shows the first-run setup, the login form or the
 * admin UI. The backend enforces every permission; this only decides what
 * to display.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const [gate, setGate] = useState<Gate>({ kind: "loading" });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    authApi.me({ signal: controller.signal }).then(
      (me) => setGate({ kind: "signed-in", me }),
      async (error: unknown) => {
        if (isAbort(error)) return;
        if (error instanceof ApiError && error.status === 401) {
          try {
            const st = await authApi.setupStatus({ signal: controller.signal });
            setGate((g) => ({ kind: "signed-out", setupRequired: st.setup_required, expired: g.kind === "signed-in" }));
          } catch (e) {
            if (!isAbort(e)) setGate({ kind: "error", error: e });
          }
          return;
        }
        setGate({ kind: "error", error });
      },
    );
    return () => controller.abort();
  }, [nonce]);

  // Any admin call answered with 401 means the session ended (expired,
  // revoked or the user was deactivated): show the login form again.
  useEffect(() => {
    const onExpired = () => setGate((g) => (g.kind === "signed-in" ? { kind: "signed-out", setupRequired: false, expired: true } : g));
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  }, []);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);
  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      setGate({ kind: "signed-out", setupRequired: false, expired: false });
    }
  }, []);

  const me = gate.kind === "signed-in" ? gate.me : null;
  const value = useMemo(() => {
    if (!me) return null;
    const perms = new Set<PermissionKey>(me.permissions);
    const teamPerms = new Set<PermissionKey>(me.is_admin ? [] : me.team_permissions);
    const teams = new Set(me.user.teams.map((t) => t.id));
    const can = (p: PermissionKey) => me.is_admin || perms.has(p);
    const teamOnly = (p: PermissionKey) => teamPerms.has(p);
    const canFor = (p: PermissionKey, owner: string | null | undefined) => can(p) && (!teamOnly(p) || (!!owner && teams.has(owner)));
    return { me, can, canFor, teamOnly, refresh, logout };
  }, [me, refresh, logout]);

  if (gate.kind === "loading") {
    return (
      <AuthShell>
        <Loading label="Verificando sessão…" />
      </AuthShell>
    );
  }
  if (gate.kind === "error") {
    return (
      <AuthShell>
        <ErrorPanel error={gate.error} onRetry={refresh} title="Não foi possível verificar a sessão" />
      </AuthShell>
    );
  }
  if (gate.kind === "signed-out") {
    const onSignedIn = (m: Me) => {
      // The setup address is not a page of the admin area: go to the overview.
      if (window.location.pathname.replace(/\/$/, "") === "/admin/setup") navigate("/admin", true);
      setGate({ kind: "signed-in", me: m });
    };
    return gate.setupRequired ? <SetupPage onDone={onSignedIn} /> : <LoginPage expired={gate.expired} onDone={onSignedIn} />;
  }
  if (gate.me.user.must_change_password) {
    // The API refuses admin calls until the user picks their own password.
    return <RequiredPasswordChange me={gate.me} onDone={refresh} onLogout={logout} />;
  }
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Renders children only when the user holds the permission. */
export function Can({ perm, children, fallback = null }: { perm: PermissionKey; children: ReactNode; fallback?: ReactNode }) {
  return useCan(perm) ? <>{children}</> : <>{fallback}</>;
}

function AuthShell({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <main className="auth-shell" id="admin-main">
      <div className="auth-card">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            D
          </span>
          <span className="brand-name">Digolitus Status Page</span>
        </div>
        {title && <h1>{title}</h1>}
        {children}
      </div>
      <p className="auth-footer">
        <a href="/">Ver página pública</a>
      </p>
    </main>
  );
}

/** Messages for ?login_error=<code>, set by the social sign-in callback. */
const SOCIAL_LOGIN_ERRORS: Record<string, string> = {
  not_allowed: "Nenhum usuário ativo corresponde a essa conta. Peça a um administrador para cadastrar você com o mesmo e-mail.",
  cancelled: "O login foi cancelado.",
  failed: "Não foi possível entrar com o provedor. Tente novamente.",
};

/** Reads and removes ?login_error from the address bar. */
function takeSocialLoginError(): string | null {
  const url = new URL(window.location.href);
  const code = url.searchParams.get("login_error");
  if (code === null) return null;
  url.searchParams.delete("login_error");
  window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
  return SOCIAL_LOGIN_ERRORS[code] ?? SOCIAL_LOGIN_ERRORS.failed;
}

function LoginPage({ expired, onDone }: { expired: boolean; onDone: (me: Me) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [socialError] = useState(takeSocialLoginError);
  const [providers, setProviders] = useState<LoginProvider[]>([]);
  const submit = useSubmit();
  useEffect(() => {
    // Without the list (older backend, network error) only the password form shows.
    const controller = new AbortController();
    authApi.providers({ signal: controller.signal }).then((r) => setProviders(r.data), () => {});
    return () => controller.abort();
  }, []);
  async function handle(e: FormEvent) {
    e.preventDefault();
    if (!email.trim() || !password) return submit.setFieldErrors({ email: email.trim() ? "" : "Informe o e-mail.", password: password ? "" : "Informe a senha." });
    const me = await submit.run(() => authApi.login(email.trim(), password));
    if (me) onDone(me);
    else setPassword("");
  }
  return (
    <AuthShell title="Entrar na administração">
      {expired && <Notice tone="warn">Sua sessão terminou. Entre novamente para continuar.</Notice>}
      {socialError && (
        <div className="error-panel" role="alert">
          <div>
            <strong>Não foi possível entrar</strong>
            <span>{socialError}</span>
          </div>
        </div>
      )}
      <form className="form" onSubmit={handle} noValidate aria-label="Entrar">
        <Field label="E-mail" error={submit.fieldErrors.email || undefined}>
          {(p) => <input {...p} type="email" autoComplete="username" value={email} maxLength={254} onChange={(e) => setEmail(e.target.value)} autoFocus />}
        </Field>
        <Field label="Senha" error={submit.fieldErrors.password || undefined}>
          {(p) => <input {...p} type="password" autoComplete="current-password" value={password} maxLength={128} onChange={(e) => setPassword(e.target.value)} />}
        </Field>
        {submit.formError && (
          <div className="error-panel" role="alert">
            <div>
              <strong>Não foi possível entrar</strong>
              <span>{submit.formError}</span>
            </div>
          </div>
        )}
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Entrando…" : "Entrar"}
        </button>
        <p className="muted small">Esqueceu a senha? Peça a um administrador para redefini-la.</p>
      </form>
      {providers.length > 0 && (
        <div className="social-login" role="group" aria-label="Outras formas de entrar">
          <p className="social-login-divider">
            <span>ou</span>
          </p>
          {providers.map((p) => (
            <a key={p.id} className="secondary-button" href={socialLoginUrl(p.id)}>
              Entrar com {p.name}
            </a>
          ))}
          <p className="muted small">Só para quem já tem usuário: use a conta com o mesmo e-mail cadastrado.</p>
        </div>
      )}
    </AuthShell>
  );
}

/** First sign-in with a password chosen by an administrator. */
function RequiredPasswordChange({ me, onDone, onLogout }: { me: Me; onDone: () => void; onLogout: () => Promise<void> }) {
  const [v, setV] = useState({ current: "", next: "", confirm: "" });
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    if (!v.current) errors.current_password = "Informe a senha recebida.";
    if (v.next.length < 12) errors.new_password = "Use pelo menos 12 caracteres.";
    if (v.next !== v.confirm) errors.confirm = "As senhas não conferem.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const ok = await submit.run(async () => {
      await authApi.changePassword(v.current, v.next);
      return true;
    });
    if (ok) onDone();
  }
  const fe = submit.fieldErrors;
  return (
    <AuthShell title="Defina sua senha">
      <p>
        Olá, {me.user.name}. Sua senha foi definida por um administrador; escolha uma senha pessoal para continuar.
      </p>
      <form className="form" onSubmit={handle} noValidate aria-label="Definir senha">
        <Field label="Senha recebida" error={fe.current_password}>
          {(p) => <input {...p} type="password" autoComplete="current-password" value={v.current} maxLength={128} onChange={(e) => setV({ ...v, current: e.target.value })} />}
        </Field>
        <Field label="Nova senha" error={fe.new_password} hint="12 a 128 caracteres. Prefira uma frase longa.">
          {(p) => <input {...p} type="password" autoComplete="new-password" value={v.next} maxLength={128} onChange={(e) => setV({ ...v, next: e.target.value })} />}
        </Field>
        <Field label="Confirmar nova senha" error={fe.confirm}>
          {(p) => <input {...p} type="password" autoComplete="new-password" value={v.confirm} maxLength={128} onChange={(e) => setV({ ...v, confirm: e.target.value })} />}
        </Field>
        <FormStatus error={submit.formError} success={null} />
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Salvando…" : "Salvar e continuar"}
        </button>
        <button type="button" className="link-button" onClick={() => void onLogout()}>
          Sair
        </button>
      </form>
    </AuthShell>
  );
}

function SetupPage({ onDone }: { onDone: (me: Me) => void }) {
  const [v, setV] = useState({ setup_token: "", name: "", email: "", password: "", confirm: "" });
  const submit = useSubmit();
  const set = (k: keyof typeof v, value: string) => setV((s) => ({ ...s, [k]: value }));
  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    if (!v.setup_token.trim()) errors.setup_token = "Informe o token exibido no log do backend.";
    if (!v.name.trim()) errors.name = "Informe seu nome.";
    if (!v.email.trim()) errors.email = "Informe o e-mail.";
    if (v.password.length < 12) errors.password = "Use pelo menos 12 caracteres.";
    if (v.password !== v.confirm) errors.confirm = "As senhas não conferem.";
    if (Object.keys(errors).length) return submit.setFieldErrors(errors);
    const me = await submit.run(() =>
      authApi.setup({ setup_token: v.setup_token.trim(), name: v.name.trim(), email: v.email.trim(), password: v.password }),
    );
    if (me) onDone(me);
  }
  const fe = submit.fieldErrors;
  return (
    <AuthShell title="Configuração inicial">
      <p>
        Crie o primeiro administrador. Ele terá acesso total, inclusive para cadastrar outros usuários e definir papéis.
      </p>
      <p className="muted small">
        O token de configuração aparece no log do backend na inicialização (mensagem <code>initial setup required</code>,
        campo <code>setup_token</code>). Com Docker Compose: <code>docker compose -f deploy/docker-compose.yml logs backend</code>.
      </p>
      <form className="form" onSubmit={handle} noValidate aria-label="Configuração inicial">
        <Field label="Token de configuração" error={fe.setup_token}>
          {(p) => <input {...p} value={v.setup_token} autoComplete="off" spellCheck={false} maxLength={200} onChange={(e) => set("setup_token", e.target.value)} />}
        </Field>
        <Field label="Nome" error={fe.name}>
          {(p) => <input {...p} value={v.name} autoComplete="name" maxLength={120} onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field label="E-mail" error={fe.email}>
          {(p) => <input {...p} type="email" autoComplete="username" value={v.email} maxLength={254} onChange={(e) => set("email", e.target.value)} />}
        </Field>
        <Field label="Senha" error={fe.password} hint="12 a 128 caracteres. Prefira uma frase longa.">
          {(p) => <input {...p} type="password" autoComplete="new-password" value={v.password} maxLength={128} onChange={(e) => set("password", e.target.value)} />}
        </Field>
        <Field label="Confirmar senha" error={fe.confirm}>
          {(p) => <input {...p} type="password" autoComplete="new-password" value={v.confirm} maxLength={128} onChange={(e) => set("confirm", e.target.value)} />}
        </Field>
        <FormStatus error={submit.formError} success={null} />
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Criando…" : "Criar administrador e entrar"}
        </button>
      </form>
    </AuthShell>
  );
}
