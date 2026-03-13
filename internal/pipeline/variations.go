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

// VariationStage creates additional variations (beyond the main variation) in
// PlentyONE for each product. The main variation is created together with the
// item in the products stage, so this stage handles only the 2nd+ variations.
//
// For each additional variation it resolves the parent item's PlentyONE ID from
// entity_mappings and calls Variations.Create.
//
// TODO: Attribute value linking for variations. Currently variations are created
// without VariationAttributeValues. To add attribute value linking, query
// variation_attributes and resolve PlentyONE attribute/value IDs.
type VariationStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewVariationStage creates a VariationStage ready for registration.
func NewVariationStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *VariationStage {
	return &VariationStage{client: client, db: db, cfg: cfg}
}

// Name returns the stage identifier.
func (s *VariationStage) Name() domain.StageName {
	return domain.StageVariations
}

// variationItem pairs a local variation with its parent product for concurrent processing.
type variationItem struct {
	variation queries.Variation
	product   queries.Product
}

// Execute creates additional variations for every product in the job.
// The main variation (first per product) is expected to already exist from the
// products stage; only additional variations are processed here.
func (s *VariationStage) Execute(ctx context.Context, rc *RunContext) error {
	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("listing products for job %d: %w", rc.JobID, err)
	}

	// Collect all additional variations across all products.
	var items []variationItem
	for _, product := range products {
		variations, err := s.db.ListVariationsByProduct(ctx, product.ID)
		if err != nil {
			rc.Logger.Error("failed to list variations for product",
				slog.Int64("product_id", product.ID),
				slog.Any("error", err),
			)
			continue
		}

		// Skip the first variation (main variation, created with the item).
		// Additional variations start from index 1.
		for i := 1; i < len(variations); i++ {
			items = append(items, variationItem{
				variation: variations[i],
				product:   product,
			})
		}
	}

	if len(items) == 0 {
		rc.Logger.Info("no additional variations to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, items, s.cfg.Concurrency, func(ctx context.Context, item variationItem) error {
		return s.processVariation(ctx, rc, item)
	})

	rc.Logger.Info("variation stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

func (s *VariationStage) processVariation(ctx context.Context, rc *RunContext, item variationItem) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, item.variation.ID, string(domain.EntityVariation))
	if err != nil {
		return fmt.Errorf("checking variation %d: %w", item.variation.ID, err)
	}
	if !check.NeedsProcessing {
		return nil
	}

	// Resolve the parent item's PlentyONE ID from entity_mappings.
	productMapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      rc.RunID,
		LocalID:    item.product.ID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return s.recordFailure(ctx, rc, item.variation.ID, check.ExistingID,
			fmt.Errorf("resolving PlentyONE item ID for product %d: %w", item.product.ID, err))
	}

	plentyItemID := productMapping.PlentyID

	req := &plenty.CreateVariationRequest{
		Name:   item.variation.Name,
		Number: item.variation.Sku,
	}

	var plentyID int64
	if rc.DryRun {
		plentyID = -1
	} else {
		result, err := s.client.Variations.Create(ctx, plentyItemID, req)
		if err != nil {
			return s.recordFailure(ctx, rc, item.variation.ID, check.ExistingID,
				fmt.Errorf("creating variation in PlentyONE: %w", err))
		}
		plentyID = result.ID
	}

	return s.recordSuccess(ctx, rc, item.variation.ID, plentyID, check.ExistingID)
}

func (s *VariationStage) recordSuccess(ctx context.Context, rc *RunContext, localID, plentyID, existingID int64) error {
	if existingID > 0 {
		// Update existing mapping (retry case).
		return s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusCreated),
			PlentyID:     plentyID,
			ErrorMessage: sql.NullString{},
			ID:           existingID,
		})
	}
	_, err := s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
		RunID:        rc.RunID,
		LocalID:      localID,
		PlentyID:     plentyID,
		EntityType:   string(domain.EntityVariation),
		Stage:        string(domain.StageVariations),
		Status:       string(domain.StatusCreated),
		ErrorMessage: sql.NullString{},
	})
	return err
}

func (s *VariationStage) recordFailure(ctx context.Context, rc *RunContext, localID, existingID int64, originalErr error) error {
	rc.Logger.Error("variation processing failed",
		slog.Int64("local_id", localID),
		slog.Any("error", originalErr),
	)
	if existingID > 0 {
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
			ID:           existingID,
		})
	} else {
		_, _ = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
			RunID:        rc.RunID,
			LocalID:      localID,
			PlentyID:     0,
			EntityType:   string(domain.EntityVariation),
			Stage:        string(domain.StageVariations),
			Status:       string(domain.StatusFailed),
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
		})
	}
	return originalErr
}

// Ensure VariationStage implements Stage at compile time.
var _ Stage = (*VariationStage)(nil)

