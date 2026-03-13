package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// ItemService provides methods for the PlentyONE items API.
type ItemService struct {
	client *Client
}

// Create creates a new item (parent product) with its main variation.
// POST /rest/items
func (s *ItemService) Create(ctx context.Context, req *CreateItemRequest) (*Item, error) {
	path := "/rest/items"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Item{ID: -1}, nil
	}

	return doJSON[Item](ctx, s.client, http.MethodPost, path, req)
}

// Get retrieves an item by ID.
// GET /rest/items/{id}
func (s *ItemService) Get(ctx context.Context, id int64) (*Item, error) {
	path := fmt.Sprintf("/rest/items/%d", id)
	return doJSON[Item](ctx, s.client, http.MethodGet, path, nil)
}

// List retrieves a paginated list of items.
// GET /rest/items?page=X&itemsPerPage=Y
func (s *ItemService) List(ctx context.Context, params PaginationParams) (*PaginatedResponse[Item], error) {
	path := fmt.Sprintf("/rest/items?page=%d&itemsPerPage=%d", params.Page, params.ItemsPerPage)
	return doJSON[PaginatedResponse[Item]](ctx, s.client, http.MethodGet, path, nil)
}

// Delete removes an item by ID.
// DELETE /rest/items/{id}
func (s *ItemService) Delete(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/rest/items/%d", id)

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
