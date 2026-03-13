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

// ProductStage creates parent items (products) in PlentyONE with correct
// category linkage. It resolves category PlentyONE IDs from entity_mappings
// and records main variation entity_mappings for use by later stages.
type ProductStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewProductStage creates a new ProductStage.
func NewProductStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *ProductStage {
	return &ProductStage{
		client: client,
		db:     db,
		cfg:    cfg,
	}
}

// Name returns the stage identifier.
func (s *ProductStage) Name() domain.StageName {
	return domain.StageProducts
}

// Execute runs the product creation stage.
// For each product it resolves the category PlentyONE ID, creates the item
// in PlentyONE with its main variation, and records entity_mappings for both
// the product and the main variation.
func (s *ProductStage) Execute(ctx context.Context, rc *RunContext) error {
	logger := rc.Logger.With(slog.String("stage", string(domain.StageProducts)))

	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("loading products for job %d: %w", rc.JobID, err)
	}

	logger.Info("processing products",
		slog.Int("count", len(products)),
	)

	if len(products) == 0 {
		logger.Info("no products to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, products, s.cfg.Concurrency, func(ctx context.Context, prod queries.Product) error {
		return s.processProduct(ctx, rc, logger, prod)
	})

	logger.Info("products stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// processProduct handles a single product: resolve category, create pending mapping,
// call PlentyONE API, record product and main variation mappings.
func (s *ProductStage) processProduct(ctx context.Context, rc *RunContext, logger *slog.Logger, prod queries.Product) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, prod.ID, string(domain.EntityProduct))
	if err != nil {
		return fmt.Errorf("checking product %d: %w", prod.ID, err)
	}

	if !check.NeedsProcessing {
		logger.Debug("product already created, skipping",
			slog.Int64("local_id", prod.ID),
			slog.Int64("plenty_id", check.ExistingID),
		)
		return nil
	}

	// Resolve the product's default category PlentyONE ID.
	plentyCategoryID, err := s.resolveCategory(ctx, rc, prod.ID)
	if err != nil {
		// If we have an existing mapping (retry), mark it as failed.
		if check.ExistingID > 0 {
			_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
				Status:       string(domain.StatusFailed),
				PlentyID:     0,
				ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
				ID:           check.ExistingID,
			})
		}
		logger.Warn("failed to resolve category for product",
			slog.Int64("local_id", prod.ID),
			slog.String("error", err.Error()),
		)
		return err
	}

	// Determine mapping ID: reuse existing failed mapping or create new pending.
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
			LocalID:      prod.ID,
			PlentyID:     0,
			EntityType:   string(domain.EntityProduct),
			Stage:        string(domain.StageProducts),
			Status:       string(domain.StatusPending),
			ErrorMessage: sql.NullString{},
		})
		if err != nil {
			return fmt.Errorf("creating pending mapping for product %d: %w", prod.ID, err)
		}
	}

	// Build PlentyONE request with the main variation.
	req := &plenty.CreateItemRequest{
		Variations: []plenty.CreateItemVariation{
			{
				VariationDefaultCategory: plenty.CreateItemCategory{
					CategoryID: plentyCategoryID,
				},
				Name: prod.Name,
			},
		},
	}

	// Create item in PlentyONE.
	result, err := s.client.Items.Create(ctx, req)
	if err != nil {
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
			ID:           mappingID,
		})
		logger.Warn("failed to create product in PlentyONE",
			slog.Int64("local_id", prod.ID),
			slog.String("name", prod.Name),
			slog.String("error", err.Error()),
		)
		return err
	}

	// Mark product as created.
	if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
		Status:       string(domain.StatusCreated),
		PlentyID:     result.ID,
		ErrorMessage: sql.NullString{},
		ID:           mappingID,
	}); err != nil {
		return fmt.Errorf("updating mapping for product %d: %w", prod.ID, err)
	}

	logger.Debug("created product in PlentyONE",
		slog.Int64("local_id", prod.ID),
		slog.Int64("plenty_id", result.ID),
		slog.String("name", prod.Name),
	)

	// Record entity_mapping for the main variation.
	// PlentyONE returns the created item with its main variation.
	if err := s.recordMainVariation(ctx, rc, logger, prod.ID, result); err != nil {
		logger.Warn("failed to record main variation mapping",
			slog.Int64("product_local_id", prod.ID),
			slog.Int64("item_plenty_id", result.ID),
			slog.String("error", err.Error()),
		)
		// Non-fatal: product was created successfully, variation mapping is
		// best-effort. Later stages can still look up the variation.
	}

	return nil
}

// resolveCategory looks up the product's category association and resolves it
// to a PlentyONE category ID via entity_mappings.
func (s *ProductStage) resolveCategory(ctx context.Context, rc *RunContext, productID int64) (int64, error) {
	categoryIDs, err := s.db.ListCategoryIDsByProduct(ctx, productID)
	if err != nil {
		return 0, fmt.Errorf("loading category IDs for product %d: %w", productID, err)
	}

	if len(categoryIDs) == 0 {
		return 0, fmt.Errorf("product %d has no category associations", productID)
	}

	// Use the first category as the default category.
	localCategoryID := categoryIDs[0]

	// Look up the PlentyONE ID from entity_mappings.
	mapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      rc.RunID,
		LocalID:    localCategoryID,
		EntityType: string(domain.EntityCategory),
	})
	if err != nil {
		return 0, fmt.Errorf("looking up category mapping for local ID %d: %w", localCategoryID, err)
	}

	if mapping.Status != string(domain.StatusCreated) {
		return 0, fmt.Errorf("category %d not created in PlentyONE (status: %s)", localCategoryID, mapping.Status)
	}

	if mapping.PlentyID == 0 {
		return 0, fmt.Errorf("category %d has no PlentyONE ID", localCategoryID)
	}

	return mapping.PlentyID, nil
}

// recordMainVariation creates an entity_mapping for the main variation that
// PlentyONE returns when creating an item. The local variation ID comes from
// the first variation in the local variations table for this product.
func (s *ProductStage) recordMainVariation(ctx context.Context, rc *RunContext, logger *slog.Logger, productLocalID int64, item *plenty.Item) error {
	if len(item.Variations) == 0 {
		logger.Debug("no variations returned in item response",
			slog.Int64("item_plenty_id", item.ID),
		)
		return nil
	}

	plentyVariationID := item.Variations[0].ID

	// Look up the first local variation for this product.
	localVariations, err := s.db.ListVariationsByProduct(ctx, productLocalID)
	if err != nil {
		return fmt.Errorf("loading local variations for product %d: %w", productLocalID, err)
	}

	if len(localVariations) == 0 {
		// No local variations -- this shouldn't happen for a valid product,
		// but we can still record the mapping with a zero local ID.
		logger.Warn("no local variations found for product",
			slog.Int64("product_local_id", productLocalID),
		)
		return nil
	}

	localVariationID := localVariations[0].ID

	// Create entity_mapping for the main variation.
	_, err = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
		RunID:        rc.RunID,
		LocalID:      localVariationID,
		PlentyID:     plentyVariationID,
		EntityType:   string(domain.EntityVariation),
		Stage:        string(domain.StageProducts),
		Status:       string(domain.StatusCreated),
		ErrorMessage: sql.NullString{},
	})
	if err != nil {
		return fmt.Errorf("creating variation mapping: %w", err)
	}

	logger.Debug("recorded main variation mapping",
		slog.Int64("local_variation_id", localVariationID),
		slog.Int64("plenty_variation_id", plentyVariationID),
		slog.Int64("item_plenty_id", item.ID),
	)

	return nil
}
