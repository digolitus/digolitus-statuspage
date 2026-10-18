// Kept as an external file so the page works under a CSP without
// 'unsafe-inline' scripts.
window.addEventListener("load", function () {
  window.ui = SwaggerUIBundle({
    url: "/api/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis],
    layout: "BaseLayout",
    validatorUrl: null,
    tryItOutEnabled: false,
  });
});
