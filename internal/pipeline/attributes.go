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

	// Push multilingual attribute name translations.
	nameTranslations, ntErr := s.db.ListAttributeNameTranslations(ctx, attr.ID)
	if ntErr != nil {
		logger.Warn("failed to load attribute name translations",
			slog.Int64("attribute_id", attr.ID), slog.String("error", ntErr.Error()))
	} else {
		for _, nt := range nameTranslations {
			if _, nErr := s.client.Attributes.CreateName(ctx, result.ID, &plenty.CreateAttributeNameRequest{
				Lang: nt.Lang,
				Name: nt.Name,
			}); nErr != nil {
				logger.Warn("failed to create attribute name translation in PlentyONE",
					slog.Int64("attribute_plenty_id", result.ID),
					slog.String("lang", nt.Lang),
					slog.String("error", nErr.Error()),
				)
			}
		}
		if len(nameTranslations) > 0 {
			logger.Debug("pushed attribute name translations",
				slog.Int64("attribute_plenty_id", result.ID),
				slog.Int("languages", len(nameTranslations)),
			)
		}
	}

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
// Each value gets an entity mapping (type "attribute_value") so the variation
// stage can resolve PlentyONE IDs. Value name translations are pushed after
// each value is created.
func (s *AttributeStage) createAttributeValues(ctx context.Context, rc *RunContext, logger *slog.Logger, localAttrID, plentyAttrID int64) error {
	values, err := s.db.ListAttributeValuesByAttribute(ctx, localAttrID)
	if err != nil {
		return fmt.Errorf("loading attribute values for attribute %d: %w", localAttrID, err)
	}

	if len(values) == 0 {
		return nil
	}

	for _, val := range values {
		// Check if this value already has a mapping (resume-safe).
		valCheck, vcErr := ShouldProcess(ctx, s.db, rc.RunID, val.ID, string(domain.EntityAttributeValue))
		if vcErr != nil {
			logger.Warn("failed to check attribute value mapping",
				slog.Int64("value_id", val.ID), slog.String("error", vcErr.Error()))
		}
		if vcErr == nil && !valCheck.NeedsProcessing {
			logger.Debug("attribute value already created, skipping",
				slog.Int64("local_id", val.ID),
				slog.Int64("plenty_id", valCheck.ExistingID),
			)
			continue
		}

		result, cErr := s.client.Attributes.CreateValue(ctx, plentyAttrID, &plenty.CreateAttributeValueRequest{
			BackendName: val.Name,
			Position:    int(val.SortOrder),
		})
		if cErr != nil {
			logger.Warn("failed to create attribute value in PlentyONE",
				slog.Int64("attribute_plenty_id", plentyAttrID),
				slog.String("value_name", val.Name),
				slog.String("error", cErr.Error()),
			)
			continue
		}

		// Create entity mapping for this value so variations can resolve PlentyONE IDs.
		_, _ = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
			RunID:        rc.RunID,
			LocalID:      val.ID,
			PlentyID:     result.ID,
			EntityType:   string(domain.EntityAttributeValue),
			Stage:        string(domain.StageAttributes),
			Status:       string(domain.StatusCreated),
			ErrorMessage: sql.NullString{},
		})

		logger.Debug("created attribute value in PlentyONE",
			slog.Int64("local_id", val.ID),
			slog.Int64("attribute_plenty_id", plentyAttrID),
			slog.Int64("value_plenty_id", result.ID),
			slog.String("value_name", val.Name),
		)

		// Push multilingual value name translations.
		valTranslations, vtErr := s.db.ListAttributeValueTranslations(ctx, val.ID)
		if vtErr != nil {
			logger.Warn("failed to load attribute value translations",
				slog.Int64("value_id", val.ID), slog.String("error", vtErr.Error()))
			continue
		}
		for _, vt := range valTranslations {
			if _, vnErr := s.client.Attributes.CreateValueName(ctx, result.ID, &plenty.CreateAttributeValueNameRequest{
				Lang: vt.Lang,
				Name: vt.Name,
			}); vnErr != nil {
				logger.Warn("failed to create attribute value name translation in PlentyONE",
					slog.Int64("value_plenty_id", result.ID),
					slog.String("lang", vt.Lang),
					slog.String("error", vnErr.Error()),
				)
			}
		}
	}

	return nil
}

// processProperties loads all properties for the job, creates a property group,
// and creates each property in PlentyONE with multilingual names.
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

	// Create property group in PlentyONE if one exists locally for this job.
	var plentyGroupID int64
	localGroup, gErr := s.db.GetPropertyGroupByJob(ctx, rc.JobID)
	if gErr == nil {
		// Check if group already has a mapping (resume-safe).
		groupCheck, gcErr := ShouldProcess(ctx, s.db, rc.RunID, localGroup.ID, string(domain.EntityPropertyGroup))
		if gcErr != nil {
			logger.Warn("failed to check property group mapping", "error", gcErr)
		} else if !groupCheck.NeedsProcessing {
			plentyGroupID = groupCheck.ExistingID
			logger.Debug("property group already created, reusing",
				slog.Int64("plenty_group_id", plentyGroupID))
		} else {
			group, createErr := s.client.Properties.CreateGroup(ctx, &plenty.CreatePropertyGroupRequest{
				Position: 0,
			})
			if createErr != nil {
				logger.Warn("failed to create property group in PlentyONE", "error", createErr)
			} else {
				plentyGroupID = group.ID

				// Create entity mapping for the group.
				_, _ = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
					RunID:        rc.RunID,
					LocalID:      localGroup.ID,
					PlentyID:     plentyGroupID,
					EntityType:   string(domain.EntityPropertyGroup),
					Stage:        string(domain.StageAttributes),
					Status:       string(domain.StatusCreated),
					ErrorMessage: sql.NullString{},
				})

				// Create group names from stored translations.
				groupTranslations, gtErr := s.db.ListPropertyGroupTranslations(ctx, localGroup.ID)
				if gtErr != nil {
					logger.Warn("failed to load property group translations", "error", gtErr)
				} else {
					for _, gt := range groupTranslations {
						_, _ = s.client.Properties.CreateGroupName(ctx, &plenty.CreatePropertyGroupNameRequest{
							PropertyGroupID: plentyGroupID,
							Lang:            gt.Lang,
							Name:            gt.Name,
						})
					}
				}

				logger.Info("created property group in PlentyONE",
					slog.Int64("plenty_group_id", plentyGroupID),
					slog.String("name", localGroup.Name),
				)
			}
		}
	}

	// Store group ID in RunContext for property processing.
	rc.PropertyGroupID = plentyGroupID

	succeeded, failed := ProcessItems(ctx, props, s.cfg.Concurrency, func(ctx context.Context, prop queries.ListPropertiesByJobRow) error {
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
func (s *AttributeStage) processProperty(ctx context.Context, rc *RunContext, logger *slog.Logger, prop queries.ListPropertiesByJobRow) error {
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

	// Build multilingual names from stored translations.
	names := []plenty.PropertyName{{Lang: "en", Name: prop.Name}}
	nameTranslations, ntErr := s.db.ListPropertyNameTranslations(ctx, prop.ID)
	if ntErr != nil {
		logger.Warn("failed to load property name translations, using English only",
			slog.Int64("property_id", prop.ID), slog.String("error", ntErr.Error()))
	} else {
		for _, nt := range nameTranslations {
			names = append(names, plenty.PropertyName{Lang: nt.Lang, Name: nt.Name})
		}
	}

	// Create property in PlentyONE.
	result, err := s.client.Properties.Create(ctx, &plenty.CreatePropertyRequest{
		Cast:     prop.PropertyType,
		Position: 0,
		Names:    names,
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
		slog.Int("languages", len(names)),
	)

	// Attach property to group if one exists.
	if rc.PropertyGroupID > 0 {
		if err := s.client.Properties.AttachPropertyToGroup(ctx, rc.PropertyGroupID, result.ID); err != nil {
			logger.Warn("failed to attach property to group",
				slog.Int64("property_plenty_id", result.ID),
				slog.Int64("group_plenty_id", rc.PropertyGroupID),
				slog.String("error", err.Error()),
			)
		}
	}

	// For selection-type properties, create selection options from stored values.
	if prop.PropertyType == "selection" {
		if err := s.createPropertySelections(ctx, logger, prop.ID, result.ID); err != nil {
			logger.Warn("failed to create some property selections",
				slog.Int64("property_local_id", prop.ID),
				slog.Int64("property_plenty_id", result.ID),
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

// createPropertySelections loads all distinct values for a selection property
// from variation_properties and creates corresponding selection options in PlentyONE.
func (s *AttributeStage) createPropertySelections(ctx context.Context, logger *slog.Logger, localPropID, plentyPropID int64) error {
	values, err := s.db.ListDistinctPropertyValues(ctx, localPropID)
	if err != nil {
		return fmt.Errorf("loading distinct property values for property %d: %w", localPropID, err)
	}

	if len(values) == 0 {
		return nil
	}

	for _, val := range values {
		if !val.Valid || val.String == "" {
			continue
		}

		// Build multilingual names for this selection option.
		selNames := []plenty.PropertyName{{Lang: "en", Name: val.String}}
		optTranslations, otErr := s.db.ListPropertyOptionTranslations(ctx, queries.ListPropertyOptionTranslationsParams{
			PropertyID: localPropID,
			OptionKey:  val.String,
		})
		if otErr != nil {
			logger.Warn("failed to load option translations",
				slog.Int64("property_id", localPropID),
				slog.String("option", val.String),
				slog.String("error", otErr.Error()),
			)
		} else {
			for _, ot := range optTranslations {
				selNames = append(selNames, plenty.PropertyName{Lang: ot.Lang, Name: ot.Name})
			}
		}

		_, err := s.client.Properties.CreateSelection(ctx, plentyPropID, &plenty.CreatePropertySelectionRequest{
			Names: selNames,
		})
		if err != nil {
			logger.Warn("failed to create property selection in PlentyONE",
				slog.Int64("property_plenty_id", plentyPropID),
				slog.String("value", val.String),
				slog.String("error", err.Error()),
			)
			continue
		}

		logger.Debug("created property selection in PlentyONE",
			slog.Int64("property_plenty_id", plentyPropID),
			slog.String("value", val.String),
			slog.Int("languages", len(selNames)),
		)
	}

	return nil
}
