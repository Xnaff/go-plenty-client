package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// CategoryStage creates categories in PlentyONE for a pipeline run.
// It loads all categories for the job, deduplicates by checking entity_mappings,
// and creates them concurrently via processItems.
type CategoryStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewCategoryStage creates a new CategoryStage.
func NewCategoryStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *CategoryStage {
	return &CategoryStage{
		client: client,
		db:     db,
		cfg:    cfg,
	}
}

// Name returns the stage identifier.
func (s *CategoryStage) Name() domain.StageName {
	return domain.StageCategories
}

// Execute runs the category creation stage.
// For each category in the job it:
//  1. Checks entity_mappings via ShouldProcess (resume-safe)
//  2. Creates a pending mapping before the API call
//  3. Creates the category in PlentyONE
//  4. Updates the mapping to created/failed after the API call
func (s *CategoryStage) Execute(ctx context.Context, rc *RunContext) error {
	logger := rc.Logger.With(slog.String("stage", string(domain.StageCategories)))

	categories, err := s.db.ListCategoriesByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("loading categories for job %d: %w", rc.JobID, err)
	}

	logger.Info("processing categories",
		slog.Int("count", len(categories)),
	)

	if len(categories) == 0 {
		logger.Info("no categories to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, categories, s.cfg.Concurrency, func(ctx context.Context, cat queries.Category) error {
		return s.processCategory(ctx, rc, logger, cat)
	})

	logger.Info("categories stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// processCategory handles a single category: check resume state, create pending
// mapping, call PlentyONE API, update mapping status.
func (s *CategoryStage) processCategory(ctx context.Context, rc *RunContext, logger *slog.Logger, cat queries.Category) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, cat.ID, string(domain.EntityCategory))
	if err != nil {
		return fmt.Errorf("checking category %d: %w", cat.ID, err)
	}

	if !check.NeedsProcessing {
		logger.Debug("category already created, skipping",
			slog.Int64("local_id", cat.ID),
			slog.Int64("plenty_id", check.ExistingID),
		)
		return nil
	}

	// Determine mapping ID: either reuse an existing failed mapping or create a new pending one.
	var mappingID int64
	if check.ExistingID > 0 {
		// Retry of a previously failed item -- reset to pending.
		mappingID = check.ExistingID
		if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusPending),
			PlentyID:     0,
			ErrorMessage: sql.NullString{},
			ID:           mappingID,
		}); err != nil {
			return fmt.Errorf("resetting mapping %d to pending: %w", mappingID, err)
		}
	} else {
		// New item -- create a pending mapping.
		mappingID, err = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
			RunID:        rc.RunID,
			LocalID:      cat.ID,
			PlentyID:     0,
			EntityType:   string(domain.EntityCategory),
			Stage:        string(domain.StageCategories),
			Status:       string(domain.StatusPending),
			ErrorMessage: sql.NullString{},
		})
		if err != nil {
			return fmt.Errorf("creating pending mapping for category %d: %w", cat.ID, err)
		}
	}

	// Build PlentyONE request.
	req := &plenty.CreateCategoryRequest{
		Type: "item",
		Details: []plenty.CategoryDetail{
			{
				Lang: "en",
				Name: cat.Name,
			},
		},
	}

	// Create in PlentyONE.
	result, err := s.client.Categories.Create(ctx, req)
	if err != nil {
		// Mark as failed.
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
			ID:           mappingID,
		})
		logger.Warn("failed to create category in PlentyONE",
			slog.Int64("local_id", cat.ID),
			slog.String("name", cat.Name),
			slog.String("error", err.Error()),
		)
		return err
	}

	// Mark as created with the PlentyONE ID.
	if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
		Status:       string(domain.StatusCreated),
		PlentyID:     result.ID,
		ErrorMessage: sql.NullString{},
		ID:           mappingID,
	}); err != nil {
		return fmt.Errorf("updating mapping for category %d: %w", cat.ID, err)
	}

	logger.Debug("created category in PlentyONE",
		slog.Int64("local_id", cat.ID),
		slog.Int64("plenty_id", result.ID),
		slog.String("name", cat.Name),
	)

	return nil
}
