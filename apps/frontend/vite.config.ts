import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Development only: forwards API and docs calls to a backend started with
  // `go run ./cmd/api` (PORT=8080), mirroring the Nginx routes.
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/swagger": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
      "/readyz": "http://localhost:8080",
    },
  },
});
