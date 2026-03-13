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

// AttributeStage creates attributes (with their values) and properties in PlentyONE.
// It handles both entity types in a single stage because they occupy the same
// pipeline position (stage 2) -- both must exist before products can reference them.
type AttributeStage struct {
	client *plenty.Client
	db     *queries.Queries
	cfg    PipelineConfig
}

// NewAttributeStage creates a new AttributeStage.
func NewAttributeStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *AttributeStage {
	return &AttributeStage{
		client: client,
		db:     db,
		cfg:    cfg,
	}
}

// Name returns the stage identifier.
func (s *AttributeStage) Name() domain.StageName {
	return domain.StageAttributes
}

// Execute runs the attribute and property creation stage.
// Phase A processes attributes (each with their values), Phase B processes properties.
func (s *AttributeStage) Execute(ctx context.Context, rc *RunContext) error {
	logger := rc.Logger.With(slog.String("stage", string(domain.StageAttributes)))

	// Phase A: Process attributes.
	if err := s.processAttributes(ctx, rc, logger); err != nil {
		return err
	}

	// Phase B: Process properties.
	if err := s.processProperties(ctx, rc, logger); err != nil {
		return err
	}

	return nil
}

// processAttributes loads all attributes for the job, creates them in PlentyONE
// concurrently, and creates attribute values for each.
func (s *AttributeStage) processAttributes(ctx context.Context, rc *RunContext, logger *slog.Logger) error {
	attrs, err := s.db.ListAttributesByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("loading attributes for job %d: %w", rc.JobID, err)
	}

	logger.Info("processing attributes",
		slog.Int("count", len(attrs)),
	)

	if len(attrs) == 0 {
		logger.Info("no attributes to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, attrs, s.cfg.Concurrency, func(ctx context.Context, attr queries.Attribute) error {
		return s.processAttribute(ctx, rc, logger, attr)
	})

	logger.Info("attributes phase complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// processAttribute handles a single attribute: create in PlentyONE, then create
// all its attribute values.
func (s *AttributeStage) processAttribute(ctx context.Context, rc *RunContext, logger *slog.Logger, attr queries.Attribute) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, attr.ID, string(domain.EntityAttribute))
	if err != nil {
		return fmt.Errorf("checking attribute %d: %w", attr.ID, err)
	}

	if !check.NeedsProcessing {
		logger.Debug("attribute already created, skipping",
			slog.Int64("local_id", attr.ID),
			slog.Int64("plenty_id", check.ExistingID),
		)
		// Even though the attribute is already created, we still need to check
		// if attribute values were created. The variation stage will look them up
		// via client.Attributes.ListValues, so as long as the attribute exists
		// in PlentyONE, values created previously are fine.
		return nil
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
			LocalID:      attr.ID,
			PlentyID:     0,
			EntityType:   string(domain.EntityAttribute),
			Stage:        string(domain.StageAttributes),
			Status:       string(domain.StatusPending),
			ErrorMessage: sql.NullString{},
		})
		if err != nil {
			return fmt.Errorf("creating pending mapping for attribute %d: %w", attr.ID, err)
		}
	}

	// Create attribute in PlentyONE.
	result, err := s.client.Attributes.Create(ctx, &plenty.CreateAttributeRequest{
		BackendName: attr.Name,
		Position:    0,
	})
	if err != nil {
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
			ID:           mappingID,
		})
		logger.Warn("failed to create attribute in PlentyONE",
			slog.Int64("local_id", attr.ID),
			slog.String("name", attr.Name),
			slog.String("error", err.Error()),
		)
		return err
	}

	// Mark attribute as created.
	if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
		Status:       string(domain.StatusCreated),
		PlentyID:     result.ID,
		ErrorMessage: sql.NullString{},
		ID:           mappingID,
	}); err != nil {
		return fmt.Errorf("updating mapping for attribute %d: %w", attr.ID, err)
	}

	logger.Debug("created attribute in PlentyONE",
		slog.Int64("local_id", attr.ID),
		slog.Int64("plenty_id", result.ID),
		slog.String("name", attr.Name),
	)

	// Create attribute values.
	if err := s.createAttributeValues(ctx, rc, logger, attr.ID, result.ID); err != nil {
		// Attribute was created but values failed. Log the error but don't fail
		// the attribute mapping -- the attribute itself exists in PlentyONE.
		logger.Warn("failed to create some attribute values",
			slog.Int64("attribute_local_id", attr.ID),
			slog.Int64("attribute_plenty_id", result.ID),
			slog.String("error", err.Error()),
		)
	}

	return nil
}

// createAttributeValues loads and creates all values for a given attribute.
// Per the plan, we do NOT create entity_mappings for individual attribute values;
// the variations stage will look them up via client.Attributes.ListValues.
func (s *AttributeStage) createAttributeValues(ctx context.Context, rc *RunContext, logger *slog.Logger, localAttrID, plentyAttrID int64) error {
	values, err := s.db.ListAttributeValuesByAttribute(ctx, localAttrID)
	if err != nil {
		return fmt.Errorf("loading attribute values for attribute %d: %w", localAttrID, err)
	}

	if len(values) == 0 {
		return nil
	}

	for _, val := range values {
		_, err := s.client.Attributes.CreateValue(ctx, plentyAttrID, &plenty.CreateAttributeValueRequest{
			BackendName: val.Name,
			Position:    int(val.SortOrder),
		})
		if err != nil {
			logger.Warn("failed to create attribute value in PlentyONE",
				slog.Int64("attribute_plenty_id", plentyAttrID),
				slog.String("value_name", val.Name),
				slog.String("error", err.Error()),
			)
			// Continue creating other values; don't stop on individual failure.
			continue
		}

		logger.Debug("created attribute value in PlentyONE",
			slog.Int64("attribute_plenty_id", plentyAttrID),
			slog.String("value_name", val.Name),
		)
	}

	return nil
}

// processProperties loads all properties for the job and creates them in PlentyONE.
func (s *AttributeStage) processProperties(ctx context.Context, rc *RunContext, logger *slog.Logger) error {
	props, err := s.db.ListPropertiesByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("loading properties for job %d: %w", rc.JobID, err)
	}

	logger.Info("processing properties",
		slog.Int("count", len(props)),
	)

	if len(props) == 0 {
		logger.Info("no properties to process")
		return nil
	}

	succeeded, failed := ProcessItems(ctx, props, s.cfg.Concurrency, func(ctx context.Context, prop queries.Property) error {
		return s.processProperty(ctx, rc, logger, prop)
	})

	logger.Info("properties phase complete",
		slog.Int("succeeded", succeeded),
		slog.Int("failed", failed),
	)

	return nil
}

// processProperty handles a single property: check resume state, create pending
// mapping, call PlentyONE API, update mapping status.
func (s *AttributeStage) processProperty(ctx context.Context, rc *RunContext, logger *slog.Logger, prop queries.Property) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, prop.ID, string(domain.EntityProperty))
	if err != nil {
		return fmt.Errorf("checking property %d: %w", prop.ID, err)
	}

	if !check.NeedsProcessing {
		logger.Debug("property already created, skipping",
			slog.Int64("local_id", prop.ID),
			slog.Int64("plenty_id", check.ExistingID),
		)
		return nil
	}

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
			LocalID:      prop.ID,
			PlentyID:     0,
			EntityType:   string(domain.EntityProperty),
			Stage:        string(domain.StageAttributes),
			Status:       string(domain.StatusPending),
			ErrorMessage: sql.NullString{},
		})
		if err != nil {
			return fmt.Errorf("creating pending mapping for property %d: %w", prop.ID, err)
		}
	}

	// Create property in PlentyONE.
	result, err := s.client.Properties.Create(ctx, &plenty.CreatePropertyRequest{
		Cast:     prop.PropertyType,
		Position: 0,
		Names: []plenty.PropertyName{
			{
				Lang: "en",
				Name: prop.Name,
			},
		},
	})
	if err != nil {
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: err.Error(), Valid: true},
			ID:           mappingID,
		})
		logger.Warn("failed to create property in PlentyONE",
			slog.Int64("local_id", prop.ID),
			slog.String("name", prop.Name),
			slog.String("error", err.Error()),
		)
		return err
	}

	// Mark as created.
	if err := s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
		Status:       string(domain.StatusCreated),
		PlentyID:     result.ID,
		ErrorMessage: sql.NullString{},
		ID:           mappingID,
	}); err != nil {
		return fmt.Errorf("updating mapping for property %d: %w", prop.ID, err)
	}

	logger.Debug("created property in PlentyONE",
		slog.Int64("local_id", prop.ID),
		slog.Int64("plenty_id", result.ID),
		slog.String("name", prop.Name),
	)

	return nil
}
