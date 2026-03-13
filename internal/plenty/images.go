package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// ImageService provides methods for the PlentyONE item images API.
// Image uploads use base64-encoded JSON payloads (NOT multipart).
type ImageService struct {
	client *Client
}

// UploadBase64 uploads an image to the given item using a base64-encoded JSON payload.
// POST /rest/items/{id}/images/upload
func (s *ImageService) UploadBase64(ctx context.Context, itemID int64, req *UploadImageBase64Request) (*Image, error) {
	path := fmt.Sprintf("/rest/items/%d/images/upload", itemID)

	if s.client.dryRun {
		// Log a truncated version of the payload to avoid flooding logs with base64 data.
		dryRunLog(s.client.logger, http.MethodPost, path, map[string]any{
			"uploadImageData": fmt.Sprintf("[base64 data, %d chars]", len(req.UploadImageData)),
			"position":        req.Position,
		})
		return &Image{ID: -1, ItemID: itemID}, nil
	}

	return doJSON[Image](ctx, s.client, http.MethodPost, path, req)
}

// UploadURL uploads an image to the given item by providing a URL for PlentyONE to fetch.
// POST /rest/items/{id}/images/upload
func (s *ImageService) UploadURL(ctx context.Context, itemID int64, req *UploadImageURLRequest) (*Image, error) {
	path := fmt.Sprintf("/rest/items/%d/images/upload", itemID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Image{ID: -1, ItemID: itemID}, nil
	}

	return doJSON[Image](ctx, s.client, http.MethodPost, path, req)
}

// List retrieves all images for the given item.
// GET /rest/items/{id}/images
func (s *ImageService) List(ctx context.Context, itemID int64) ([]Image, error) {
	path := fmt.Sprintf("/rest/items/%d/images", itemID)
	result, err := doJSON[[]Image](ctx, s.client, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

// Delete removes an image from the given item.
// DELETE /rest/items/{id}/images/{imageId}
func (s *ImageService) Delete(ctx context.Context, itemID, imageID int64) error {
	path := fmt.Sprintf("/rest/items/%d/images/%d", itemID, imageID)

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
