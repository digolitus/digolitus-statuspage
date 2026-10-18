import type { ReactNode } from "react";
import { Link } from "../lib/router";

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
            <a href="/#incidents">Incidentes</a>
            <a href="/#maintenances">Manutenções</a>
            <a href="/#history">Histórico</a>
          </nav>
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
