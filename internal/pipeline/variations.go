package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// VariationStage creates additional variations (beyond the main variation) in
// PlentyONE for each product, then sets sales prices on ALL variations (main +
// additional) that have non-zero prices in the local DB.
//
// The main variation is created together with the item in the products stage,
// so this stage handles only the 2nd+ variations for creation. Price setting
// applies to all variations.
//
// TODO: Attribute value linking for variations. Currently variations are created
// without VariationAttributeValues. To add attribute value linking, query
// variation_attributes and resolve PlentyONE attribute/value IDs.
type VariationStage struct {
	client              *plenty.Client
	db                  *queries.Queries
	cfg                 PipelineConfig
	defaultSalesPriceID int64 // cached after first lookup
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

// priceItem pairs a local variation with the PlentyONE IDs needed for price setting.
type priceItem struct {
	variation       queries.Variation
	plentyItemID    int64
	plentyVarID     int64
	salesPriceID    int64
	price           float64
}

// Execute creates additional variations for every product in the job, then
// sets sales prices on all variations that have non-zero prices.
func (s *VariationStage) Execute(ctx context.Context, rc *RunContext) error {
	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("listing products for job %d: %w", rc.JobID, err)
	}

	// Phase 1: Create additional variations (index 1+).
	var createItems []variationItem
	// Also collect ALL variations for price setting in Phase 2.
	var allVariations []variationItem

	for _, product := range products {
		variations, err := s.db.ListVariationsByProduct(ctx, product.ID)
		if err != nil {
			rc.Logger.Error("failed to list variations for product",
				slog.Int64("product_id", product.ID),
				slog.Any("error", err),
			)
			continue
		}

		for i, v := range variations {
			item := variationItem{variation: v, product: product}
			allVariations = append(allVariations, item)
			if i > 0 {
				// Additional variations need creation.
				createItems = append(createItems, item)
			}
		}
	}

	// Phase 1: Create additional variations.
	if len(createItems) > 0 {
		succeeded, failed := ProcessItems(ctx, createItems, s.cfg.Concurrency, func(ctx context.Context, item variationItem) error {
			return s.processVariation(ctx, rc, item)
		})

		rc.Logger.Info("variation creation complete",
			slog.Int("succeeded", succeeded),
			slog.Int("failed", failed),
		)
	} else {
		rc.Logger.Info("no additional variations to create")
	}

	// Phase 2: Set prices on all variations with non-zero prices.
	salesPriceID, err := s.resolveDefaultSalesPriceID(ctx)
	if err != nil {
		rc.Logger.Warn("could not resolve sales price config, skipping price setting", "error", err)
		return nil
	}
	if salesPriceID == 0 {
		rc.Logger.Warn("no sales price configuration found in PlentyONE, skipping price setting")
		return nil
	}

	// Build price items for variations with non-zero prices.
	var priceItems []priceItem
	for _, item := range allVariations {
		// Parse price from the variation's sql.NullString.
		if !item.variation.Price.Valid {
			continue
		}
		price, parseErr := strconv.ParseFloat(item.variation.Price.String, 64)
		if parseErr != nil || price == 0 {
			continue
		}

		// Look up the PlentyONE variation ID from entity_mappings.
		varMapping, mErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
			RunID:      rc.RunID,
			LocalID:    item.variation.ID,
			EntityType: string(domain.EntityVariation),
		})
		if mErr != nil {
			rc.Logger.Warn("no PlentyONE mapping for variation, skipping price",
				slog.Int64("variation_id", item.variation.ID),
				slog.Any("error", mErr),
			)
			continue
		}
		if varMapping.Status != string(domain.StatusCreated) {
			continue
		}

		// Resolve parent product's PlentyONE item ID.
		prodMapping, pErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
			RunID:      rc.RunID,
			LocalID:    item.product.ID,
			EntityType: string(domain.EntityProduct),
		})
		if pErr != nil {
			rc.Logger.Warn("no PlentyONE mapping for product, skipping price",
				slog.Int64("product_id", item.product.ID),
				slog.Any("error", pErr),
			)
			continue
		}

		priceItems = append(priceItems, priceItem{
			variation:    item.variation,
			plentyItemID: prodMapping.PlentyID,
			plentyVarID:  varMapping.PlentyID,
			salesPriceID: salesPriceID,
			price:        price,
		})
	}

	if len(priceItems) == 0 {
		rc.Logger.Info("no variations with prices to set")
		return nil
	}

	priceSucceeded, priceFailed := ProcessItems(ctx, priceItems, s.cfg.Concurrency, func(ctx context.Context, item priceItem) error {
		_, err := s.client.Variations.SetSalesPrice(ctx, item.plentyItemID, item.plentyVarID, &plenty.VariationSalesPriceRequest{
			SalesPriceID: item.salesPriceID,
			Price:        item.price,
		})
		if err != nil {
			rc.Logger.Error("failed to set sales price",
				slog.Int64("variation_id", item.variation.ID),
				slog.Float64("price", item.price),
				slog.Any("error", err),
			)
			return err
		}
		rc.Logger.Debug("set sales price on variation",
			slog.Int64("variation_id", item.variation.ID),
			slog.Int64("plenty_variation_id", item.plentyVarID),
			slog.Float64("price", item.price),
		)
		return nil
	})

	rc.Logger.Info("variation price setting complete",
		slog.Int("succeeded", priceSucceeded),
		slog.Int("failed", priceFailed),
	)

	return nil
}

// resolveDefaultSalesPriceID fetches sales price configs from PlentyONE and
// returns the ID of the default one. Results are cached for subsequent calls.
func (s *VariationStage) resolveDefaultSalesPriceID(ctx context.Context) (int64, error) {
	if s.defaultSalesPriceID != 0 {
		return s.defaultSalesPriceID, nil
	}

	configs, err := s.client.Variations.ListSalesPriceConfigs(ctx)
	if err != nil {
		return 0, err
	}

	if len(configs) == 0 {
		return 0, nil
	}

	// Prefer the config with type "default"; fall back to first.
	for _, c := range configs {
		if c.Type == "default" {
			s.defaultSalesPriceID = c.ID
			return c.ID, nil
		}
	}

	s.defaultSalesPriceID = configs[0].ID
	return configs[0].ID, nil
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
