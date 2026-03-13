package dashboard

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/dashboard/views"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// Handlers holds dependencies for dashboard HTTP handlers.
type Handlers struct {
	db     *queries.Queries
	rawDB  *sql.DB
	cfg    *app.Config
	broker *Broker
}

// NewHandlers creates a new Handlers with the given dependencies.
func NewHandlers(db *queries.Queries, rawDB *sql.DB, cfg *app.Config, broker *Broker) *Handlers {
	return &Handlers{
		db:     db,
		rawDB:  rawDB,
		cfg:    cfg,
		broker: broker,
	}
}

// HandlePipelineStatus renders the pipeline status page with recent job runs.
func (h *Handlers) HandlePipelineStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobs, err := h.db.ListRecentJobs(ctx, 20)
	if err != nil {
		slog.Error("listing recent jobs", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var runs []views.PipelineRunView
	for _, job := range jobs {
		run, err := h.db.GetPipelineRunByJobLatest(ctx, job.ID)
		if err != nil {
			// No pipeline run for this job; skip.
			continue
		}

		stages, err := h.db.ListStageStatesByRun(ctx, run.ID)
		if err != nil {
			slog.Error("listing stage states", "run_id", run.ID, "error", err)
			continue
		}

		failedCount, err := h.db.CountFailedByRun(ctx, run.ID)
		if err != nil {
			slog.Error("counting failed mappings", "run_id", run.ID, "error", err)
			failedCount = 0
		}

		runs = append(runs, views.PipelineRunView{
			Job:         job,
			Run:         run,
			Stages:      stages,
			FailedCount: failedCount,
		})
	}

	views.PipelineStatusPage(runs).Render(ctx, w)
}

// HandleStagesFragment renders just the stage progress section for HTMX partial updates.
func (h *Handlers) HandleStagesFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	runIDStr := chi.URLParam(r, "runID")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid run ID", http.StatusBadRequest)
		return
	}

	stages, err := h.db.ListStageStatesByRun(ctx, runID)
	if err != nil {
		slog.Error("listing stage states", "run_id", runID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	failedCount, err := h.db.CountFailedByRun(ctx, runID)
	if err != nil {
		slog.Error("counting failed mappings", "run_id", runID, "error", err)
		failedCount = 0
	}

	views.StageProgressFragment(stages, failedCount).Render(ctx, w)
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
