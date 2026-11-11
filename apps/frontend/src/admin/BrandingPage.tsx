import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent, type ReactNode } from "react";
import { adminApi } from "../api/endpoints";
import type { AdminBranding, BrandingColors, BrandingImageKind, BrandingSettings, BrandingUpload, PublicBranding } from "../api/types";
import { BrandMark } from "../components/Branding";
import { InfoTip, WithInfo } from "../components/InfoTip";
import { ConfirmButton, ErrorPanel, Field, Loading } from "../components/ui";
import { FONTS, contrastProblems, initialFavicon, useBranding, useBrandingVars } from "../lib/branding";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, useSubmit } from "./shared";

const PRESETS: { name: string; colors: BrandingColors }[] = [
  { name: "Digolitus", colors: { primary: "#197552", header_background: "#ffffff", header_text: "#17242c", page_background: "#f6f8f9" } },
  { name: "Oceano", colors: { primary: "#1f5fa8", header_background: "#0f2a47", header_text: "#ffffff", page_background: "#f4f7fb" } },
  { name: "Grafite", colors: { primary: "#3a4a57", header_background: "#1d262d", header_text: "#ffffff", page_background: "#f5f6f7" } },
  { name: "Vinho", colors: { primary: "#8c2a45", header_background: "#ffffff", header_text: "#2a1a20", page_background: "#faf6f7" } },
  { name: "Âmbar", colors: { primary: "#8a5200", header_background: "#fff8ec", header_text: "#3a2a10", page_background: "#fffcf6" } },
];

const COLOR_FIELDS: { key: keyof BrandingColors; label: string; hint: string }[] = [
  { key: "primary", label: "Cor principal", hint: "Botões, links e marca. Precisa de contraste com o texto branco dos botões." },
  { key: "header_background", label: "Fundo do cabeçalho", hint: "Barra do topo da página pública." },
  { key: "header_text", label: "Texto do cabeçalho", hint: "Título e menu sobre o fundo do cabeçalho." },
  { key: "page_background", label: "Fundo da página", hint: "Precisa ser claro, para o texto secundário continuar legível." },
];

const IMAGE_LIMITS: Record<BrandingImageKind, { kb: number; accept: string; types: string }> = {
  logo: { kb: 512, accept: "image/png,image/jpeg,image/webp", types: "PNG, JPG ou WebP" },
  favicon: { kb: 128, accept: "image/png,image/x-icon,image/vnd.microsoft.icon,.ico", types: "PNG ou ICO" },
};

/** The editable part of the admin view. */
function toSettings(a: BrandingSettings): BrandingSettings {
  return {
    title: a.title,
    subtitle: a.subtitle,
    description: a.description,
    login_title: a.login_title,
    login_message: a.login_message,
    footer_text: a.footer_text,
    footer_link_label: a.footer_link_label,
    footer_link_url: a.footer_link_url,
    font: a.font,
    colors: { ...a.colors },
    show_title_with_logo: a.show_title_with_logo,
    logo: { source: a.logo.source, url: a.logo.url ?? null },
    favicon: { source: a.favicon.source, url: a.favicon.url ?? null },
  };
}

type Pending = Partial<Record<BrandingImageKind, { file: File; dataUrl: string }>>;

export function BrandingPage() {
  const { state, reload } = useApi((signal) => adminApi.branding.get({ signal }), []);
  return (
    <div className="admin-content">
      <PageHeader
        title="Identidade visual"
        info="Nome, textos, logotipo, favicon, fonte e cores da página pública e da tela de login. A área administrativa mantém o visual padrão. Visitantes veem a mudança no próximo carregamento da página."
      />
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" && <BrandingForm initial={state.data} />}
    </div>
  );
}

function BrandingForm({ initial }: { initial: AdminBranding }) {
  const canManage = useAuth().can("branding.manage");
  const { refresh } = useBranding();
  const [server, setServer] = useState(initial);
  const [saved, setSaved] = useState(() => toSettings(initial));
  const [v, setV] = useState(saved);
  const [pending, setPending] = useState<Pending>({});
  const submit = useSubmit();

  const problems = useMemo(() => contrastProblems(v.colors), [v.colors]);
  const dirty = JSON.stringify(v) !== JSON.stringify(saved) || Object.keys(pending).length > 0;
  const set = <K extends keyof BrandingSettings>(k: K, value: BrandingSettings[K]) => setV((s) => ({ ...s, [k]: value }));
  const setColor = (k: keyof BrandingColors, value: string) => setV((s) => ({ ...s, colors: { ...s.colors, [k]: value.toLowerCase() } }));

  function pickFile(kind: BrandingImageKind, file: File | undefined) {
    if (!file) return;
    const limit = IMAGE_LIMITS[kind];
    if (file.size > limit.kb * 1024) {
      submit.setFieldErrors({ [`${kind}.source`]: `A imagem passa de ${limit.kb} KB.` });
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setPending((p) => ({ ...p, [kind]: { file, dataUrl: String(reader.result) } }));
      setV((s) => ({ ...s, [kind]: { source: "upload", url: null } }));
    };
    reader.readAsDataURL(file);
  }

  function apply(next: AdminBranding) {
    const settings = toSettings(next);
    setServer(next);
    setSaved(settings);
    setV(settings);
    setPending({});
    refresh();
  }

  async function handle(e: FormEvent) {
    e.preventDefault();
    if (Object.keys(problems).length) return submit.setFieldErrors(problems);
    const result = await submit.run(async () => {
      for (const kind of ["logo", "favicon"] as const) {
        const p = pending[kind];
        if (p) await adminApi.branding.uploadImage(kind, p.file);
      }
      const body: BrandingSettings = {
        ...v,
        logo: v.logo.source === "url" ? v.logo : { source: v.logo.source, url: null },
        favicon: v.favicon.source === "url" ? v.favicon : { source: v.favicon.source, url: null },
      };
      return adminApi.branding.update(body);
    }, "Identidade visual salva e publicada.");
    if (result) apply(result);
  }

  async function removeUpload(kind: BrandingImageKind) {
    const result = await submit.run(() => adminApi.branding.deleteImage(kind), "Imagem removida.");
    if (result) apply(result);
  }

  const imageUrl = (kind: BrandingImageKind): string | null => {
    const choice = v[kind];
    if (choice.source === "upload") return pending[kind]?.dataUrl ?? server[`${kind}_upload`]?.url ?? null;
    if (choice.source === "url") return choice.url?.startsWith("https://") ? choice.url : null;
    return null;
  };
  const preview: PublicBranding = {
    title: v.title.trim() || "Status",
    subtitle: v.subtitle.trim(),
    description: v.description,
    login_title: v.login_title.trim() || "Entrar",
    login_message: v.login_message.trim(),
    footer_text: v.footer_text.trim(),
    footer_link: v.footer_link_label.trim() && v.footer_link_url.trim() ? { label: v.footer_link_label.trim(), url: v.footer_link_url.trim() } : null,
    font: v.font,
    colors: v.colors,
    show_title_with_logo: v.show_title_with_logo,
    logo_url: imageUrl("logo"),
    favicon_url: imageUrl("favicon"),
    updated_at: null,
  };

  const fe = { ...problems, ...submit.fieldErrors };
  const disabled = !canManage;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Identidade visual">
      {!canManage && <ReadOnlyNote what="alterar a identidade visual" />}
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={disabled || submit.busy || !dirty || Object.keys(problems).length > 0}>
          {submit.busy ? "Salvando…" : "Salvar e publicar"}
        </button>
        <button
          type="button"
          className="secondary-button"
          disabled={disabled || submit.busy}
          onClick={() => {
            setV(toSettings(server.defaults));
            setPending({});
          }}
        >
          Restaurar padrão
        </button>
        {dirty && <span className="muted small">Alterações não salvas</span>}
      </div>
      <FormStatus error={submit.formError} success={submit.success} />

      <div className="branding-layout">
        <div className="form">
          <fieldset className="panel form" disabled={disabled}>
            <legend className="section-title">Nome e textos</legend>
            <Field label="Título da aplicação" error={fe.title} info="Aparece no cabeçalho, na aba do navegador, na tela de login, nos e-mails e nos feeds.">
              {(p) => <input {...p} value={v.title} maxLength={60} onChange={(e) => set("title", e.target.value)} />}
            </Field>
            <Field label="Subtítulo do cabeçalho (opcional)" error={fe.subtitle}>
              {(p) => (
                <input
                  {...p}
                  value={v.subtitle}
                  maxLength={80}
                  placeholder="Ex.: Status dos serviços da empresa"
                  onChange={(e) => set("subtitle", e.target.value)}
                />
              )}
            </Field>
            <Field label="Descrição" error={fe.description} info="Usada por buscadores e na prévia de links compartilhados. Vazia, usa o título.">
              {(p) => <input {...p} value={v.description} maxLength={160} onChange={(e) => set("description", e.target.value)} />}
            </Field>
            <div className="form-grid form-grid-2">
              <Field label="Título da tela de login" error={fe.login_title}>
                {(p) => <input {...p} value={v.login_title} maxLength={60} onChange={(e) => set("login_title", e.target.value)} />}
              </Field>
              <Field label="Mensagem da tela de login (opcional)" error={fe.login_message}>
                {(p) => (
                  <input
                    {...p}
                    value={v.login_message}
                    maxLength={200}
                    placeholder="Ex.: Acesso restrito à equipe de operações."
                    onChange={(e) => set("login_message", e.target.value)}
                  />
                )}
              </Field>
            </div>
            <Field label="Texto do rodapé" error={fe.footer_text} hint={`${v.footer_text.length}/280`}>
              {(p) => <textarea {...p} rows={2} value={v.footer_text} maxLength={280} onChange={(e) => set("footer_text", e.target.value)} />}
            </Field>
            <div className="form-grid form-grid-2">
              <Field label="Link no rodapé: texto (opcional)" error={fe.footer_link_label}>
                {(p) => (
                  <input
                    {...p}
                    value={v.footer_link_label}
                    maxLength={40}
                    placeholder="Ex.: Suporte"
                    onChange={(e) => set("footer_link_label", e.target.value)}
                  />
                )}
              </Field>
              <Field label="Link no rodapé: endereço" error={fe.footer_link_url}>
                {(p) => (
                  <input
                    {...p}
                    type="url"
                    value={v.footer_link_url}
                    maxLength={300}
                    placeholder="https://"
                    onChange={(e) => set("footer_link_url", e.target.value)}
                  />
                )}
              </Field>
            </div>
          </fieldset>

          <fieldset className="panel form" disabled={disabled}>
            <legend className="section-title">
              Logotipo e favicon
              <InfoTip
                label="Sobre imagens"
                text="Envie o arquivo (fica guardado no Status Page) ou use uma URL https:// de um domínio liberado. Sem favicon, a aba mostra a inicial do título na cor principal."
              />
            </legend>
            <ImageSetting
              kind="logo"
              label="Logotipo"
              value={v.logo}
              upload={server.logo_upload}
              pendingUrl={pending.logo?.dataUrl}
              hosts={server.allowed_image_hosts}
              errors={fe}
              onChange={(c) => set("logo", c)}
              onFile={(f) => pickFile("logo", f)}
              onRemove={() => void removeUpload("logo")}
            />
            <label className="checkbox">
              <input type="checkbox" checked={v.show_title_with_logo} onChange={(e) => set("show_title_with_logo", e.target.checked)} />
              Mostrar o título ao lado do logotipo
            </label>
            <ImageSetting
              kind="favicon"
              label="Favicon"
              value={v.favicon}
              upload={server.favicon_upload}
              pendingUrl={pending.favicon?.dataUrl}
              hosts={server.allowed_image_hosts}
              errors={fe}
              onChange={(c) => set("favicon", c)}
              onFile={(f) => pickFile("favicon", f)}
              onRemove={() => void removeUpload("favicon")}
            />
            <div className="field">
              <WithInfo
                label="Sobre domínios liberados"
                info="Para usar uma URL, a imagem precisa estar em https:// num destes domínios. A lista vem da variável BRANDING_IMAGE_HOSTS do servidor, porque também entra na política de segurança da página. Para incluir um domínio, ajuste a variável no backend e no frontend e reinicie os dois."
              >
                <span className="field-label">Domínios liberados para imagens</span>
              </WithInfo>
              {server.allowed_image_hosts.length > 0 ? (
                <div className="host-chips">
                  {server.allowed_image_hosts.map((h) => (
                    <span key={h} className="host-chip">
                      {h}
                    </span>
                  ))}
                </div>
              ) : (
                <span className="muted small">Nenhum</span>
              )}
            </div>
          </fieldset>

          <fieldset className="panel form" disabled={disabled}>
            <legend className="section-title">
              Fonte e cores
              <InfoTip
                label="Sobre cores"
                text="O contraste mínimo é 4,5:1 (WCAG AA). As cores de status (operacional, degradado, indisponível…) não mudam: têm significado fixo em qualquer marca."
              />
            </legend>
            <Field label="Fonte do texto" error={fe.font} info="Fontes livres servidas pelo próprio Status Page, sem depender de serviços externos.">
              {(p) => (
                <select {...p} value={v.font} onChange={(e) => set("font", e.target.value as BrandingSettings["font"])}>
                  {FONTS.map((f) => (
                    <option key={f.key} value={f.key}>
                      {f.label}
                      {f.key === "inter" ? " (padrão)" : ""}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <div className="field">
              <span className="field-label">Paletas prontas</span>
              <div className="preset-row">
                {PRESETS.map((pr) => (
                  <button key={pr.name} type="button" className="preset-chip" onClick={() => set("colors", { ...pr.colors })}>
                    <PresetDot color={pr.colors.primary} />
                    {pr.name}
                  </button>
                ))}
              </div>
            </div>
            <div className="branding-swatches">
              {COLOR_FIELDS.map((c) => (
                <Field key={c.key} label={c.label} error={fe[`colors.${c.key}`]} info={c.hint}>
                  {(p) => (
                    <div className="color-field">
                      <input
                        type="color"
                        aria-label={`${c.label} (seletor)`}
                        value={/^#[0-9a-f]{6}$/i.test(v.colors[c.key]) ? v.colors[c.key] : "#000000"}
                        onChange={(e) => setColor(c.key, e.target.value)}
                      />
                      <input {...p} type="text" value={v.colors[c.key]} maxLength={7} spellCheck={false} onChange={(e) => setColor(c.key, e.target.value)} />
                    </div>
                  )}
                </Field>
              ))}
            </div>
          </fieldset>
        </div>

        <BrandingPreview branding={preview} />
      </div>
    </form>
  );
}

/** Colored dot set through CSSOM (the CSP forbids style attributes). */
function PresetDot({ color }: { color: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    ref.current?.style.setProperty("background", color);
  }, [color]);
  return <span ref={ref} className="preset-dot" aria-hidden="true" />;
}

function ImageSetting({
  kind,
  label,
  value,
  upload,
  pendingUrl,
  hosts,
  errors,
  onChange,
  onFile,
  onRemove,
}: {
  kind: BrandingImageKind;
  label: string;
  value: BrandingSettings["logo"];
  upload: BrandingUpload | null;
  pendingUrl?: string;
  hosts: string[];
  errors: Record<string, string>;
  onChange: (c: BrandingSettings["logo"]) => void;
  onFile: (f: File | undefined) => void;
  onRemove: () => void;
}) {
  const limit = IMAGE_LIMITS[kind];
  const fileId = `${kind}-file`;
  const options: { source: BrandingSettings["logo"]["source"]; label: string }[] = [
    { source: "none", label: "Nenhum" },
    { source: "upload", label: "Enviar arquivo" },
    { source: "url", label: "Usar URL" },
  ];
  const thumb = pendingUrl ?? upload?.url;
  return (
    <div className="image-setting" role="group" aria-label={label}>
      <div className="image-setting-head">
        <strong>{label}</strong>
        <div className="segmented" role="group" aria-label={`Origem do ${label.toLowerCase()}`}>
          {options.map((o) => (
            <button
              key={o.source}
              type="button"
              aria-pressed={value.source === o.source}
              onClick={() => onChange({ source: o.source, url: o.source === "url" ? (value.url ?? "") : null })}
            >
              {o.label}
            </button>
          ))}
        </div>
      </div>
      {value.source === "upload" && (
        <div className="image-upload-row">
          <div className={`image-thumb${kind === "favicon" ? " square" : ""}`}>{thumb ? <img src={thumb} alt="" /> : "sem arquivo"}</div>
          <div className="grow">
            <label className="secondary-button" htmlFor={fileId}>
              {thumb ? "Trocar arquivo" : "Escolher arquivo"}
            </label>
            <input
              id={fileId}
              className="visually-hidden"
              type="file"
              accept={limit.accept}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                onFile(e.target.files?.[0]);
                e.target.value = "";
              }}
            />
            <small className="field-hint">
              {limit.types}, até {limit.kb} KB{pendingUrl ? " · enviado ao salvar" : ""}
            </small>
          </div>
          {upload && !pendingUrl && (
            <ConfirmButton
              label="Remover arquivo"
              confirmLabel="Remover agora"
              variant="secondary"
              question={
                kind === "logo"
                  ? "Remover o logotipo enviado? Se estiver em uso, a página volta a mostrar a inicial."
                  : "Remover o favicon enviado? Se estiver em uso, volta o favicon padrão."
              }
              onConfirm={onRemove}
            />
          )}
        </div>
      )}
      {value.source === "upload" && errors[`${kind}.source`] && <small className="field-error">{errors[`${kind}.source`]}</small>}
      {value.source === "url" && (
        <Field
          label={`URL pública do ${label.toLowerCase()}`}
          error={errors[`${kind}.url`]}
          info={hosts.length ? `Use https:// num destes domínios: ${hosts.join(", ")}.` : "Nenhum domínio está liberado: envie o arquivo."}
        >
          {(p) => (
            <input
              {...p}
              type="url"
              value={value.url ?? ""}
              maxLength={500}
              placeholder={hosts.length ? `https://${hosts[0]}/${kind}.png` : "https://"}
              onChange={(e) => onChange({ source: "url", url: e.target.value })}
            />
          )}
        </Field>
      )}
    </div>
  );
}

function BrandingPreview({ branding }: { branding: PublicBranding }) {
  const [view, setView] = useState<"public" | "login">("public");
  const ref = useRef<HTMLDivElement>(null);
  useBrandingVars(ref, branding);
  const favicon = branding.favicon_url ?? initialFavicon(branding.title, branding.colors.primary);
  return (
    <aside className="branding-preview" aria-label="Prévia">
      <div className="preview-head">
        <div className="segmented" role="tablist" aria-label="Prévia">
          <button type="button" role="tab" aria-selected={view === "public"} aria-pressed={view === "public"} onClick={() => setView("public")}>
            Página pública
          </button>
          <button type="button" role="tab" aria-selected={view === "login"} aria-pressed={view === "login"} onClick={() => setView("login")}>
            Tela de login
          </button>
        </div>
        <InfoTip
          label="Sobre prévia"
          text="A prévia usa o que está no formulário, antes de salvar. Visitantes veem a mudança no próximo carregamento da página."
        />
      </div>
      <div className="preview-frame">
        <div className="preview-tab">
          <img src={favicon} alt="" />
          <span>{branding.title}</span>
        </div>
        <div className="preview-body branded" ref={ref}>
          {view === "public" ? <PublicPreview branding={branding} /> : <LoginPreview branding={branding} />}
        </div>
      </div>
    </aside>
  );
}

function PublicPreview({ branding }: { branding: PublicBranding }) {
  return (
    <>
      <header className="site-header">
        <div className="container header-content">
          <span className="brand">
            <BrandMark branding={branding} />
          </span>
          <nav className="main-navigation" aria-label="Navegação (prévia)">
            <a href="#branding-preview" onClick={(e) => e.preventDefault()}>
              Componentes
            </a>
            <a href="#branding-preview" onClick={(e) => e.preventDefault()}>
              Histórico
            </a>
          </nav>
        </div>
      </header>
      <div className="preview-sample">
        <div className="preview-status">Todos os sistemas operacionais</div>
        <PreviewRow name="API pública" />
        <PreviewRow name="Portal do cliente" />
      </div>
      <footer className="site-footer">
        <div className="container footer-content">
          <span>{branding.title}</span>
          <span className="footer-text">
            {branding.footer_text}
            {branding.footer_link && (
              <>
                {branding.footer_text && " "}
                <a href={branding.footer_link.url} onClick={(e) => e.preventDefault()}>
                  {branding.footer_link.label}
                </a>
              </>
            )}
          </span>
        </div>
      </footer>
    </>
  );
}

function PreviewRow({ name }: { name: string }): ReactNode {
  return (
    <div className="preview-row">
      <a href="#branding-preview" onClick={(e) => e.preventDefault()}>
        {name}
      </a>
      <span className="badge badge-ok">Operacional</span>
    </div>
  );
}

function LoginPreview({ branding }: { branding: PublicBranding }) {
  return (
    <div className="preview-login">
      <div className="auth-card">
        <div className="brand">
          <BrandMark branding={branding} subtitle={false} />
        </div>
        <h1>{branding.login_title}</h1>
        {branding.login_message && <p className="auth-message">{branding.login_message}</p>}
        <span className="field-label">E-mail</span>
        <div className="preview-fake-input" />
        <span className="field-label">Senha</span>
        <div className="preview-fake-input" />
        <span className="primary-button">Entrar</span>
      </div>
    </div>
  );
}
