import { Component, type ErrorInfo, type ReactNode } from "react";

/** Shows a clear message instead of a blank page if rendering fails. */
export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Unexpected rendering error", error, info.componentStack);
  }

  render() {
    if (this.state.failed) {
      return (
        <div className="container page-section">
          <div className="error-panel" role="alert">
            <div>
              <strong>Ocorreu um erro inesperado ao exibir esta página.</strong>
              <span>Recarregue a página. Se o problema persistir, contate a equipe responsável.</span>
            </div>
            <button type="button" className="secondary-button" onClick={() => window.location.reload()}>
              Recarregar
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
