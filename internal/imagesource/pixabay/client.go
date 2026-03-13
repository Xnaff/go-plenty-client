// Package pixabay implements the imagesource.ImageSource interface
// for the Pixabay photo API with rate limiting (100 req/min).
package pixabay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	"github.com/janemig/plentyone/internal/imagesource"
)

// Compile-time check: Client implements imagesource.ImageSource.
var _ imagesource.ImageSource = (*Client)(nil)

const baseURL = "https://pixabay.com/api/"

// Client is a Pixabay API client with built-in rate limiting.
type Client struct {
	httpClient *http.Client
	apiKey     string
	limiter    *rate.Limiter
}

// NewClient creates a Pixabay client with the given API key.
// Rate limited to 100 requests per minute.
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		limiter:    rate.NewLimiter(rate.Limit(100.0/60.0), 1),
	}
}

// Name returns the source provider name.
func (c *Client) Name() string { return "pixabay" }

// Search finds photos matching the query on Pixabay.
func (c *Client) Search(ctx context.Context, query string, opts imagesource.SearchOptions) ([]imagesource.Photo, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("pixabay rate limit wait: %w", err)
	}

	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("q", query)
	params.Set("image_type", "photo")
	params.Set("safesearch", "true")
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if opts.Orientation != "" {
		params.Set("orientation", opts.Orientation)
	}
	if opts.MinWidth > 0 {
		params.Set("min_width", strconv.Itoa(opts.MinWidth))
	}
	if opts.MinHeight > 0 {
		params.Set("min_height", strconv.Itoa(opts.MinHeight))
	}

	reqURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pixabay create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pixabay search: status %d: %s", resp.StatusCode, string(body))
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("pixabay parse response: %w", err)
	}

	photos := make([]imagesource.Photo, 0, len(result.Hits))
	for _, h := range result.Hits {
		downloadURL := h.LargeImageURL
		if downloadURL == "" {
			downloadURL = h.WebformatURL
		}
		photos = append(photos, imagesource.Photo{
			ID:           strconv.Itoa(h.ID),
			Description:  h.Tags,
			URL:          h.PreviewURL,
			DownloadURL:  downloadURL,
			Width:        h.ImageWidth,
			Height:       h.ImageHeight,
			Photographer: h.User,
			Attribution:  "Image from Pixabay",
			SourceName:   "pixabay",
			LicenseType:  "pixabay",
		})
	}

	return photos, nil
}

// Download fetches the image data from the given URL.
func (c *Client) Download(ctx context.Context, downloadURL string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("pixabay rate limit wait: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pixabay create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pixabay download: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pixabay read download body: %w", err)
	}

	return data, nil
}

// Pixabay API response types (private to package).

type searchResponse struct {
	Total     int   `json:"total"`
	TotalHits int   `json:"totalHits"`
	Hits      []hit `json:"hits"`
}

type hit struct {
	ID            int    `json:"id"`
	Tags          string `json:"tags"`
	PreviewURL    string `json:"previewURL"`
	WebformatURL  string `json:"webformatURL"`
	LargeImageURL string `json:"largeImageURL"`
	ImageWidth    int    `json:"imageWidth"`
	ImageHeight   int    `json:"imageHeight"`
	User          string `json:"user"`
	UserID        int    `json:"user_id"`
}
