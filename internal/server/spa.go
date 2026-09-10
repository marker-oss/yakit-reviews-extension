package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:admin_dist
var adminDistFS embed.FS

// adminSPAHandler serves the embedded SPA, falling back to index.html for
// client-side routes that do not match a built asset.
func (s *Server) adminSPAHandler() http.Handler {
	sub, err := fs.Sub(adminDistFS, "admin_dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/admin/")
		if trimmed == "" {
			trimmed = "index.html"
		}
		if _, err := fs.Stat(sub, trimmed); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/admin/index.html"
			http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r2)
			return
		}
		http.StripPrefix("/admin/", fileServer).ServeHTTP(w, r)
	})
}
