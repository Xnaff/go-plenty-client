// Package enrichment defines the interface and shared types for enriching
// generated product data from external public databases.
package enrichment

import "context"

// Enricher enriches product data from an external source.
type Enricher interface {
	// Enrich queries the external source and returns enrichment data for the
	// given product request.
	Enrich(ctx context.Context, req EnrichmentRequest) (*EnrichmentResult, error)

	// Name returns the identifier of this enrichment source (e.g. "openfoodfacts", "wikidata").
	Name() string
}

// EnrichmentRequest contains the product details used to query external sources.
type EnrichmentRequest struct {
	ProductName string   `json:"product_name"`
	ProductType string   `json:"product_type"`
	Category    string   `json:"category"`
	Barcode     string   `json:"barcode"` // EAN/UPC if available
	Keywords    []string `json:"keywords"`
}

// EnrichmentResult holds the data returned by an enrichment source.
type EnrichmentResult struct {
	Source     string            `json:"source"`
	Fields     map[string]string `json:"fields"`      // Key-value data (ingredients, nutrition, etc.)
	Categories []string          `json:"categories"`   // Suggested categories from source
	ImageURLs  []string          `json:"image_urls"`   // Product images from source
	Confidence float64           `json:"confidence"`   // 0-1 match confidence
}
