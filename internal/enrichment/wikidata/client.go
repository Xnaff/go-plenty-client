// Package wikidata provides an enrichment client for Wikidata
// (https://www.wikidata.org) entity search and retrieval.
package wikidata

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

// Client queries the Wikidata API for entity enrichment data.
type Client struct {
	httpClient *http.Client
	limiter    *rate.Limiter
	userAgent  string
}

// NewClient creates a Wikidata client with a conservative rate limiter
// (~10 requests/min to be respectful of the public API).
func NewClient(userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(10.0/60.0), 1),
		userAgent:  userAgent,
	}
}

// Name returns "wikidata".
func (c *Client) Name() string { return "wikidata" }

// Enrich searches Wikidata for entities matching the product name, then
// fetches detailed entity data including multilingual labels, descriptions,
// and claims.
func (c *Client) Enrich(ctx context.Context, req enrichment.EnrichmentRequest) (*enrichment.EnrichmentResult, error) {
	searchTerm := req.ProductName
	if searchTerm == "" {
		return nil, fmt.Errorf("wikidata: product name required for search")
	}

	// Step 1: Search for matching entities.
	qids, err := c.searchEntities(ctx, searchTerm)
	if err != nil {
		return nil, err
	}
	if len(qids) == 0 {
		return nil, fmt.Errorf("wikidata: no entities found for %q", searchTerm)
	}

	// Step 2: Fetch the top result entity.
	return c.getEntity(ctx, qids[0])
}

// searchEntities uses wbsearchentities to find matching Q-IDs.
func (c *Client) searchEntities(ctx context.Context, term string) ([]string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wikidata: rate limit wait: %w", err)
	}

	params := url.Values{
		"action":   {"wbsearchentities"},
		"search":   {term},
		"language": {"en"},
		"format":   {"json"},
		"limit":    {"3"},
	}
	u := "https://www.wikidata.org/w/api.php?" + params.Encode()

	resp, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("wikidata: search entities: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		Search []struct {
			ID string `json:"id"`
		} `json:"search"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("wikidata: decode search response: %w", err)
	}

	qids := make([]string, 0, len(body.Search))
	for _, s := range body.Search {
		qids = append(qids, s.ID)
	}
	return qids, nil
}

// getEntity fetches entity data via wbgetentities with multilingual labels,
// descriptions, and claims.
func (c *Client) getEntity(ctx context.Context, qid string) (*enrichment.EnrichmentResult, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wikidata: rate limit wait: %w", err)
	}

	params := url.Values{
		"action":    {"wbgetentities"},
		"ids":       {qid},
		"languages": {"en|de|es|fr|it"},
		"props":     {"labels|descriptions|claims"},
		"format":    {"json"},
	}
	u := "https://www.wikidata.org/w/api.php?" + params.Encode()

	resp, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("wikidata: get entity %s: %w", qid, err)
	}
	defer resp.Body.Close()

	var body struct {
		Entities map[string]struct {
			Labels       map[string]labelValue       `json:"labels"`
			Descriptions map[string]labelValue       `json:"descriptions"`
			Claims       map[string][]claimStatement `json:"claims"`
		} `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("wikidata: decode entity response: %w", err)
	}

	entity, ok := body.Entities[qid]
	if !ok {
		return nil, fmt.Errorf("wikidata: entity %s not found in response", qid)
	}

	result := &enrichment.EnrichmentResult{
		Source:     "wikidata",
		Fields:     make(map[string]string),
		Confidence: 0.4, // Wikidata has sparse product data
	}

	// Set wikidata_id.
	result.Fields["wikidata_id"] = qid

	// Build description from multilingual labels/descriptions.
	if en, ok := entity.Descriptions["en"]; ok {
		result.Fields["description"] = en.Value
	}

	// Collect multilingual labels.
	var labels []string
	for lang, lv := range entity.Labels {
		labels = append(labels, lang+":"+lv.Value)
	}
	if len(labels) > 0 {
		result.Fields["labels"] = strings.Join(labels, "; ")
	}

	// Extract categories from instance-of claims (P31).
	if p31Claims, ok := entity.Claims["P31"]; ok {
		for _, claim := range p31Claims {
			if claim.MainSnak.DataValue.Value.ID != "" {
				result.Categories = append(result.Categories, claim.MainSnak.DataValue.Value.ID)
			}
		}
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

// labelValue represents a Wikidata label or description with language.
type labelValue struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// claimStatement represents a single Wikidata claim/statement.
type claimStatement struct {
	MainSnak struct {
		DataValue struct {
			Value struct {
				ID string `json:"id"` // Q-ID for wikibase-entityid type
			} `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}
