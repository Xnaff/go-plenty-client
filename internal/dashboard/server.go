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
func NewRouter(h *Handlers, broker *Broker) chi.Router {
	r := chi.NewRouter()

	// Middleware stack (applied to all routes).
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// SSE endpoint must NOT be wrapped by Compress middleware (buffering breaks SSE).
	r.Get("/events", broker.ServeHTTP)

	// All other routes get compression.
	r.Group(func(r chi.Router) {
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

		// API routes for HTMX partial updates.
		r.Get("/api/pipeline/{runID}/stages", h.HandleStagesFragment)
		r.Get("/api/preview/{productID}", h.HandleProductDetail)
		r.Get("/api/mappings/filter", h.HandleMappingsFilter)

		// Config API routes.
		r.Post("/api/config", h.HandleConfigUpdate)
		r.Get("/api/config/edit/*", h.HandleConfigEdit)
		r.Get("/api/config/cancel/*", h.HandleConfigCancel)

		// Bulk action API routes.
		r.Post("/api/mappings/retry", h.HandleBulkRetry)
		r.Post("/api/mappings/skip", h.HandleBulkSkip)
	})

	return r
}
