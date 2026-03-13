package pipeline

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// ImageStage uploads product images to PlentyONE. For each image it resolves
// the parent item's PlentyONE ID from entity_mappings and calls UploadURL (if
// source_url is set) or UploadBase64 (if local_path points to a file).
type ImageStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewImageStage creates an ImageStage ready for registration.
func NewImageStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *ImageStage {
	return &ImageStage{client: client, db: db, cfg: cfg}
}

// Name returns the stage identifier.
func (s *ImageStage) Name() domain.StageName {
	return domain.StageImages
}

// imageItem pairs a local image record with its parent product for concurrent processing.
type imageItem struct {
	image   queries.Image
	product queries.Product
}

// Execute uploads images for every product in the job.
func (s *ImageStage) Execute(ctx context.Context, rc *RunContext) error {
	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("listing products for job %d: %w", rc.JobID, err)
	}

	// Flatten all (product, image) pairs.
	var items []imageItem
	for _, product := range products {
		images, err := s.db.ListImagesByProduct(ctx, product.ID)
		if err != nil {
			rc.Logger.Error("failed to list images for product",
				slog.Int64("product_id", product.ID),
				slog.Any("error", err),
			)
			continue
		}
		for _, img := range images {
			items = append(items, imageItem{image: img, product: product})
		}
	}

	if len(items) == 0 {
		rc.Logger.Info("no images to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, items, s.cfg.Concurrency, func(ctx context.Context, item imageItem) error {
		return s.processImage(ctx, rc, item)
	})

	rc.Logger.Info("image stage complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

func (s *ImageStage) processImage(ctx context.Context, rc *RunContext, item imageItem) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, item.image.ID, string(domain.EntityImage))
	if err != nil {
		return fmt.Errorf("checking image %d: %w", item.image.ID, err)
	}
	if !check.NeedsProcessing {
		return nil
	}

	// Resolve the parent item's PlentyONE ID.
	productMapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      rc.RunID,
		LocalID:    item.product.ID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return s.recordFailure(ctx, rc, item.image.ID, check.ExistingID,
			fmt.Errorf("resolving PlentyONE item ID for product %d: %w", item.product.ID, err))
	}

	plentyItemID := productMapping.PlentyID

	var plentyID int64
	if rc.DryRun {
		plentyID = -1
	} else {
		img, uploadErr := s.upload(ctx, plentyItemID, item.image)
		if uploadErr != nil {
			return s.recordFailure(ctx, rc, item.image.ID, check.ExistingID, uploadErr)
		}
		plentyID = img.ID
	}

	return s.recordSuccess(ctx, rc, item.image.ID, plentyID, check.ExistingID)
}

// upload determines the upload method (URL vs base64) and calls the appropriate
// PlentyONE API method.
func (s *ImageStage) upload(ctx context.Context, plentyItemID int64, img queries.Image) (*plenty.Image, error) {
	// Prefer URL upload if source_url is set.
	if img.SourceUrl != "" {
		return s.client.Images.UploadURL(ctx, plentyItemID, &plenty.UploadImageURLRequest{
			UploadURL: img.SourceUrl,
			Position:  int(img.Position),
		})
	}

	// Fall back to base64 upload if local_path is set.
	if img.LocalPath != "" {
		data, err := os.ReadFile(img.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("reading image file %s: %w", img.LocalPath, err)
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		return s.client.Images.UploadBase64(ctx, plentyItemID, &plenty.UploadImageBase64Request{
			UploadImageData: encoded,
			Position:        int(img.Position),
		})
	}

	return nil, fmt.Errorf("image %d has neither source_url nor local_path", img.ID)
}

func (s *ImageStage) recordSuccess(ctx context.Context, rc *RunContext, localID, plentyID, existingID int64) error {
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
		EntityType:   string(domain.EntityImage),
		Stage:        string(domain.StageImages),
		Status:       string(domain.StatusCreated),
		ErrorMessage: sql.NullString{},
	})
	return err
}

func (s *ImageStage) recordFailure(ctx context.Context, rc *RunContext, localID, existingID int64, originalErr error) error {
	rc.Logger.Error("image processing failed",
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
			EntityType:   string(domain.EntityImage),
			Stage:        string(domain.StageImages),
			Status:       string(domain.StatusFailed),
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
		})
	}
	return originalErr
}

// Ensure ImageStage implements Stage at compile time.
var _ Stage = (*ImageStage)(nil)
