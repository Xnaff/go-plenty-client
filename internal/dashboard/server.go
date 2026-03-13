package dashboard

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed static
var staticFS embed.FS

// NewRouter creates a chi router with middleware, static file serving, and page routes.
func NewRouter(h *Handlers) chi.Router {
	r := chi.NewRouter()

	// Middleware stack.
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Serve static files from embedded filesystem.
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/",
		http.FileServer(http.FS(staticSub))))

	// Dashboard page routes.
	r.Get("/", h.HandlePipelineStatus)
	r.Get("/preview", h.HandleDataPreview)
	r.Get("/config", h.HandleConfig)
	r.Get("/mappings", h.HandleMappings)

	return r
}
