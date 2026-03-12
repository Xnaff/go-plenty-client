package plenty

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// RetryTransport is an http.RoundTripper that retries requests on 429 and 5xx
// responses with appropriate backoff strategies.
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
	Logger     *slog.Logger
}

// NewRetryTransport creates a RetryTransport with the given base transport and max retries.
func NewRetryTransport(base http.RoundTripper, maxRetries int, logger *slog.Logger) *RetryTransport {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &RetryTransport{
		Base:       base,
		MaxRetries: maxRetries,
		Logger:     logger,
	}
}

// RoundTrip implements http.RoundTripper. It retries on 429 (using Retry-After)
// and 5xx (using exponential backoff with jitter). Request bodies are buffered
// to enable retries.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Buffer the request body so we can replay it on retries
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		if req.GetBody != nil {
			// Use GetBody if available (set by http.NewRequest for known body types)
			newBody, getErr := req.GetBody()
			if getErr == nil {
				bodyBytes, err = io.ReadAll(newBody)
				newBody.Close()
				if err != nil {
					return nil, fmt.Errorf("buffering request body: %w", err)
				}
			}
		}
		if bodyBytes == nil {
			bodyBytes, err = io.ReadAll(req.Body)
			req.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("buffering request body: %w", err)
			}
		}
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Reset body for each attempt
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		resp, err := t.Base.RoundTrip(req)
		if err != nil {
			lastErr = err
			// Network errors are not retried -- return immediately
			return nil, err
		}

		// Check if response is retryable
		if resp.StatusCode == http.StatusTooManyRequests {
			lastResp = resp
			apiErr := parseErrorResponse(resp)
			resp.Body.Close()

			if attempt >= t.MaxRetries {
				return nil, apiErr
			}

			waitDuration := time.Duration(apiErr.RetryAfter) * time.Second
			if waitDuration <= 0 {
				waitDuration = 5 * time.Second // default if no Retry-After
			}

			t.Logger.Debug("rate limited, waiting before retry",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", t.MaxRetries),
				slog.Duration("wait", waitDuration),
			)

			timer := time.NewTimer(waitDuration)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}

			continue
		}

		if resp.StatusCode >= 500 {
			lastResp = resp
			apiErr := parseErrorResponse(resp)
			resp.Body.Close()

			if attempt >= t.MaxRetries {
				return nil, apiErr
			}

			// Exponential backoff with jitter: base 1s, factor 2, +/- 25%
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			jitter := backoff / 4
			waitDuration := backoff + time.Duration(rand.Int64N(int64(2*jitter)+1)) - jitter

			t.Logger.Debug("server error, retrying with backoff",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", t.MaxRetries),
				slog.Int("status", resp.StatusCode),
				slog.Duration("wait", waitDuration),
			)

			timer := time.NewTimer(waitDuration)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}

			continue
		}

		// Non-retryable response -- return as is
		return resp, nil
	}

	// Should not reach here, but handle gracefully
	if lastResp != nil {
		return nil, parseErrorResponse(lastResp)
	}
	return nil, lastErr
}
