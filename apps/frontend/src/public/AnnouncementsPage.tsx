import { useEffect, useState } from "react";
import { publicApi } from "../api/endpoints";
import type { PublicAnnouncement } from "../api/types";
import { excerpt, RichText } from "../components/RichText";
import { EmptyState, ErrorPanel, Loading, Pager } from "../components/ui";
import { markSeen } from "../lib/announcementsSeen";
import { dayKey, dayLabel, formatDateTime } from "../lib/format";
import { Link } from "../lib/router";
import { useApi } from "../lib/useApi";

const PAGE = 15;
/** Bodies longer than this show an excerpt with a link to the full text. */
const LONG_BODY = 600;

function groupByDay(items: PublicAnnouncement[]): { key: string; label: string; items: PublicAnnouncement[] }[] {
  const groups: { key: string; label: string; items: PublicAnnouncement[] }[] = [];
  for (const a of items) {
    const key = dayKey(a.published_at);
    const last = groups[groups.length - 1];
    if (last && last.key === key) last.items.push(a);
    else groups.push({ key, label: dayLabel(a.published_at), items: [a] });
  }
  return groups;
}

function timeOnly(iso: string): string {
  return new Intl.DateTimeFormat("pt-BR", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(iso));
}

/** Small metadata line: category, importance, team and components. */
function AnnouncementMeta({ a }: { a: PublicAnnouncement }) {
  return (
    <div className="announcement-meta">
      {a.is_pinned && <span className="announcement-flag">Fixado</span>}
      {a.importance === "important" && <span className="announcement-flag announcement-flag-important">Importante</span>}
      {a.category && <span className={`category-chip category-${a.category.color}`}>{a.category.name}</span>}
      {a.team && <span className="muted small">Por {a.team.name}</span>}
      {a.components.length > 0 && <span className="muted small">· {a.components.map((c) => c.name).join(", ")}</span>}
    </div>
  );
}

function AnnouncementCard({ a, full = false, showDate = false }: { a: PublicAnnouncement; full?: boolean; showDate?: boolean }) {
  const long = !full && a.body.length > LONG_BODY;
  const edited = new Date(a.updated_at).getTime() - new Date(a.published_at).getTime() > 60_000;
  return (
    <article className={`announcement-card${a.importance === "important" ? " announcement-important" : ""}`} aria-labelledby={`a-${a.id}`}>
      <AnnouncementMeta a={a} />
      {full ? (
        <h1 id={`a-${a.id}`} className="announcement-title">
          {a.title}
        </h1>
      ) : (
        <h3 id={`a-${a.id}`} className="announcement-title">
          <Link to={`/announcements/${a.id}`}>{a.title}</Link>
        </h3>
      )}
      {long ? (
        <p className="announcement-excerpt">
          {excerpt(a.body, LONG_BODY)} <Link to={`/announcements/${a.id}`}>Ler comunicado completo</Link>
        </p>
      ) : (
        <RichText text={a.body} />
      )}
      <p className="announcement-time muted small">
        <time dateTime={a.published_at}>{showDate ? formatDateTime(a.published_at) : timeOnly(a.published_at)}</time>
        {edited && <> · atualizado em {formatDateTime(a.updated_at)}</>}
      </p>
    </article>
  );
}

function CategoryFilter({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { state } = useApi((signal) => publicApi.announcementCategories({ signal }), []);
  if (state.status !== "success" || state.data.data.length === 0) return null;
  const options = [{ id: "", name: "Todas", color: "gray" as const }, ...state.data.data];
  return (
    <div className="filters" role="group" aria-label="Filtrar por categoria">
      {options.map((c) => (
        <button
          key={c.id || "all"}
          type="button"
          className={value === c.id ? "chip-button chip-button-active" : "chip-button"}
          aria-pressed={value === c.id}
          onClick={() => onChange(c.id)}
        >
          {c.id && <span className={`category-dot category-${c.color}`} aria-hidden="true" />}
          {c.name}
        </button>
      ))}
    </div>
  );
}

export function AnnouncementsPage() {
  const [category, setCategory] = useState("");
  const [offset, setOffset] = useState(0);
  const pinned = useApi(
    (signal) => publicApi.announcements({ pinned: true, category_id: category || undefined, limit: 5 }, { signal }),
    [category],
  );
  const timeline = useApi(
    (signal) => publicApi.announcements({ pinned: false, category_id: category || undefined, limit: PAGE, offset }, { signal }),
    [category, offset],
  );

  // Opening the page marks what was loaded as seen (newest publication).
  const newest = [pinned.state.data?.data[0]?.published_at, offset === 0 ? timeline.state.data?.data[0]?.published_at : undefined]
    .filter((d): d is string => Boolean(d))
    .sort()
    .pop();
  useEffect(() => {
    if (newest) markSeen(newest);
  }, [newest]);

  const pinnedItems = pinned.state.data?.data ?? [];
  return (
    <section className="container page-section" aria-labelledby="announcements-title">
      <h1 id="announcements-title" className="page-title">
        Comunicados
      </h1>
      <p className="section-note">Avisos gerais da equipe: novidades, mudanças e lembretes. Incidentes e manutenções ficam na página de status.</p>
      <CategoryFilter
        value={category}
        onChange={(id) => {
          setCategory(id);
          setOffset(0);
        }}
      />

      {pinnedItems.length > 0 && offset === 0 && (
        <div className="announcement-day">
          <h2 className="announcement-day-title">Fixados</h2>
          <div className="announcement-list">
            {pinnedItems.map((a) => (
              <AnnouncementCard key={a.id} a={a} showDate />
            ))}
          </div>
        </div>
      )}

      {timeline.state.status === "loading" && <Loading label="Carregando comunicados…" />}
      {timeline.state.status === "error" && <ErrorPanel error={timeline.state.error} onRetry={timeline.reload} />}
      {timeline.state.status === "success" &&
        (timeline.state.data.data.length === 0 && pinnedItems.length === 0 ? (
          <EmptyState title="Nenhum comunicado publicado">{category ? "Não há comunicados nesta categoria." : undefined}</EmptyState>
        ) : (
          <>
            {groupByDay(timeline.state.data.data).map((g) => (
              <div key={g.key} className="announcement-day">
                <h2 className="announcement-day-title">{g.label}</h2>
                <div className="announcement-list">
                  {g.items.map((a) => (
                    <AnnouncementCard key={a.id} a={a} />
                  ))}
                </div>
              </div>
            ))}
            <Pager pagination={timeline.state.data.pagination} onChange={setOffset} />
          </>
        ))}
    </section>
  );
}

export function AnnouncementDetailPage({ id }: { id: string }) {
  const { state, reload } = useApi((signal) => publicApi.announcement(id, { signal }), [id]);
  return (
    <section className="container page-section">
      <p>
        <Link to="/announcements">← Todos os comunicados</Link>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorPanel error={state.error} onRetry={reload} title="Comunicado não encontrado ou indisponível" />}
      {state.status === "success" && <AnnouncementCard a={state.data} full showDate />}
    </section>
  );
}
