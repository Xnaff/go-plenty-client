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
// It loads all categories for the job ordered by level (parents first),
// supports parent-child hierarchy, multilingual descriptions, and
// cross-job category reuse via the category_registry.
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
// Categories are processed sequentially by level (parents first) so that
// child categories can reference their parent's PlentyONE ID.
func (s *CategoryStage) Execute(ctx context.Context, rc *RunContext) error {
	logger := rc.Logger.With(slog.String("stage", string(domain.StageCategories)))

	// ListCategoriesByJob already orders by level, sort_order.
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

	// Track local_id → plenty_id for parent lookups within this run.
	localToPlentyID := make(map[int64]int64)

	var succeeded, failed int
	for _, cat := range categories {
		if err := s.processCategory(ctx, rc, logger, cat, localToPlentyID); err != nil {
			failed++
			logger.Warn("category processing failed",
				slog.Int64("local_id", cat.ID),
				slog.String("name", cat.Name),
				slog.String("error", err.Error()),
			)
		} else {
			succeeded++
		}
	}

	logger.Info("categories stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// processCategory handles a single category: check resume state, check registry,
// create pending mapping, call PlentyONE API, update mapping status, register.
func (s *CategoryStage) processCategory(ctx context.Context, rc *RunContext, logger *slog.Logger, cat queries.Category, localToPlentyID map[int64]int64) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, cat.ID, string(domain.EntityCategory))
	if err != nil {
		return fmt.Errorf("checking category %d: %w", cat.ID, err)
	}

	if !check.NeedsProcessing {
		// Already created — record mapping for child lookups.
		if check.ExistingID > 0 {
			// Look up the plenty_id from the mapping.
			mapping, mErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
				RunID:      rc.RunID,
				LocalID:    cat.ID,
				EntityType: string(domain.EntityCategory),
			})
			if mErr == nil && mapping.PlentyID > 0 {
				localToPlentyID[cat.ID] = mapping.PlentyID
			}
		}
		logger.Debug("category already created, skipping",
			slog.Int64("local_id", cat.ID),
			slog.Int64("plenty_id", check.ExistingID),
		)
		return nil
	}

	// Determine parent name for registry lookups.
	parentName := ""
	if cat.ParentID.Valid {
		parentName = s.lookupParentName(ctx, cat.ParentID.Int64)
	}

	// Check category registry for cross-job reuse.
	reg, regErr := s.db.LookupCategoryRegistry(ctx, queries.LookupCategoryRegistryParams{
		Name:       cat.Name,
		ParentName: parentName,
	})
	if regErr == nil && reg.PlentyID > 0 {
		// Category already exists in PlentyONE from a previous job — reuse it.
		localToPlentyID[cat.ID] = reg.PlentyID

		// Create mapping as "created" directly.
		mappingID, mErr := s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
			RunID:        rc.RunID,
			LocalID:      cat.ID,
			PlentyID:     reg.PlentyID,
			EntityType:   string(domain.EntityCategory),
			Stage:        string(domain.StageCategories),
			Status:       string(domain.StatusCreated),
			ErrorMessage: sql.NullString{},
		})
		if mErr != nil {
			return fmt.Errorf("creating reuse mapping for category %d: %w", cat.ID, mErr)
		}

		logger.Info("reused category from registry",
			slog.Int64("local_id", cat.ID),
			slog.Int64("plenty_id", reg.PlentyID),
			slog.String("name", cat.Name),
			slog.Int64("mapping_id", mappingID),
		)
		return nil
	}

	// Determine mapping ID: either reuse an existing failed mapping or create a new pending one.
	var mappingID int64
	if check.ExistingID > 0 {
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

	// Build PlentyONE request with multilingual details + descriptions.
	details := []plenty.CategoryDetail{
		{Lang: "en", Name: cat.Name},
	}

	// Load stored translations (now includes descriptions).
	translations, tErr := s.db.ListCategoryTranslations(ctx, cat.ID)
	if tErr != nil {
		logger.Warn("failed to load category translations, using English only",
			slog.Int64("category_id", cat.ID),
			slog.String("error", tErr.Error()),
		)
	} else {
		for _, t := range translations {
			if t.Lang == "en" {
				// Update English detail with description.
				details[0].Description = t.Description
				continue
			}
			details = append(details, plenty.CategoryDetail{
				Lang:        t.Lang,
				Name:        t.Name,
				Description: t.Description,
			})
		}
	}

	req := &plenty.CreateCategoryRequest{
		Type:    "item",
		Details: details,
	}

	// Set parent category ID for child categories.
	if cat.ParentID.Valid {
		if parentPlentyID, ok := localToPlentyID[cat.ParentID.Int64]; ok {
			req.ParentCategoryID = &parentPlentyID
		} else {
			logger.Warn("parent PlentyONE ID not found for child category",
				slog.Int64("local_id", cat.ID),
				slog.Int64("parent_local_id", cat.ParentID.Int64),
			)
		}
	}

	// Create in PlentyONE.
	result, err := s.client.Categories.Create(ctx, req)
	if err != nil {
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

	// Track the PlentyONE ID for child lookups.
	localToPlentyID[cat.ID] = result.ID

	// Mark as created.
	if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
		Status:       string(domain.StatusCreated),
		PlentyID:     result.ID,
		ErrorMessage: sql.NullString{},
		ID:           mappingID,
	}); err != nil {
		return fmt.Errorf("updating mapping for category %d: %w", cat.ID, err)
	}

	// Register in category_registry for future cross-job reuse.
	if _, regErr := s.db.RegisterCategory(ctx, queries.RegisterCategoryParams{
		Name:       cat.Name,
		ParentName: parentName,
		PlentyID:   result.ID,
	}); regErr != nil {
		logger.Warn("failed to register category in registry",
			slog.Int64("local_id", cat.ID),
			slog.String("name", cat.Name),
			slog.String("error", regErr.Error()),
		)
	}

	logger.Debug("created category in PlentyONE",
		slog.Int64("local_id", cat.ID),
		slog.Int64("plenty_id", result.ID),
		slog.String("name", cat.Name),
	)

	return nil
}

// lookupParentName retrieves the name of a category by its local ID.
func (s *CategoryStage) lookupParentName(ctx context.Context, parentID int64) string {
	cat, err := s.db.GetCategory(ctx, parentID)
	if err != nil {
		return ""
	}
	return cat.Name
}
