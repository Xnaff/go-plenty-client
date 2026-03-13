// Package pexels implements the imagesource.ImageSource interface
// for the Pexels photo API with rate limiting (200 req/hr).
package pexels

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

const baseURL = "https://api.pexels.com/v1"

// Client is a Pexels API client with built-in rate limiting.
type Client struct {
	httpClient *http.Client
	apiKey     string
	limiter    *rate.Limiter
}

// NewClient creates a Pexels client with the given API key.
// Rate limited to 200 requests per hour (~1 request per 18 seconds).
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		limiter:    rate.NewLimiter(rate.Every(18*time.Second), 1),
	}
}

// Name returns the source provider name.
func (c *Client) Name() string { return "pexels" }

// Search finds photos matching the query on Pexels.
func (c *Client) Search(ctx context.Context, query string, opts imagesource.SearchOptions) ([]imagesource.Photo, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("pexels rate limit wait: %w", err)
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

	reqURL := baseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pexels create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pexels search: status %d: %s", resp.StatusCode, string(body))
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("pexels parse response: %w", err)
	}

	photos := make([]imagesource.Photo, 0, len(result.Photos))
	for _, p := range result.Photos {
		if opts.MinWidth > 0 && p.Width < opts.MinWidth {
			continue
		}
		if opts.MinHeight > 0 && p.Height < opts.MinHeight {
			continue
		}
		photos = append(photos, imagesource.Photo{
			ID:           strconv.Itoa(p.ID),
			Description:  p.Alt,
			URL:          p.Src.Medium,
			DownloadURL:  p.Src.Large,
			Width:        p.Width,
			Height:       p.Height,
			Photographer: p.Photographer,
			Attribution:  fmt.Sprintf("Photo by %s on Pexels", p.Photographer),
			SourceName:   "pexels",
			LicenseType:  "pexels",
		})
	}

	return photos, nil
}

// Download fetches the image data from the given URL.
func (c *Client) Download(ctx context.Context, downloadURL string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("pexels rate limit wait: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pexels create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels download: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pexels read download body: %w", err)
	}

	return data, nil
}

// Pexels API response types (private to package).

type searchResponse struct {
	TotalResults int     `json:"total_results"`
	Page         int     `json:"page"`
	PerPage      int     `json:"per_page"`
	Photos       []photo `json:"photos"`
}

type photo struct {
	ID           int       `json:"id"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	URL          string    `json:"url"`
	Photographer string    `json:"photographer"`
	Alt          string    `json:"alt"`
	Src          photoSrc  `json:"src"`
}

type photoSrc struct {
	Original  string `json:"original"`
	Large2x   string `json:"large2x"`
	Large     string `json:"large"`
	Medium    string `json:"medium"`
	Small     string `json:"small"`
	Portrait  string `json:"portrait"`
	Landscape string `json:"landscape"`
	Tiny      string `json:"tiny"`
}
