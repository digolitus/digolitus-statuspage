// Package docs serves the Swagger UI for the OpenAPI specification.
//
// The swaggerui directory vendors the official swagger-ui dist files
// (swagger-api/swagger-ui, Apache-2.0, see swaggerui/LICENSE and NOTICE) so
// the documentation works offline and without a CDN. VERSION records the
// upstream release; update with scripts/update-swagger-ui.sh.
package docs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed swaggerui
var files embed.FS

// csp allows only same-origin scripts; Swagger UI needs inline styles and
// data: images for its icons.
const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Handler serves the Swagger UI under /swagger/.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "swaggerui")
	if err != nil {
		panic(err)
	}
	static := http.StripPrefix("/swagger/", http.FileServerFS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/swagger" {
			http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
			return
		}
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	})
}
