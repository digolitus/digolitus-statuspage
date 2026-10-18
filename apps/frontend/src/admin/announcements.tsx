import { useState, type FormEvent } from "react";
import { adminApi } from "../api/endpoints";
import type { Announcement, AnnouncementCategory, AnnouncementCreate, AnnouncementImportance, AnnouncementState, AnnouncementTemplate, Team } from "../api/types";
import { MarkdownEditor } from "../components/MarkdownEditor";
import { RichText } from "../components/RichText";
import { Badge, ConfirmButton, EmptyState, ErrorPanel, Field, Loading, Pager } from "../components/ui";
import { formatDateTime, type Tone } from "../lib/format";
import { Link, navigate } from "../lib/router";
import { useApi } from "../lib/useApi";
import { useAuth } from "./authContext";
import { ComponentPicker, FormStatus, PageHeader, ReadOnlyNote, requireText, TemplateSelect, useSubmit } from "./shared";

const announcementStateLabel: Record<AnnouncementState, string> = {
  draft: "Rascunho",
  scheduled: "Agendado",
  published: "Publicado",
  archived: "Arquivado",
};

const stateTone: Record<AnnouncementState, Tone> = {
  draft: "neutral",
  scheduled: "info",
  published: "ok",
  archived: "neutral",
};

const FILTERS: [AnnouncementState | "", string][] = [
  ["", "Todos"],
  ["draft", "Rascunhos"],
  ["scheduled", "Agendados"],
  ["published", "Publicados"],
  ["archived", "Arquivados"],
];

/** Active categories and teams for the editor (plus ones already in use). */
function useTaxonomy() {
  return useApi(async (signal) => {
    const [categories, teams] = await Promise.all([
      adminApi.announcementCategories.list({}, { signal }),
      adminApi.teams.list({}, { signal }),
    ]);
    return { categories: categories.data, teams: teams.data };
  }, []);
}

export function AnnouncementsPage() {
  const [stateFilter, setStateFilter] = useState<AnnouncementState | "">("");
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [offset, setOffset] = useState(0);
  const taxonomy = useTaxonomy();
  const list = useApi(
    (signal) =>
      adminApi.announcements.list(
        { state: stateFilter || undefined, q: search.trim() || undefined, category_id: category || undefined, limit: 20, offset },
        { signal },
      ),
    [stateFilter, search, category, offset],
  );
  const categories = taxonomy.state.data?.categories ?? [];
  const canWrite = useAuth().can("announcements.write");

  return (
    <div className="admin-content">
      <PageHeader title="Comunicados">
        <Link className="secondary-button" to="/admin/announcement-settings">
          Categorias
        </Link>
        {canWrite && (
          <Link className="primary-button" to="/admin/announcements/new">
            Novo comunicado
          </Link>
        )}
      </PageHeader>
      {!canWrite && <ReadOnlyNote what="escrever comunicados" />}
      <p className="muted">
        Avisos gerais para os clientes, que não são incidentes nem manutenções. Comunicados não alteram nenhum status e
        aparecem em <Link to="/announcements">/announcements</Link>, com aviso de novidade no menu da página pública.
      </p>
      <div className="filters" role="group" aria-label="Filtrar por estado">
        {FILTERS.map(([value, label]) => (
          <button
            key={label}
            type="button"
            aria-pressed={stateFilter === value}
            className={stateFilter === value ? "chip-button chip-button-active" : "chip-button"}
            onClick={() => {
              setStateFilter(value);
              setOffset(0);
            }}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="toolbar">
        <label className="inline-filter">
          Buscar no título{" "}
          <input
            type="search"
            value={search}
            maxLength={100}
            onChange={(e) => {
              setSearch(e.target.value);
              setOffset(0);
            }}
          />
        </label>
        {categories.length > 0 && (
          <label className="inline-filter">
            Categoria{" "}
            <select
              value={category}
              onChange={(e) => {
                setCategory(e.target.value);
                setOffset(0);
              }}
            >
              <option value="">Todas</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      {list.state.status === "loading" && <Loading />}
      {list.state.status === "error" && <ErrorPanel error={list.state.error} onRetry={list.reload} />}
      {list.state.status === "success" &&
        (list.state.data.data.length === 0 ? (
          <EmptyState title="Nenhum comunicado encontrado" />
        ) : (
          <>
            <ul className="admin-list">
              {list.state.data.data.map((a) => (
                <li key={a.id} className="admin-item">
                  <div className="admin-item-head">
                    <div>
                      <Link to={`/admin/announcements/${a.id}`}>
                        <strong>{a.title}</strong>
                      </Link>
                      <div className="admin-item-tags">
                        {a.category && <span className={`category-chip category-${a.category.color}`}>{a.category.name}</span>}
                        {a.team && <span className="tag">Time: {a.team.name}</span>}
                        {a.importance === "important" && <span className="tag tag-warn">Importante</span>}
                        {a.is_pinned && <span className="tag">Fixado</span>}
                        {a.published_at && (
                          <span className="tag">
                            {a.state === "scheduled" ? "Publica em" : "Publicado em"} {formatDateTime(a.published_at)}
                          </span>
                        )}
                        {a.state === "draft" && <span className="tag">Editado em {formatDateTime(a.updated_at)}</span>}
                      </div>
                    </div>
                    <Badge tone={stateTone[a.state]}>{announcementStateLabel[a.state]}</Badge>
                  </div>
                </li>
              ))}
            </ul>
            <Pager pagination={list.state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </div>
  );
}

interface FormValues {
  title: string;
  body: string;
  importance: AnnouncementImportance;
  is_pinned: boolean;
  category_id: string;
  team_id: string;
  component_ids: string[];
}

const EMPTY: FormValues = { title: "", body: "", importance: "info", is_pinned: false, category_id: "", team_id: "", component_ids: [] };

function toValues(a: Announcement): FormValues {
  return {
    title: a.title,
    body: a.body,
    importance: a.importance,
    is_pinned: a.is_pinned,
    category_id: a.category?.id ?? "",
    team_id: a.team?.id ?? "",
    component_ids: a.components.map((c) => c.id),
  };
}

/** Copies title, text, category and team from a previous announcement. */
function CopyFrom({ onApply }: { onApply: (a: Announcement) => void }) {
  const { state } = useApi((signal) => adminApi.announcements.list({ sort: "-updated_at", limit: 30 }, { signal }), []);
  if (state.status !== "success" || state.data.data.length === 0) return null;
  return (
    <div className="template-select">
      <label htmlFor="copy-from">Usar um comunicado anterior como base</label>
      <select
        id="copy-from"
        value=""
        onChange={(e) => {
          const a = state.data.data.find((x) => x.id === e.target.value);
          if (a) onApply(a);
        }}
      >
        <option value="">Escolha um comunicado…</option>
        {state.data.data.map((a) => (
          <option key={a.id} value={a.id}>
            {a.title} ({announcementStateLabel[a.state].toLowerCase()})
          </option>
        ))}
      </select>
      <small className="field-hint">Copia título, texto, categoria, time e componentes para este formulário. Nada é publicado.</small>
    </div>
  );
}

function optionsWithCurrent<T extends { id: string; name: string; is_active: boolean }>(items: T[], current: string) {
  return items.filter((i) => i.is_active || i.id === current);
}

function AnnouncementForm({
  initial,
  existing,
  categories,
  teams,
  onSaved,
}: {
  initial: FormValues;
  existing?: Announcement;
  categories: AnnouncementCategory[];
  teams: Team[];
  onSaved: (a: Announcement) => void;
}) {
  const [v, setV] = useState(initial);
  const submit = useSubmit();
  const templates = useApi((signal) => adminApi.announcementTemplates.list({ is_active: true, limit: 100 }, { signal }), []).state;
  const applyTemplate = (t: AnnouncementTemplate) =>
    setV((s) => ({
      ...s,
      title: t.title,
      body: t.body,
      importance: t.importance,
      category_id: t.category_id && categories.some((c) => c.id === t.category_id && c.is_active) ? t.category_id : s.category_id,
    }));
  const set = <K extends keyof FormValues>(k: K, value: FormValues[K]) => setV((s) => ({ ...s, [k]: value }));
  const category = categories.find((c) => c.id === v.category_id);

  async function handle(e: FormEvent) {
    e.preventDefault();
    const errors: Record<string, string> = {};
    requireText(errors, "title", v.title, 200, "Título");
    requireText(errors, "body", v.body, 10000, "Texto");
    if (!errors.body && /\]\(https?:\/\/\)/.test(v.body)) errors.body = "Complete o endereço do link (https://...) ou remova-o.";
    if (Object.keys(errors).length) {
      submit.setFieldErrors(errors);
      return;
    }
    const body: AnnouncementCreate = {
      title: v.title.trim(),
      body: v.body.trim(),
      importance: v.importance,
      is_pinned: v.is_pinned,
      category_id: v.category_id || null,
      team_id: v.team_id || null,
      component_ids: v.component_ids,
    };
    const saved = await submit.run(
      () => (existing ? adminApi.announcements.update(existing.id, body) : adminApi.announcements.create(body)),
      existing ? "Alterações salvas." : undefined,
    );
    if (saved) onSaved(saved);
  }

  const fe = submit.fieldErrors;
  return (
    <form className="form" onSubmit={handle} noValidate aria-label="Comunicado">
      {!existing && templates.status === "success" && templates.data.data.length > 0 && (
        <TemplateSelect templates={templates.data.data} onApply={applyTemplate} label="Usar template de comunicado" />
      )}
      {!existing && (
        <CopyFrom
          onApply={(a) =>
            setV({
              ...toValues(a),
              importance: a.importance,
              is_pinned: false,
              category_id: categories.some((c) => c.id === a.category?.id && c.is_active) ? (a.category?.id ?? "") : "",
              team_id: teams.some((t) => t.id === a.team?.id && t.is_active) ? (a.team?.id ?? "") : "",
            })
          }
        />
      )}
      <Field label="Título" error={fe.title} hint={`${v.title.length}/200`}>
        {(p) => <input {...p} value={v.title} maxLength={200} onChange={(e) => set("title", e.target.value)} />}
      </Field>
      <div className="form-grid">
        <Field label="Categoria" error={fe.category_id} hint={category?.description || undefined}>
          {(p) => (
            <select {...p} value={v.category_id} onChange={(e) => set("category_id", e.target.value)}>
              <option value="">Sem categoria</option>
              {optionsWithCurrent(categories, initial.category_id).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                  {c.is_active ? "" : " (desativada)"}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Time responsável" error={fe.team_id}>
          {(p) => (
            <select {...p} value={v.team_id} onChange={(e) => set("team_id", e.target.value)}>
              <option value="">Não informar</option>
              {optionsWithCurrent(teams, initial.team_id).map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                  {t.is_active ? "" : " (desativado)"}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>
      <fieldset className="field">
        <legend>Destaque</legend>
        <div className="picker-options picker-inline">
          <label className="checkbox">
            <input type="radio" name="importance" checked={v.importance === "info"} onChange={() => set("importance", "info")} />
            Informativo
          </label>
          <label className="checkbox">
            <input type="radio" name="importance" checked={v.importance === "important"} onChange={() => set("importance", "important")} />
            Importante (destacado na página)
          </label>
          <label className="checkbox">
            <input type="checkbox" checked={v.is_pinned} onChange={(e) => set("is_pinned", e.target.checked)} />
            Fixar no topo dos comunicados
          </label>
        </div>
      </fieldset>

      <div className={`field${fe.body ? " field-invalid" : ""}`}>
        <MarkdownEditor
          id="announcement-body"
          label="Texto"
          value={v.body}
          onChange={(b) => set("body", b)}
          invalid={!!fe.body}
          describedBy={fe.body ? "announcement-body-error" : undefined}
        />
        {fe.body && (
          <small id="announcement-body-error" className="field-error">
            {fe.body}
          </small>
        )}
      </div>

      <ComponentPicker permission="announcements.write"
        legend="Componentes relacionados (opcional)"
        selected={v.component_ids}
        onChange={(ids) => set("component_ids", ids)}
        keep={existing?.components}
        error={fe.component_ids}
      />
      <FormStatus error={submit.formError} success={submit.success} />
      <div className="form-actions">
        <button type="submit" className="primary-button" disabled={submit.busy}>
          {submit.busy ? "Salvando…" : existing ? "Salvar alterações" : "Salvar rascunho"}
        </button>
      </div>
    </form>
  );
}

/** Converts a datetime-local value (viewer's zone) to RFC 3339 with offset. */
function localToISO(value: string): string | null {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

function Lifecycle({ a, onChanged }: { a: Announcement; onChanged: (a: Announcement) => void }) {
  const { canFor } = useAuth();
  const canPublish = canFor("announcements.publish", a.team?.id);
  const canWrite = canFor("announcements.write", a.team?.id);
  const [when, setWhen] = useState("");
  const submit = useSubmit();
  const run = (fn: () => Promise<Announcement>, msg: string) =>
    submit.run(fn, msg).then((r) => {
      if (r) onChanged(r);
    });

  return (
    <section className="panel" aria-label="Publicação">
      <h2>Publicação</h2>
      <p>
        <Badge tone={stateTone[a.state]}>{announcementStateLabel[a.state]}</Badge>{" "}
        {a.state === "scheduled" && <>Será publicado em {formatDateTime(a.published_at)}.</>}
        {a.state === "published" && (
          <>
            Publicado em {formatDateTime(a.published_at)}. <Link to={`/announcements/${a.id}`}>Ver na página pública</Link>
          </>
        )}
        {a.state === "archived" && <>Arquivado em {formatDateTime(a.archived_at)}; não aparece na página pública.</>}
        {a.state === "draft" && <>Visível apenas na administração.</>}
      </p>
      {!canPublish && <ReadOnlyNote what="publicar, agendar, arquivar ou despublicar comunicados" />}
      {a.state === "draft" && (canPublish || canWrite) && (
        <div className="form-actions">
          {canPublish && <>
          <ConfirmButton
            label="Publicar agora"
            confirmLabel="Confirmar publicação"
            question="Publicar este comunicado para todos os visitantes?"
            onConfirm={() => run(() => adminApi.announcements.publish(a.id), "Comunicado publicado.")}
          />
          <label className="inline-filter">
            Agendar para{" "}
            <input type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} aria-describedby="schedule-hint" />
          </label>
          <button
            type="button"
            className="secondary-button"
            disabled={!when || submit.busy}
            onClick={() => {
              const iso = localToISO(when);
              if (!iso) return submit.setFieldErrors({ publish_at: "Data inválida." });
              void run(() => adminApi.announcements.publish(a.id, iso), "Publicação agendada.");
            }}
          >
            Agendar
          </button>
          </>}
          {canWrite && <ConfirmButton
            variant="danger"
            label="Excluir rascunho"
            confirmLabel="Confirmar exclusão"
            question="Excluir este rascunho definitivamente?"
            onConfirm={async () => {
              await adminApi.announcements.remove(a.id);
              navigate("/admin/announcements");
            }}
          />}
        </div>
      )}
      {a.state === "draft" && canPublish && (
        <small id="schedule-hint" className="field-hint">
          O horário de agendamento usa o fuso deste navegador ({Intl.DateTimeFormat().resolvedOptions().timeZone}).
        </small>
      )}
      {canPublish && (a.state === "scheduled" || a.state === "published") && (
        <div className="form-actions">
          {a.state === "published" && (
            <ConfirmButton
              variant="secondary"
              label="Arquivar"
              confirmLabel="Confirmar arquivamento"
              question="Remover da página pública e manter o registro?"
              onConfirm={() => run(() => adminApi.announcements.transition(a.id, "archive"), "Comunicado arquivado.")}
            />
          )}
          <ConfirmButton
            variant="secondary"
            label={a.state === "scheduled" ? "Cancelar agendamento" : "Despublicar (voltar a rascunho)"}
            confirmLabel="Confirmar"
            question={a.state === "scheduled" ? "Cancelar o agendamento e voltar a rascunho?" : "Remover da página pública e voltar a rascunho?"}
            onConfirm={() => run(() => adminApi.announcements.transition(a.id, "unpublish"), "Comunicado voltou a rascunho.")}
          />
        </div>
      )}
      {canPublish && a.state === "archived" && (
        <div className="form-actions">
          <button
            type="button"
            className="secondary-button"
            disabled={submit.busy}
            onClick={() => run(() => adminApi.announcements.transition(a.id, "restore"), "Comunicado restaurado.")}
          >
            Restaurar publicação
          </button>
        </div>
      )}
      {submit.fieldErrors.publish_at && <p className="field-error">{submit.fieldErrors.publish_at}</p>}
      <FormStatus error={submit.formError} success={submit.success} />
    </section>
  );
}

export function AnnouncementNewPage() {
  const taxonomy = useTaxonomy();
  return (
    <div className="admin-content">
      <PageHeader title="Novo comunicado" />
      <p>
        <Link to="/admin/announcements">← Comunicados</Link>
      </p>
      {taxonomy.state.status === "loading" && <Loading />}
      {taxonomy.state.status === "error" && <ErrorPanel error={taxonomy.state.error} onRetry={taxonomy.reload} />}
      {taxonomy.state.status === "success" && (
        <>
          {taxonomy.state.data.categories.length === 0 && (
            <p className="muted small">
              Dica: crie <Link to="/admin/announcement-settings">categorias</Link> e <Link to="/admin/teams">times</Link> para organizar os comunicados.
            </p>
          )}
          <AnnouncementForm
            initial={EMPTY}
            categories={taxonomy.state.data.categories}
            teams={taxonomy.state.data.teams}
            onSaved={(a) => navigate(`/admin/announcements/${a.id}`)}
          />
        </>
      )}
    </div>
  );
}

export function AnnouncementDetailPage({ id }: { id: string }) {
  const taxonomy = useTaxonomy();
  const { canFor } = useAuth();
  const item = useApi((signal) => adminApi.announcements.get(id, { signal }), [id]);
  const canWrite = item.state.status === "success" && canFor("announcements.write", item.state.data.team?.id);
  return (
    <div className="admin-content">
      <PageHeader title="Comunicado" />
      <p>
        <Link to="/admin/announcements">← Comunicados</Link>
      </p>
      {(item.state.status === "loading" || taxonomy.state.status === "loading") && <Loading />}
      {item.state.status === "error" && <ErrorPanel error={item.state.error} onRetry={item.reload} />}
      {taxonomy.state.status === "error" && <ErrorPanel error={taxonomy.state.error} onRetry={taxonomy.reload} />}
      {item.state.status === "success" && taxonomy.state.status === "success" && (
        <>
          <Lifecycle a={item.state.data} onChanged={item.setData} />
          {canWrite ? (
          <AnnouncementForm
            key={item.state.data.id}
            initial={toValues(item.state.data)}
            existing={item.state.data}
            categories={taxonomy.state.data.categories}
            teams={taxonomy.state.data.teams}
            onSaved={item.setData}
          />
          ) : (
            <section className="panel" aria-label="Conteúdo">
              <ReadOnlyNote what="editar o texto de comunicados" />
              <h2>{item.state.data.title}</h2>
              <RichText text={item.state.data.body} />
            </section>
          )}
        </>
      )}
    </div>
  );
}
