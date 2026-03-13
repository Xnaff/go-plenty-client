package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/dashboard/views"
	"github.com/janemig/plentyone/internal/storage/queries"
	"github.com/spf13/viper"
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

// sensitiveKeyNames defines leaf key names that contain sensitive values.
var sensitiveKeyNames = map[string]bool{
	"api_key":  true,
	"password": true,
	"username": true,
}

// isSensitiveKey checks if a dotted config key has a sensitive leaf name.
func isSensitiveKey(key string) bool {
	parts := strings.Split(key, ".")
	leaf := parts[len(parts)-1]
	return sensitiveKeyNames[leaf]
}

// maskValue shows first 4 characters followed by **** for sensitive values.
func maskValue(val string) string {
	if len(val) == 0 {
		return ""
	}
	if len(val) > 4 {
		return val[:4] + "****"
	}
	return "****"
}

// flattenSettings recursively flattens a nested map into dotted key-value pairs.
func flattenSettings(prefix string, m map[string]any, out *[]views.ConfigField) {
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenSettings(fullKey, val, out)
		default:
			strVal := fmt.Sprintf("%v", v)
			field := views.ConfigField{
				Key:      fullKey,
				RawValue: strVal,
				Value:    strVal,
				Section:  strings.Split(fullKey, ".")[0],
			}
			if isSensitiveKey(fullKey) {
				field.Sensitive = true
				field.Value = maskValue(strVal)
				field.RawValue = "" // do not expose raw value for sensitive fields
			}
			_ = val // suppress unused warning for type switch
			*out = append(*out, field)
		}
	}
}

// HandleConfig renders the configuration page.
func (h *Handlers) HandleConfig(w http.ResponseWriter, r *http.Request) {
	savedKey := r.URL.Query().Get("saved")

	settings := viper.AllSettings()
	var fields []views.ConfigField
	flattenSettings("", settings, &fields)

	// Group fields by section.
	sectionMap := make(map[string][]views.ConfigField)
	for _, f := range fields {
		sectionMap[f.Section] = append(sectionMap[f.Section], f)
	}

	// Order sections deterministically.
	sectionOrder := []string{"server", "database", "log", "pipeline", "ai", "api"}
	var sections []views.ConfigSection
	seen := make(map[string]bool)
	for _, name := range sectionOrder {
		if fs, ok := sectionMap[name]; ok {
			sort.Slice(fs, func(i, j int) bool { return fs[i].Key < fs[j].Key })
			sections = append(sections, views.ConfigSection{Name: name, Fields: fs})
			seen[name] = true
		}
	}
	// Add any extra sections not in the predefined order.
	var extraNames []string
	for name := range sectionMap {
		if !seen[name] {
			extraNames = append(extraNames, name)
		}
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		fs := sectionMap[name]
		sort.Slice(fs, func(i, j int) bool { return fs[i].Key < fs[j].Key })
		sections = append(sections, views.ConfigSection{Name: name, Fields: fs})
	}

	views.ConfigPage(sections, savedKey).Render(r.Context(), w)
}

// HandleConfigUpdate saves a config value via viper and returns the updated field fragment.
func (h *Handlers) HandleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	key := r.FormValue("key")
	value := r.FormValue("value")

	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	// Validate that the key exists in viper settings.
	if !viper.IsSet(key) {
		http.Error(w, "unknown config key", http.StatusBadRequest)
		return
	}

	viper.Set(key, value)

	// Persist to config file.
	if err := viper.WriteConfig(); err != nil {
		// If no config file exists yet, try SafeWriteConfig.
		if err2 := viper.SafeWriteConfig(); err2 != nil {
			slog.Warn("could not write config file", "error", err, "safe_error", err2)
		}
	}

	// Log the change (mask value if sensitive).
	logValue := value
	if isSensitiveKey(key) {
		logValue = maskValue(value)
	}
	slog.Info("config updated", "key", key, "value", logValue)

	// Build updated field for fragment response.
	field := views.ConfigField{
		Key:      key,
		RawValue: value,
		Value:    value,
		Section:  strings.Split(key, ".")[0],
	}
	if isSensitiveKey(key) {
		field.Sensitive = true
		field.Value = maskValue(value)
		field.RawValue = ""
	}

	views.ConfigFieldFragment(field, true).Render(r.Context(), w)
}

// HandleConfigEdit renders an edit input for a sensitive config field.
func (h *Handlers) HandleConfigEdit(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	field := views.ConfigField{
		Key:       key,
		Sensitive: true,
		Section:   strings.Split(key, ".")[0],
	}

	views.ConfigFieldEditFragment(field).Render(r.Context(), w)
}

// HandleConfigCancel renders the read-only view for a sensitive config field (cancel edit).
func (h *Handlers) HandleConfigCancel(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	strVal := fmt.Sprintf("%v", viper.Get(key))
	field := views.ConfigField{
		Key:     key,
		Value:   maskValue(strVal),
		Section: strings.Split(key, ".")[0],
	}
	if isSensitiveKey(key) {
		field.Sensitive = true
		field.RawValue = ""
	}

	views.ConfigFieldFragment(field, false).Render(r.Context(), w)
}

// HandleBulkRetry resets all failed entity mappings for a pipeline run to pending status.
func (h *Handlers) HandleBulkRetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		views.BulkActionResult("Bad form data.", false).Render(ctx, w)
		return
	}

	runIDStr := r.FormValue("run_id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		views.BulkActionResult("Invalid run ID.", false).Render(ctx, w)
		return
	}

	if err := h.db.ResetFailedMappingsForRetry(ctx, runID); err != nil {
		slog.Error("bulk retry failed", "run_id", runID, "error", err)
		views.BulkActionResult("Failed to reset items: "+err.Error(), false).Render(ctx, w)
		return
	}

	slog.Info("bulk retry completed", "run_id", runID)
	views.BulkActionResult("Failed items have been reset for retry.", true).Render(ctx, w)
}

// HandleBulkSkip marks all failed entity mappings for a pipeline run as skipped.
func (h *Handlers) HandleBulkSkip(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		views.BulkActionResult("Bad form data.", false).Render(ctx, w)
		return
	}

	runIDStr := r.FormValue("run_id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		views.BulkActionResult("Invalid run ID.", false).Render(ctx, w)
		return
	}

	failed, err := h.db.ListFailedMappingsByRun(ctx, runID)
	if err != nil {
		slog.Error("listing failed mappings for skip", "run_id", runID, "error", err)
		views.BulkActionResult("Failed to list items: "+err.Error(), false).Render(ctx, w)
		return
	}

	skipped := 0
	for _, m := range failed {
		if err := h.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       "skipped",
			PlentyID:     m.PlentyID,
			ErrorMessage: sql.NullString{},
			ID:           m.ID,
		}); err != nil {
			slog.Error("skipping mapping", "mapping_id", m.ID, "error", err)
			continue
		}
		skipped++
	}

	slog.Info("bulk skip completed", "run_id", runID, "count", skipped)
	views.BulkActionResult(fmt.Sprintf("Skipped %d failed items.", skipped), true).Render(ctx, w)
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
