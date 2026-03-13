// Package unsplash implements the imagesource.ImageSource interface
// for the Unsplash photo API with rate limiting (50 req/hr).
package unsplash

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

const baseURL = "https://api.unsplash.com"

// Client is an Unsplash API client with built-in rate limiting.
type Client struct {
	httpClient *http.Client
	accessKey  string
	limiter    *rate.Limiter
}

// NewClient creates an Unsplash client with the given access key.
// Rate limited to 50 requests per hour (~1 request per 72 seconds).
func NewClient(accessKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		accessKey:  accessKey,
		limiter:    rate.NewLimiter(rate.Every(72*time.Second), 1),
	}
}

// Name returns the source provider name.
func (c *Client) Name() string { return "unsplash" }

// Search finds photos matching the query on Unsplash.
func (c *Client) Search(ctx context.Context, query string, opts imagesource.SearchOptions) ([]imagesource.Photo, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("unsplash rate limit wait: %w", err)
	}

	params := url.Values{}
	params.Set("query", query)
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if opts.Orientation != "" {
		params.Set("orientation", opts.Orientation)
	}

	reqURL := baseURL + "/search/photos?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("unsplash create request: %w", err)
	}
	req.Header.Set("Authorization", "Client-ID "+c.accessKey)
	req.Header.Set("Accept-Version", "v1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unsplash search: status %d: %s", resp.StatusCode, string(body))
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("unsplash parse response: %w", err)
	}

	photos := make([]imagesource.Photo, 0, len(result.Results))
	for _, r := range result.Results {
		if opts.MinWidth > 0 && r.Width < opts.MinWidth {
			continue
		}
		if opts.MinHeight > 0 && r.Height < opts.MinHeight {
			continue
		}
		photos = append(photos, imagesource.Photo{
			ID:           r.ID,
			Description:  firstNonEmpty(r.Description, r.AltDescription),
			URL:          r.URLs.Small,
			DownloadURL:  r.URLs.Regular,
			Width:        r.Width,
			Height:       r.Height,
			Photographer: r.User.Name,
			Attribution:  fmt.Sprintf("Photo by %s on Unsplash", r.User.Name),
			SourceName:   "unsplash",
			LicenseType:  "unsplash",
		})
	}

	return photos, nil
}

// Download fetches the image data from the given URL.
func (c *Client) Download(ctx context.Context, downloadURL string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("unsplash rate limit wait: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("unsplash create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsplash download: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unsplash read download body: %w", err)
	}

	return data, nil
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Unsplash API response types (private to package).

type searchResponse struct {
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
	Results    []searchResult `json:"results"`
}

type searchResult struct {
	ID             string     `json:"id"`
	Description    string     `json:"description"`
	AltDescription string     `json:"alt_description"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	URLs           photoURLs  `json:"urls"`
	User           photoUser  `json:"user"`
}

type photoURLs struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

type photoUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
