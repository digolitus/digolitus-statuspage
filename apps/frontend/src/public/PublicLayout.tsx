import type { ReactNode } from "react";
import { useUnseenAnnouncements } from "../lib/announcementsSeen";
import { Link, useLocation } from "../lib/router";
import { SubscribeMenu } from "./Subscribe";

function AnnouncementsLink() {
  const unseen = useUnseenAnnouncements();
  const { pathname } = useLocation();
  const current = pathname === "/announcements" || pathname.startsWith("/announcements/");
  return (
    <Link to="/announcements" className="nav-announcements" aria-current={current ? "page" : undefined}>
      Comunicados
      {unseen > 0 && (
        <span className="nav-badge">
          <span aria-hidden="true">{unseen > 9 ? "9+" : unseen}</span>
          <span className="visually-hidden">{unseen === 1 ? "(1 novo)" : `(${unseen} novos)`}</span>
        </span>
      )}
    </Link>
  );
}

export function PublicLayout({ children }: { children: ReactNode }) {
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        Pular para o conteúdo
      </a>
      <header className="site-header">
        <div className="container header-content">
          <Link className="brand" to="/" aria-label="Digolitus Status Page — início">
            <span className="brand-mark" aria-hidden="true">
              D
            </span>
            <span className="brand-name">Digolitus Status Page</span>
          </Link>
          <nav className="main-navigation" aria-label="Navegação principal">
            <a href="/#components">Componentes</a>
            <a href="/#history">Histórico</a>
            <AnnouncementsLink />
          </nav>
          <SubscribeMenu />
        </div>
      </header>
      <main id="main">{children}</main>
      <footer className="site-footer">
        <div className="container footer-content">
          <span>Digolitus Status Page</span>
          <span>Status informado pela equipe de operações.</span>
        </div>
      </footer>
    </div>
  );
}
