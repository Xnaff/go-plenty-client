package dashboard

import (
	"context"
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

// HandleDataPreview renders the data preview page with job selection and product list.
func (h *Handlers) HandleDataPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobs, err := h.db.ListRecentJobs(ctx, 50)
	if err != nil {
		slog.Error("listing recent jobs", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var selectedJob *queries.Job
	var products []queries.Product

	jobIDStr := r.URL.Query().Get("job_id")
	if jobIDStr != "" {
		jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
		if err == nil {
			job, err := h.db.GetJob(ctx, jobID)
			if err == nil {
				selectedJob = &job
				products, err = h.db.ListProductsByJob(ctx, jobID)
				if err != nil {
					slog.Error("listing products by job", "job_id", jobID, "error", err)
					products = nil
				}
			}
		}
	}

	// If this is an HTMX request (job selector changed), return just the products fragment.
	if r.Header.Get("HX-Request") == "true" && jobIDStr != "" {
		if selectedJob == nil || len(products) == 0 {
			if selectedJob == nil {
				views.EmptyState("Select a job to preview generated products.").Render(ctx, w)
			} else {
				views.EmptyState("No products found for this job.").Render(ctx, w)
			}
			return
		}
		views.PreviewProductsFragment(products).Render(ctx, w)
		return
	}

	views.DataPreviewPage(jobs, selectedJob, products).Render(ctx, w)
}

// HandleProductDetail renders the product detail fragment for HTMX partial loading.
func (h *Handlers) HandleProductDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	productIDStr := chi.URLParam(r, "productID")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := h.db.GetProduct(ctx, productID)
	if err != nil {
		slog.Error("getting product", "product_id", productID, "error", err)
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	variations, err := h.db.ListVariationsByProduct(ctx, productID)
	if err != nil {
		slog.Error("listing variations", "product_id", productID, "error", err)
		variations = nil
	}

	images, err := h.db.ListImagesByProduct(ctx, productID)
	if err != nil {
		slog.Error("listing images", "product_id", productID, "error", err)
		images = nil
	}

	texts, err := h.db.ListTextsByProduct(ctx, productID)
	if err != nil {
		slog.Error("listing texts", "product_id", productID, "error", err)
		texts = nil
	}

	views.ProductDetailFragment(product, variations, images, texts).Render(ctx, w)
}

// HandleConfig renders the configuration page.
func (h *Handlers) HandleConfig(w http.ResponseWriter, r *http.Request) {
	views.ConfigPage().Render(r.Context(), w)
}

// HandleMappings renders the mapping overview page with filtering.
func (h *Handlers) HandleMappings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	selectedType := r.URL.Query().Get("entity_type")
	selectedStatus := r.URL.Query().Get("status")

	mappings, err := h.queryMappings(ctx, selectedType, selectedStatus)
	if err != nil {
		slog.Error("querying mappings", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	views.MappingsPage(mappings, views.EntityTypeOptions, views.StatusOptions, selectedType, selectedStatus).Render(ctx, w)
}

// HandleMappingsFilter renders just the mapping table for HTMX partial updates.
func (h *Handlers) HandleMappingsFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	selectedType := r.URL.Query().Get("entity_type")
	selectedStatus := r.URL.Query().Get("status")

	mappings, err := h.queryMappings(ctx, selectedType, selectedStatus)
	if err != nil {
		slog.Error("querying mappings", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	views.MappingsTable(mappings).Render(ctx, w)
}

// queryMappings loads entity mappings for the latest pipeline run, with optional
// entity_type and status filters. Uses rawDB because the sqlc-generated query
// ListEntityMappingsByRun requires entity_type and doesn't support "all types".
func (h *Handlers) queryMappings(ctx context.Context, entityType, status string) ([]queries.EntityMapping, error) {
	// Find the latest pipeline run.
	jobs, err := h.db.ListRecentJobs(ctx, 50)
	if err != nil {
		return nil, err
	}

	var runID int64
	for _, job := range jobs {
		run, err := h.db.GetPipelineRunByJobLatest(ctx, job.ID)
		if err == nil {
			runID = run.ID
			break
		}
	}

	if runID == 0 {
		return nil, nil // No pipeline runs found.
	}

	// Build dynamic query with optional filters.
	query := "SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at FROM entity_mappings WHERE run_id = ?"
	args := []any{runID}

	if entityType != "" {
		query += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at LIMIT 500"

	rows, err := h.rawDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []queries.EntityMapping
	for rows.Next() {
		var m queries.EntityMapping
		if err := rows.Scan(
			&m.ID, &m.RunID, &m.LocalID, &m.PlentyID,
			&m.EntityType, &m.Stage, &m.Status, &m.ErrorMessage,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}
