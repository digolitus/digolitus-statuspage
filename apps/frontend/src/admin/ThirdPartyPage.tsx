import { useState, type FormEvent } from "react";
import { errorMessage } from "../api/client";
import { adminApi } from "../api/endpoints";
import type { ExternalComponent, ExternalSource } from "../api/types";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading } from "../components/ui";
import { externalStatusLabel, formatDateTime, toneForStatus } from "../lib/format";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { FormStatus, PageHeader, ReadOnlyNote, useSubmit } from "./shared";

const EXAMPLES = ["https://www.githubstatus.com", "https://status.veracode.com"];

function AddSourceForm({ onCreated }: { onCreated: (s: ExternalSource) => void }) {
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const submit = useSubmit();
  async function handle(e: FormEvent) {
    e.preventDefault();
    if (!url.trim()) return submit.setFieldErrors({ url: "Informe a URL da página de status." });
    const created = await submit.run(
      () => adminApi.external.create({ url: url.trim(), ...(name.trim() ? { name: name.trim() } : {}) }),
      "Provedor cadastrado. Escolha abaixo os componentes a exibir.",
    );
    if (created) {
      setUrl("");
      setName("");
      onCreated(created);
    }
  }
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Cadastrar provedor">
      <div className="form-grid">
        <Field label="URL da página de status" error={submit.fieldErrors.url} hint={`Páginas Atlassian Statuspage, ex.: ${EXAMPLES.join(" ou ")}`}>
          {(p) => <input {...p} type="url" inputMode="url" placeholder="https://www.githubstatus.com" value={url} maxLength={500} onChange={(e) => setUrl(e.target.value)} />}
        </Field>
        <Field label="Nome (opcional)" error={submit.fieldErrors.name} hint="Padrão: nome informado pelo provedor.">
          {(p) => <input {...p} value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />}
        </Field>
      </div>
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Consultando o provedor…" : "Cadastrar provedor"}
        </button>
      </div>
    </form>
  );
}

/** Checkbox list of provider components, grouped as the provider does. */
function SelectionEditor({ source, onSaved }: { source: ExternalSource; onSaved: () => void }) {
  const { state, reload } = useApi((signal) => adminApi.external.components(source.id, { signal }), [source.id]);
  const [draft, setDraft] = useState<Set<string> | null>(null);
  const submit = useSubmit();

  if (state.status === "loading") return <Loading label="Carregando componentes do provedor…" />;
  if (state.status === "error") return <ErrorPanel error={state.error} onRetry={reload} />;

  const components = state.data.data;
  const selected = draft ?? new Set(components.filter((c) => c.is_selected).map((c) => c.id));
  const groups = new Map<string, ExternalComponent[]>();
  for (const c of components) {
    const key = c.group_name ?? "";
    groups.set(key, [...(groups.get(key) ?? []), c]);
  }
  const toggle = (ids: string[], on: boolean) => {
    const next = new Set(selected);
    for (const id of ids) {
      if (on) next.add(id);
      else next.delete(id);
    }
    setDraft(next);
  };

  return (
    <div className="selection-editor">
      {[...groups.entries()].map(([group, items]) => {
        const ids = items.filter((c) => c.is_present).map((c) => c.id);
        const all = ids.length > 0 && ids.every((id) => selected.has(id));
        return (
          <fieldset key={group || "_"} className="picker selection-group">
            <legend>
              <label className="checkbox">
                <input type="checkbox" checked={all} disabled={ids.length === 0} onChange={(e) => toggle(ids, e.target.checked)} />
                {group || "Sem grupo"}
              </label>
            </legend>
            <div className="picker-options">
              {items.map((c) => (
                <label key={c.id} className="checkbox">
                  <input type="checkbox" checked={selected.has(c.id)} disabled={!c.is_present && !selected.has(c.id)} onChange={(e) => toggle([c.id], e.target.checked)} />
                  {c.name}
                  <Badge tone={toneForStatus[c.status]}>{externalStatusLabel[c.status]}</Badge>
                  {!c.is_present && <span className="tag tag-warn">Removido no provedor</span>}
                </label>
              ))}
            </div>
          </fieldset>
        );
      })}
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button
          type="button"
          className="primary-button"
          disabled={submit.busy || draft === null}
          onClick={async () => {
            const ok = await submit.run(() => adminApi.external.setSelection(source.id, [...selected]), "Seleção salva.");
            if (ok) {
              setDraft(null);
              reload();
              onSaved();
            }
          }}
        >
          Salvar seleção ({selected.size})
        </button>
        <button type="button" className="secondary-button" disabled={draft === null} onClick={() => setDraft(null)}>
          Descartar alterações
        </button>
      </div>
    </div>
  );
}

function SourceItem({ source: s, onChanged, startOpen }: { source: ExternalSource; onChanged: () => void; startOpen: boolean }) {
  const canManage = useAuth().can("third_party.manage");
  const [open, setOpen] = useState(startOpen);
  const [syncing, setSyncing] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  return (
    <li className="admin-item">
      <div className="admin-item-head">
        <div>
          <strong>{s.name}</strong>{" "}
          <a href={s.base_url} target="_blank" rel="noopener noreferrer" className="muted">
            {s.base_url.replace("https://", "")} ↗
          </a>
          <div className="admin-item-tags">
            <span className="tag">
              {s.selected_count} de {s.component_count} componentes publicados
            </span>
            {!s.is_active && <span className="tag">Inativo</span>}
            {s.stale && <span className="tag tag-warn">Dados desatualizados</span>}
            {s.indicator_description && <span className="tag">Provedor: {s.indicator_description}</span>}
          </div>
          <p className="muted small">
            Última leitura com sucesso: {formatDateTime(s.last_success_at)}
            {s.last_error && (
              <>
                {" "}
                · <span className="field-error">Último erro: {s.last_error}</span>
              </>
            )}
          </p>
        </div>
      </div>
      {canManage && <div className="form-actions">
        <button type="button" className="secondary-button" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          Escolher componentes
        </button>
        <button
          type="button"
          className="secondary-button"
          disabled={syncing}
          onClick={async () => {
            setSyncing(true);
            setSyncError(null);
            try {
              await adminApi.external.sync(s.id);
            } catch (e) {
              setSyncError(errorMessage(e));
            } finally {
              setSyncing(false);
              onChanged();
            }
          }}
        >
          {syncing ? "Sincronizando…" : "Sincronizar agora"}
        </button>
        <button
          type="button"
          className="secondary-button"
          onClick={async () => {
            await adminApi.external.update(s.id, { is_active: !s.is_active });
            onChanged();
          }}
        >
          {s.is_active ? "Desativar" : "Ativar"}
        </button>
        <ConfirmButton
          variant="danger"
          label="Remover"
          confirmLabel="Confirmar remoção"
          question={`Remover "${s.name}" e os componentes importados?`}
          onConfirm={async () => {
            await adminApi.external.remove(s.id);
            onChanged();
          }}
        />
      </div>}
      {syncError && (
        <p className="field-error" role="alert">
          {syncError}
        </p>
      )}
      {open && <SelectionEditor source={s} onSaved={onChanged} />}
    </li>
  );
}

export function ThirdPartyPage() {
  const { state, reload } = useApi((signal) => adminApi.external.list({ signal }), []);
  const [lastCreated, setLastCreated] = useState<string | null>(null);
  const canManage = useAuth().can("third_party.manage");
  return (
    <div className="admin-content">
      <PageHeader title="Serviços de terceiros" />
      <p className="muted">
        Incorpore o status de serviços de que a plataforma depende (GitHub, Veracode e outras páginas Atlassian Statuspage). O
        status é lido periodicamente da página de cada provedor, exibido em seção própria e não altera o status geral.
      </p>
      {canManage ? (
        <section className="panel" aria-label="Novo provedor">
          <AddSourceForm
            onCreated={(s) => {
              setLastCreated(s.id);
              reload();
            }}
          />
        </section>
      ) : (
        <ReadOnlyNote what="cadastrar ou alterar provedores" />
      )}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} />}
      {state.status === "success" &&
        (state.data.data.length === 0 ? (
          <EmptyState title="Nenhum provedor cadastrado" />
        ) : (
          <ul className="admin-list">
            {state.data.data.map((s) => (
              <SourceItem key={s.id} source={s} onChanged={reload} startOpen={s.id === lastCreated} />
            ))}
          </ul>
        ))}
    </div>
  );
}
