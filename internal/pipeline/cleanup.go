package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// cleanupEntityType defines one entity type to clean up, including how to
// resolve the PlentyONE delete call from an entity_mapping row.
type cleanupEntityType struct {
	entityType string
	deleteFn   func(ctx context.Context, p *Pipeline, mapping queries.EntityMapping) error
}

// CleanupRun deletes all PlentyONE entities created during a pipeline run in
// REVERSE stage order to respect dependency ordering (children before parents).
//
// For each entity type:
//  1. Query entity_mappings with status "created" for this run + type
//  2. Skip dry-run entities (plenty_id = -1)
//  3. Call the appropriate PlentyONE Delete method
//  4. On success: mark mapping as "skipped" (repurposing as "cleaned/deleted")
//  5. On failure: mark mapping as "orphaned", log warning, continue
//
// Text descriptions are skipped because PlentyONE does not support deleting
// individual variation descriptions.
func (p *Pipeline) CleanupRun(ctx context.Context, runID int64) error {
	p.logger.Info("starting cleanup for run", slog.Int64("run_id", runID))

	// Reverse order: texts -> images -> variations -> products -> properties -> attributes -> categories.
	// Texts are skipped (no PlentyONE delete endpoint for descriptions).
	entityTypes := []cleanupEntityType{
		{
			entityType: string(domain.EntityText),
			deleteFn:   nil, // Skip -- no delete API for descriptions.
		},
		{
			entityType: string(domain.EntityImage),
			deleteFn:   deleteImage,
		},
		{
			entityType: string(domain.EntityVariation),
			deleteFn:   deleteVariation,
		},
		{
			entityType: string(domain.EntityProduct),
			deleteFn:   deleteProduct,
		},
		{
			entityType: string(domain.EntityProperty),
			deleteFn:   deleteProperty,
		},
		{
			entityType: string(domain.EntityAttribute),
			deleteFn:   deleteAttribute,
		},
		{
			entityType: string(domain.EntityCategory),
			deleteFn:   deleteCategory,
		},
	}

	var totalCleaned, totalOrphaned, totalSkipped int

	for _, et := range entityTypes {
		mappings, err := p.db.ListCreatedMappingsByRunAndType(ctx, queries.ListCreatedMappingsByRunAndTypeParams{
			RunID:      runID,
			EntityType: et.entityType,
		})
		if err != nil {
			p.logger.Error("failed to list mappings for cleanup",
				slog.String("entity_type", et.entityType),
				slog.Any("error", err),
			)
			continue
		}

		if len(mappings) == 0 {
			continue
		}

		// Skip entity types without a delete function (e.g., texts).
		if et.deleteFn == nil {
			for _, m := range mappings {
				_ = p.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
					Status:       string(domain.StatusSkipped),
					PlentyID:     m.PlentyID,
					ErrorMessage: sql.NullString{},
					ID:           m.ID,
				})
				totalSkipped++
			}
			p.logger.Info("skipped entity type cleanup (no delete API)",
				slog.String("entity_type", et.entityType),
				slog.Int("count", len(mappings)),
			)
			continue
		}

		for _, m := range mappings {
			// Skip dry-run entities (plenty_id = -1).
			if m.PlentyID <= 0 {
				_ = p.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
					Status:       string(domain.StatusSkipped),
					PlentyID:     m.PlentyID,
					ErrorMessage: sql.NullString{},
					ID:           m.ID,
				})
				totalSkipped++
				continue
			}

			if err := et.deleteFn(ctx, p, m); err != nil {
				p.logger.Warn("failed to delete entity, marking as orphaned",
					slog.String("entity_type", et.entityType),
					slog.Int64("plenty_id", m.PlentyID),
					slog.Any("error", err),
				)
				_ = p.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
					Status:       string(domain.StatusOrphaned),
					PlentyID:     m.PlentyID,
					ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
					ID:           m.ID,
				})
				totalOrphaned++
				continue
			}

			_ = p.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
				Status:       string(domain.StatusSkipped),
				PlentyID:     m.PlentyID,
				ErrorMessage: sql.NullString{},
				ID:           m.ID,
			})
			totalCleaned++
		}

		p.logger.Info("cleaned entity type",
			slog.String("entity_type", et.entityType),
			slog.Int("total", len(mappings)),
		)
	}

	p.logger.Info("cleanup complete",
		slog.Int64("run_id", runID),
		slog.Int("cleaned", totalCleaned),
		slog.Int("orphaned", totalOrphaned),
		slog.Int("skipped", totalSkipped),
	)

	return nil
}

// deleteImage resolves the parent item PlentyONE ID and deletes the image.
// The entity_mapping local_id is the image's local ID. We look up the image
// row to find its product_id, then resolve the product's PlentyONE ID.
func deleteImage(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	var productID int64
	err := p.rawDB.QueryRowContext(ctx,
		"SELECT product_id FROM images WHERE id = ?", m.LocalID,
	).Scan(&productID)
	if err != nil {
		return fmt.Errorf("looking up product for image %d: %w", m.LocalID, err)
	}

	productMapping, err := p.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      m.RunID,
		LocalID:    productID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return fmt.Errorf("resolving PlentyONE item ID for image cleanup: %w", err)
	}

	return p.client.Images.Delete(ctx, productMapping.PlentyID, m.PlentyID)
}

// deleteVariation resolves the parent item PlentyONE ID and deletes the variation.
func deleteVariation(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	// The variation mapping local_id is the variation's local ID. Look up
	// its product_id from the variations table.
	var productID int64
	err := p.rawDB.QueryRowContext(ctx,
		"SELECT product_id FROM variations WHERE id = ?", m.LocalID,
	).Scan(&productID)
	if err != nil {
		return fmt.Errorf("looking up product for variation %d: %w", m.LocalID, err)
	}

	productMapping, err := p.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      m.RunID,
		LocalID:    productID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return fmt.Errorf("resolving PlentyONE item ID for variation cleanup: %w", err)
	}

	return p.client.Variations.Delete(ctx, productMapping.PlentyID, m.PlentyID)
}

// deleteProduct deletes an item (product) by its PlentyONE ID.
func deleteProduct(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	return p.client.Items.Delete(ctx, m.PlentyID)
}

// deleteProperty deletes a property by its PlentyONE ID.
func deleteProperty(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	return p.client.Properties.Delete(ctx, m.PlentyID)
}

// deleteAttribute deletes an attribute by its PlentyONE ID.
func deleteAttribute(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	return p.client.Attributes.Delete(ctx, m.PlentyID)
}

// deleteCategory deletes a category by its PlentyONE ID.
func deleteCategory(ctx context.Context, p *Pipeline, m queries.EntityMapping) error {
	return p.client.Categories.Delete(ctx, m.PlentyID)
}
