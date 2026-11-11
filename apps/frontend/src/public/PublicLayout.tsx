import { useRef, type ReactNode } from "react";
import { BrandMark } from "../components/Branding";
import { useBranding, useBrandingVars } from "../lib/branding";
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
  const { branding } = useBranding();
  const ref = useRef<HTMLDivElement>(null);
  useBrandingVars(ref, branding);
  return (
    <div className="app branded" ref={ref}>
      <a className="skip-link" href="#main">
        Pular para o conteúdo
      </a>
      <header className="site-header">
        <div className="container header-content">
          <Link className="brand" to="/" aria-label={`${branding.title} — início`}>
            <BrandMark branding={branding} />
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
          <span>{branding.title}</span>
          {(branding.footer_text || branding.footer_link) && (
            <span className="footer-text">
              {branding.footer_text}
              {branding.footer_link && (
                <>
                  {branding.footer_text && " "}
                  <a href={branding.footer_link.url} rel="noopener noreferrer">
                    {branding.footer_link.label}
                  </a>
                </>
              )}
            </span>
          )}
        </div>
      </footer>
    </div>
  );
}
