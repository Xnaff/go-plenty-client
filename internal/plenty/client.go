package plenty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/janemig/plentyone/internal/storage/queries"
)

// ClientConfig holds all configuration needed to construct a PlentyONE API client.
type ClientConfig struct {
	// BaseURL is the per-shop URL (e.g., https://myshop.plentymarkets-cloud01.com).
	BaseURL string

	// Username and Password for PlentyONE API authentication.
	// Read from PLENTYONE_API_USERNAME and PLENTYONE_API_PASSWORD by the caller.
	Username string
	Password string

	// RateLimit is the maximum API calls per minute. Defaults to 40 (PlentyONE Basic plan).
	RateLimit int

	// Timeout is the default HTTP request timeout. Defaults to 60s.
	Timeout time.Duration

	// ImageTimeout is the timeout for image upload requests. Defaults to 120s.
	ImageTimeout time.Duration

	// DryRun, when true, prevents actual API mutations (for testing pipelines).
	DryRun bool

	// Logger is the structured logger for all client operations.
	Logger *slog.Logger

	// DB is the sqlc queries interface for token persistence.
	DB *queries.Queries
}

// Client is the PlentyONE REST API client. It wraps an http.Client configured
// with a RoundTripper chain that handles auth, rate limiting, retries, and logging.
type Client struct {
	httpClient *http.Client
	baseURL    string
	dryRun     bool
	logger     *slog.Logger
	tokenStore *TokenStore

	// Entity services — initialized in NewClient.
	Categories *CategoryService
	Attributes *AttributeService
	Properties *PropertyService
	Items      *ItemService
	Variations *VariationService
	Images     *ImageService
	Texts      *TextService
}

// NewClient creates a new PlentyONE API client with the full RoundTripper chain:
// AuthTransport -> RateLimitTransport -> RetryTransport -> LoggingTransport -> http.DefaultTransport
func NewClient(cfg ClientConfig) *Client {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.ImageTimeout == 0 {
		cfg.ImageTimeout = 120 * time.Second
	}

	tokenStore := NewTokenStore(cfg.DB, cfg.BaseURL, cfg.Username, cfg.Password, cfg.Logger)

	// Build RoundTripper chain (innermost to outermost):
	// http.DefaultTransport -> LoggingTransport -> RetryTransport -> RateLimitTransport -> AuthTransport
	var transport http.RoundTripper = http.DefaultTransport

	transport = &LoggingTransport{
		Base:   transport,
		Logger: cfg.Logger,
	}

	transport = NewRetryTransport(transport, 3, cfg.Logger)

	transport = NewRateLimitTransport(transport, cfg.RateLimit, cfg.Logger)

	transport = &AuthTransport{
		Base:       transport,
		TokenStore: tokenStore,
	}

	c := &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
		baseURL:    cfg.BaseURL,
		dryRun:     cfg.DryRun,
		logger:     cfg.Logger,
		tokenStore: tokenStore,
	}

	// Initialize entity services.
	c.Categories = &CategoryService{client: c}
	c.Attributes = &AttributeService{client: c}
	c.Properties = &PropertyService{client: c}
	c.Items = &ItemService{client: c}
	c.Variations = &VariationService{client: c}
	c.Images = &ImageService{client: c}
	c.Texts = &TextService{client: c}

	return c
}

// doJSON performs an HTTP request and decodes the JSON response into the type T.
// It handles error responses by parsing them into APIError.
func doJSON[T any](ctx context.Context, c *Client, method, path string, body any) (*T, error) {
	resp, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, parseErrorResponse(resp)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response for %s %s: %w", method, path, err)
	}

	return &result, nil
}

// do performs an HTTP request and decodes the JSON response into the provided result.
// This is the non-generic version for callers who prefer interfaces over generics.
func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	resp, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseErrorResponse(resp)
	}

	if result != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body for %s %s: %w", method, path, err)
		}
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response for %s %s: %w", method, path, err)
		}
	}

	return nil
}

// doRequest performs a raw HTTP request and returns the response.
// Callers are responsible for closing the response body.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body for %s %s: %w", method, path, err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s %s: %w", method, path, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}
