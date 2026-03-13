package dashboard

import (
	"database/sql"
	"net/http"

	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/dashboard/views"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// Handlers holds dependencies for dashboard HTTP handlers.
type Handlers struct {
	db    *queries.Queries
	rawDB *sql.DB
	cfg   *app.Config
}

// NewHandlers creates a new Handlers with the given dependencies.
func NewHandlers(db *queries.Queries, rawDB *sql.DB, cfg *app.Config) *Handlers {
	return &Handlers{
		db:    db,
		rawDB: rawDB,
		cfg:   cfg,
	}
}

// HandlePipelineStatus renders the pipeline status page.
func (h *Handlers) HandlePipelineStatus(w http.ResponseWriter, r *http.Request) {
	views.PipelineStatusPage().Render(r.Context(), w)
}

// HandleDataPreview renders the data preview page.
func (h *Handlers) HandleDataPreview(w http.ResponseWriter, r *http.Request) {
	views.DataPreviewPage().Render(r.Context(), w)
}

// HandleConfig renders the configuration page.
func (h *Handlers) HandleConfig(w http.ResponseWriter, r *http.Request) {
	views.ConfigPage().Render(r.Context(), w)
}

// HandleMappings renders the mapping overview page.
func (h *Handlers) HandleMappings(w http.ResponseWriter, r *http.Request) {
	views.MappingsPage().Render(r.Context(), w)
}
