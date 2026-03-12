package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// PropertyService provides methods for the PlentyONE properties API.
type PropertyService struct {
	client *Client
}

// Create creates a new property in PlentyONE.
// POST /rest/properties
func (s *PropertyService) Create(ctx context.Context, req *CreatePropertyRequest) (*Property, error) {
	path := "/rest/properties"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Property{ID: -1}, nil
	}

	return doJSON[Property](ctx, s.client, http.MethodPost, path, req)
}

// CreateSelection creates a selection value for a selection-type property.
// POST /rest/properties/{id}/selections
func (s *PropertyService) CreateSelection(ctx context.Context, propertyID int64, req *CreatePropertySelectionRequest) (*PropertySelection, error) {
	path := fmt.Sprintf("/rest/properties/%d/selections", propertyID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &PropertySelection{ID: -1}, nil
	}

	return doJSON[PropertySelection](ctx, s.client, http.MethodPost, path, req)
}

// CreateRelation creates a relation between a property and a target (e.g., variation).
// POST /rest/properties/relations
func (s *PropertyService) CreateRelation(ctx context.Context, req *PropertyRelationRequest) (*PropertyRelation, error) {
	path := "/rest/properties/relations"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &PropertyRelation{ID: -1}, nil
	}

	return doJSON[PropertyRelation](ctx, s.client, http.MethodPost, path, req)
}
