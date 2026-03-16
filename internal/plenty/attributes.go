package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// AttributeService provides methods for the PlentyONE attributes API.
type AttributeService struct {
	client *Client
}

// Create creates a new attribute in PlentyONE.
// POST /rest/attributes
func (s *AttributeService) Create(ctx context.Context, req *CreateAttributeRequest) (*Attribute, error) {
	path := "/rest/attributes"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Attribute{ID: -1}, nil
	}

	return doJSON[Attribute](ctx, s.client, http.MethodPost, path, req)
}

// CreateValue creates a new value for the given attribute.
// POST /rest/attributes/{id}/values
func (s *AttributeService) CreateValue(ctx context.Context, attributeID int64, req *CreateAttributeValueRequest) (*AttributeValue, error) {
	path := fmt.Sprintf("/rest/attributes/%d/values", attributeID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &AttributeValue{ID: -1}, nil
	}

	return doJSON[AttributeValue](ctx, s.client, http.MethodPost, path, req)
}

// List retrieves a paginated list of attributes.
// GET /rest/attributes?page=X&itemsPerPage=Y
func (s *AttributeService) List(ctx context.Context, params PaginationParams) (*PaginatedResponse[Attribute], error) {
	path := fmt.Sprintf("/rest/attributes?page=%d&itemsPerPage=%d", params.Page, params.ItemsPerPage)
	return doJSON[PaginatedResponse[Attribute]](ctx, s.client, http.MethodGet, path, nil)
}

// ListValues retrieves a paginated list of values for the given attribute.
// GET /rest/attributes/{id}/values?page=X&itemsPerPage=Y
func (s *AttributeService) ListValues(ctx context.Context, attributeID int64, params PaginationParams) (*PaginatedResponse[AttributeValue], error) {
	path := fmt.Sprintf("/rest/attributes/%d/values?page=%d&itemsPerPage=%d", attributeID, params.Page, params.ItemsPerPage)
	return doJSON[PaginatedResponse[AttributeValue]](ctx, s.client, http.MethodGet, path, nil)
}

// CreateName creates a multilingual name for an attribute.
// POST /rest/items/attributes/{attributeId}/names
func (s *AttributeService) CreateName(ctx context.Context, attributeID int64, req *CreateAttributeNameRequest) (*AttributeName, error) {
	path := fmt.Sprintf("/rest/items/attributes/%d/names", attributeID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &AttributeName{ID: -1, AttributeID: attributeID, Lang: req.Lang, Name: req.Name}, nil
	}

	return doJSON[AttributeName](ctx, s.client, http.MethodPost, path, req)
}

// CreateValueName creates a multilingual name for an attribute value.
// POST /rest/items/attribute_values/{valueId}/names
func (s *AttributeService) CreateValueName(ctx context.Context, valueID int64, req *CreateAttributeValueNameRequest) (*AttributeValueName, error) {
	path := fmt.Sprintf("/rest/items/attribute_values/%d/names", valueID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &AttributeValueName{ID: -1, ValueID: valueID, Lang: req.Lang, Name: req.Name}, nil
	}

	return doJSON[AttributeValueName](ctx, s.client, http.MethodPost, path, req)
}

// Delete removes an attribute by ID.
// DELETE /rest/attributes/{id}
func (s *AttributeService) Delete(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/rest/attributes/%d", id)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodDelete, path, nil)
		return nil
	}

	resp, err := s.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseErrorResponse(resp)
	}

	return nil
}
