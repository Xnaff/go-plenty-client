package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// CategoryService provides methods for the PlentyONE categories API.
type CategoryService struct {
	client *Client
}

// Create creates a new category in PlentyONE.
// POST /rest/categories
func (s *CategoryService) Create(ctx context.Context, req *CreateCategoryRequest) (*Category, error) {
	path := "/rest/categories"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Category{ID: -1}, nil
	}

	return doJSON[Category](ctx, s.client, http.MethodPost, path, req)
}

// Get retrieves a category by ID.
// GET /rest/categories/{id}
func (s *CategoryService) Get(ctx context.Context, id int64) (*Category, error) {
	path := fmt.Sprintf("/rest/categories/%d", id)
	return doJSON[Category](ctx, s.client, http.MethodGet, path, nil)
}

// List retrieves a paginated list of categories.
// GET /rest/categories?page=X&itemsPerPage=Y
func (s *CategoryService) List(ctx context.Context, params PaginationParams) (*PaginatedResponse[Category], error) {
	path := fmt.Sprintf("/rest/categories?page=%d&itemsPerPage=%d", params.Page, params.ItemsPerPage)
	return doJSON[PaginatedResponse[Category]](ctx, s.client, http.MethodGet, path, nil)
}

// Delete removes a category by ID.
// DELETE /rest/categories/{id}
func (s *CategoryService) Delete(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/rest/categories/%d", id)

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
