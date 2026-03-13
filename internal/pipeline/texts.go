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

// TextStage creates multilingual variation descriptions in PlentyONE. For each
// product it groups text records by language and makes one CreateDescription
// call per language, mapping local text fields to the PlentyONE description
// fields (name, description, shortDescription, technicalData, metaDescription,
// urlContent).
type TextStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewTextStage creates a TextStage ready for registration.
func NewTextStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *TextStage {
	return &TextStage{client: client, db: db, cfg: cfg}
}

// Name returns the stage identifier.
func (s *TextStage) Name() domain.StageName {
	return domain.StageTexts
}

// textItem groups all text fields for one product + language combination into
// a single processable unit. The anchorTextID is the first text record ID for
// this (product, language) group, used as the entity_mapping local_id.
type textItem struct {
	product        queries.Product
	lang           string
	anchorTextID   int64
	plentyItemID   int64
	plentyVarID    int64
	request        *plenty.CreateDescriptionRequest
}

// Execute creates multilingual descriptions for every product in the job.
func (s *TextStage) Execute(ctx context.Context, rc *RunContext) error {
	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("listing products for job %d: %w", rc.JobID, err)
	}

	var items []textItem
	for _, product := range products {
		productItems, err := s.buildTextItems(ctx, rc, product)
		if err != nil {
			rc.Logger.Error("failed to build text items for product",
				slog.Int64("product_id", product.ID),
				slog.Any("error", err),
			)
			continue
		}
		items = append(items, productItems...)
	}

	if len(items) == 0 {
		rc.Logger.Info("no texts to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, items, s.cfg.Concurrency, func(ctx context.Context, item textItem) error {
		return s.processText(ctx, rc, item)
	})

	rc.Logger.Info("text stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// buildTextItems resolves PlentyONE IDs for a product and groups its text
// records by language, building one textItem per language.
func (s *TextStage) buildTextItems(ctx context.Context, rc *RunContext, product queries.Product) ([]textItem, error) {
	// Resolve the product's PlentyONE item ID.
	productMapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      rc.RunID,
		LocalID:    product.ID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return nil, fmt.Errorf("resolving PlentyONE item ID for product %d: %w", product.ID, err)
	}

	// Resolve the main variation's PlentyONE ID. We look for a variation
	// mapping for this product. The main variation was created with the item
	// in the products stage. We look for any variation of this product that
	// has a mapping to get the PlentyONE variation ID.
	variations, err := s.db.ListVariationsByProduct(ctx, product.ID)
	if err != nil || len(variations) == 0 {
		return nil, fmt.Errorf("listing variations for product %d: no variations found", product.ID)
	}

	// Try to find a variation mapping. The main variation (first) should have
	// been mapped by the products stage with entity_type "variation".
	var plentyVarID int64
	for _, v := range variations {
		varMapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
			RunID:      rc.RunID,
			LocalID:    v.ID,
			EntityType: string(domain.EntityVariation),
		})
		if err == nil {
			plentyVarID = varMapping.PlentyID
			break
		}
	}

	// If no variation mapping exists, the main variation was likely created
	// inline with the item. In PlentyONE, the main variation ID can be
	// fetched from the item response. For dry-run, use -1.
	if plentyVarID == 0 {
		if rc.DryRun {
			plentyVarID = -1
		} else {
			// Attempt to get the item's main variation from PlentyONE directly.
			item, err := s.client.Items.Get(ctx, productMapping.PlentyID)
			if err != nil {
				return nil, fmt.Errorf("fetching item %d to find main variation: %w", productMapping.PlentyID, err)
			}
			if len(item.Variations) > 0 {
				plentyVarID = item.Variations[0].ID
			} else {
				return nil, fmt.Errorf("item %d has no variations in PlentyONE", productMapping.PlentyID)
			}
		}
	}

	// Load all texts for this product and group by language.
	texts, err := s.db.ListTextsByProduct(ctx, product.ID)
	if err != nil {
		return nil, fmt.Errorf("listing texts for product %d: %w", product.ID, err)
	}

	if len(texts) == 0 {
		return nil, nil
	}

	// Group texts by language. Key: lang, Value: map[field]content.
	type langGroup struct {
		anchorID int64
		fields   map[string]string
	}
	groups := make(map[string]*langGroup)

	for _, t := range texts {
		g, ok := groups[t.Lang]
		if !ok {
			g = &langGroup{anchorID: t.ID, fields: make(map[string]string)}
			groups[t.Lang] = g
		}
		g.fields[t.Field] = t.Content
	}

	// Build one textItem per language.
	var items []textItem
	for lang, g := range groups {
		req := &plenty.CreateDescriptionRequest{
			Lang: lang,
		}
		if v, ok := g.fields["name"]; ok {
			req.Name = v
		}
		if v, ok := g.fields["description"]; ok {
			req.Description = v
		}
		if v, ok := g.fields["shortDescription"]; ok {
			req.ShortDescription = v
		}
		if v, ok := g.fields["technicalData"]; ok {
			req.TechnicalData = v
		}
		if v, ok := g.fields["metaDescription"]; ok {
			req.MetaDescription = v
		}
		if v, ok := g.fields["urlContent"]; ok {
			req.URLContent = v
		}

		items = append(items, textItem{
			product:      product,
			lang:         lang,
			anchorTextID: g.anchorID,
			plentyItemID: productMapping.PlentyID,
			plentyVarID:  plentyVarID,
			request:      req,
		})
	}

	return items, nil
}

func (s *TextStage) processText(ctx context.Context, rc *RunContext, item textItem) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, item.anchorTextID, string(domain.EntityText))
	if err != nil {
		return fmt.Errorf("checking text %d: %w", item.anchorTextID, err)
	}
	if !check.NeedsProcessing {
		return nil
	}

	if rc.DryRun {
		return s.recordSuccess(ctx, rc, item.anchorTextID, -1, check.ExistingID)
	}

	_, err = s.client.Texts.CreateDescription(ctx, item.plentyItemID, item.plentyVarID, item.request)
	if err != nil {
		return s.recordFailure(ctx, rc, item.anchorTextID, check.ExistingID,
			fmt.Errorf("creating description for item %d var %d lang %s: %w",
				item.plentyItemID, item.plentyVarID, item.lang, err))
	}

	// Text descriptions don't have their own unique ID in PlentyONE,
	// so store plentyItemID as the plenty_id for reference.
	return s.recordSuccess(ctx, rc, item.anchorTextID, item.plentyItemID, check.ExistingID)
}

func (s *TextStage) recordSuccess(ctx context.Context, rc *RunContext, localID, plentyID, existingID int64) error {
	if existingID > 0 {
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
		EntityType:   string(domain.EntityText),
		Stage:        string(domain.StageTexts),
		Status:       string(domain.StatusCreated),
		ErrorMessage: sql.NullString{},
	})
	return err
}

func (s *TextStage) recordFailure(ctx context.Context, rc *RunContext, localID, existingID int64, originalErr error) error {
	rc.Logger.Error("text processing failed",
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
			EntityType:   string(domain.EntityText),
			Stage:        string(domain.StageTexts),
			Status:       string(domain.StatusFailed),
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
		})
	}
	return originalErr
}

// Ensure TextStage implements Stage at compile time.
var _ Stage = (*TextStage)(nil)
