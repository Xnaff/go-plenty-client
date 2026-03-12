package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// TextService provides methods for the PlentyONE variation descriptions API.
// Each call handles ONE language at a time (not all languages in one call).
type TextService struct {
	client *Client
}

// CreateDescription creates a description for a variation in a single language.
// POST /rest/items/{id}/variations/{variationId}/descriptions
func (s *TextService) CreateDescription(ctx context.Context, itemID, variationID int64, req *CreateDescriptionRequest) (*Description, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d/descriptions", itemID, variationID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Description{
			ItemID:      itemID,
			VariationID: variationID,
			Lang:        req.Lang,
			Name:        req.Name,
		}, nil
	}

	return doJSON[Description](ctx, s.client, http.MethodPost, path, req)
}

// UpdateDescription updates a description for a variation in a single language.
// PUT /rest/items/{id}/variations/{variationId}/descriptions/{lang}
func (s *TextService) UpdateDescription(ctx context.Context, itemID, variationID int64, lang string, req *CreateDescriptionRequest) (*Description, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d/descriptions/%s", itemID, variationID, lang)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPut, path, req)
		return &Description{
			ItemID:      itemID,
			VariationID: variationID,
			Lang:        lang,
			Name:        req.Name,
		}, nil
	}

	return doJSON[Description](ctx, s.client, http.MethodPut, path, req)
}

// ListDescriptions retrieves all descriptions for a variation (all languages).
// GET /rest/items/{id}/variations/{variationId}/descriptions
func (s *TextService) ListDescriptions(ctx context.Context, itemID, variationID int64) ([]Description, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d/descriptions", itemID, variationID)
	result, err := doJSON[[]Description](ctx, s.client, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return *result, nil
}
