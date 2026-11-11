import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
// Fonts offered in the visual identity settings, bundled (CSP: font-src
// 'self'). Browsers download a file only when a page uses that font.
import "@fontsource-variable/inter";
import "@fontsource-variable/ibm-plex-sans";
import "@fontsource-variable/source-sans-3";
import "@fontsource-variable/nunito-sans";
import "@fontsource-variable/roboto";
import "@fontsource-variable/merriweather-sans";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>,
);
