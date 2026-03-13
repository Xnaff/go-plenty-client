package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// Stage defines the contract for a pipeline stage.
// Each stage is responsible for creating one type of entity in PlentyONE.
type Stage interface {
	// Name returns the stage identifier (e.g., "categories", "products").
	Name() domain.StageName

	// Execute runs the stage for the given pipeline run context.
	// It should process all relevant items, tracking successes and failures
	// via entity_mappings.
	Execute(ctx context.Context, rc *RunContext) error
}

// RunContext carries per-run state through the pipeline.
type RunContext struct {
	RunID  int64
	JobID  int64
	DryRun bool
	Logger *slog.Logger
}

// StageOrder defines the strict execution order for the 6-stage pipeline.
// Each stage depends on entities created by earlier stages.
var StageOrder = []domain.StageName{
	domain.StageCategories,
	domain.StageAttributes,
	domain.StageProducts,
	domain.StageVariations,
	domain.StageImages,
	domain.StageTexts,
}

// ProcessCheck holds the result of checking whether an item needs processing.
type ProcessCheck struct {
	// NeedsProcessing is true if the item should be processed (new or retry).
	NeedsProcessing bool
	// ExistingID holds the entity_mapping ID (for retries) or the PlentyONE ID
	// (for skips). Zero if this is a new item.
	ExistingID int64
}

// ShouldProcess checks entity_mappings to determine if an item needs processing.
// It enables idempotent resume by skipping items that were already successfully created.
//
// Returns:
//   - NeedsProcessing=true, ExistingID=0: new item, needs processing
//   - NeedsProcessing=false, ExistingID=plentyID: already created/complete, skip
//   - NeedsProcessing=true, ExistingID=mappingID: previously failed, retry
func ShouldProcess(ctx context.Context, db *queries.Queries, runID, localID int64, entityType string) (ProcessCheck, error) {
	mapping, err := db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      runID,
		LocalID:    localID,
		EntityType: entityType,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No mapping exists yet -- this item needs processing.
			return ProcessCheck{NeedsProcessing: true, ExistingID: 0}, nil
		}
		return ProcessCheck{}, err
	}

	switch domain.EntityStatus(mapping.Status) {
	case domain.StatusCreated, domain.StatusComplete:
		// Already done -- skip and return the PlentyONE ID.
		return ProcessCheck{NeedsProcessing: false, ExistingID: mapping.PlentyID}, nil
	case domain.StatusFailed:
		// Previously failed -- retry with the mapping ID for updates.
		return ProcessCheck{NeedsProcessing: true, ExistingID: mapping.ID}, nil
	default:
		// Other statuses (pending, orphaned, etc.) -- process.
		return ProcessCheck{NeedsProcessing: true, ExistingID: 0}, nil
	}
}
