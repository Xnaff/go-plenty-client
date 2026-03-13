// Package openfoodfacts provides an enrichment client for the Open Food Facts
// public database (https://world.openfoodfacts.org).
package openfoodfacts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/janemig/plentyone/internal/enrichment"
	"golang.org/x/time/rate"
)

// Compile-time interface check.
var _ enrichment.Enricher = (*Client)(nil)

// Client queries the Open Food Facts API for product enrichment data.
type Client struct {
	httpClient     *http.Client
	searchLimiter  *rate.Limiter
	productLimiter *rate.Limiter
	userAgent      string
}

// NewClient creates an Open Food Facts client with appropriate rate limiters.
//   - searchLimiter: 10 requests/min (search endpoint)
//   - productLimiter: 100 requests/min (product lookup endpoint)
func NewClient(userAgent string) *Client {
	return &Client{
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		searchLimiter:  rate.NewLimiter(rate.Every(6*time.Second), 1),
		productLimiter: rate.NewLimiter(rate.Limit(100.0/60.0), 1),
		userAgent:      userAgent,
	}
}

// Name returns "openfoodfacts".
func (c *Client) Name() string { return "openfoodfacts" }

// Enrich queries Open Food Facts for product data. If req.Barcode is set,
// a barcode lookup is attempted first (higher confidence). Otherwise text
// search by product name is used.
func (c *Client) Enrich(ctx context.Context, req enrichment.EnrichmentRequest) (*enrichment.EnrichmentResult, error) {
	if req.Barcode != "" {
		result, err := c.lookupBarcode(ctx, req.Barcode)
		if err == nil && result != nil {
			return result, nil
		}
		// Fall through to text search if barcode lookup fails.
	}

	if req.ProductName == "" {
		return nil, fmt.Errorf("openfoodfacts: product name required for search")
	}
	return c.searchByName(ctx, req.ProductName)
}

// lookupBarcode retrieves a product by its EAN/UPC barcode via the v2 API.
func (c *Client) lookupBarcode(ctx context.Context, barcode string) (*enrichment.EnrichmentResult, error) {
	if err := c.productLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("openfoodfacts: rate limit wait: %w", err)
	}

	u := fmt.Sprintf(
		"https://world.openfoodfacts.org/api/v2/product/%s.json?fields=product_name,brands,categories,ingredients_text,nutriments,image_url,nutriscore_grade",
		url.PathEscape(barcode),
	)

	resp, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("openfoodfacts: barcode lookup: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		Status  int `json:"status"`
		Product struct {
			ProductName    string `json:"product_name"`
			Brands         string `json:"brands"`
			Categories     string `json:"categories"`
			IngredientsText string `json:"ingredients_text"`
			ImageURL       string `json:"image_url"`
			NutriscoreGrade string `json:"nutriscore_grade"`
		} `json:"product"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openfoodfacts: decode barcode response: %w", err)
	}
	if body.Status != 1 {
		return nil, fmt.Errorf("openfoodfacts: product not found for barcode %s", barcode)
	}

	result := &enrichment.EnrichmentResult{
		Source:     "openfoodfacts",
		Fields:     make(map[string]string),
		Confidence: 0.8,
	}
	if body.Product.Brands != "" {
		result.Fields["brand"] = body.Product.Brands
	}
	if body.Product.IngredientsText != "" {
		result.Fields["ingredients"] = body.Product.IngredientsText
	}
	if body.Product.NutriscoreGrade != "" {
		result.Fields["nutriscore"] = body.Product.NutriscoreGrade
	}
	if body.Product.Categories != "" {
		result.Fields["categories_tags"] = body.Product.Categories
		result.Categories = splitTrim(body.Product.Categories)
	}
	if body.Product.ImageURL != "" {
		result.ImageURLs = []string{body.Product.ImageURL}
	}

	return result, nil
}

// searchByName performs a text search via the v1 search.pl endpoint.
func (c *Client) searchByName(ctx context.Context, name string) (*enrichment.EnrichmentResult, error) {
	if err := c.searchLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("openfoodfacts: rate limit wait: %w", err)
	}

	params := url.Values{
		"search_terms":  {name},
		"search_simple": {"1"},
		"action":        {"process"},
		"json":          {"1"},
		"page_size":     {"5"},
	}
	u := "https://world.openfoodfacts.org/cgi/search.pl?" + params.Encode()

	resp, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("openfoodfacts: search: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		Count    int `json:"count"`
		Products []struct {
			ProductName    string `json:"product_name"`
			Brands         string `json:"brands"`
			Categories     string `json:"categories"`
			IngredientsText string `json:"ingredients_text"`
			ImageURL       string `json:"image_url"`
			NutriscoreGrade string `json:"nutriscore_grade"`
		} `json:"products"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openfoodfacts: decode search response: %w", err)
	}
	if body.Count == 0 || len(body.Products) == 0 {
		return nil, fmt.Errorf("openfoodfacts: no results for %q", name)
	}

	// Use the first (best) result.
	p := body.Products[0]
	result := &enrichment.EnrichmentResult{
		Source:     "openfoodfacts",
		Fields:     make(map[string]string),
		Confidence: 0.5,
	}
	if p.Brands != "" {
		result.Fields["brand"] = p.Brands
	}
	if p.IngredientsText != "" {
		result.Fields["ingredients"] = p.IngredientsText
	}
	if p.NutriscoreGrade != "" {
		result.Fields["nutriscore"] = p.NutriscoreGrade
	}
	if p.Categories != "" {
		result.Fields["categories_tags"] = p.Categories
		result.Categories = splitTrim(p.Categories)
	}
	if p.ImageURL != "" {
		result.ImageURLs = []string{p.ImageURL}
	}

	return result, nil
}

// doGet performs an HTTP GET with the configured User-Agent header.
func (c *Client) doGet(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return resp, nil
}

// splitTrim splits a comma-separated string and trims whitespace from each element.
func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
