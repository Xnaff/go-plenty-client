package plenty

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

// defaultCallsPerMinute is the conservative rate for PlentyONE Basic plan.
const defaultCallsPerMinute = 40

// RateLimitTransport is an http.RoundTripper that throttles outgoing requests
// using a token bucket limiter. It adapts the rate dynamically based on
// PlentyONE's response headers indicating remaining API call quota.
type RateLimitTransport struct {
	Base    http.RoundTripper
	limiter *rate.Limiter
	maxRate rate.Limit
	mu      sync.Mutex
	logger  *slog.Logger
}

// NewRateLimitTransport creates a rate-limited transport. callsPerMinute defaults
// to 40 (PlentyONE Basic plan) if set to 0 or negative.
func NewRateLimitTransport(base http.RoundTripper, callsPerMinute int, logger *slog.Logger) *RateLimitTransport {
	if callsPerMinute <= 0 {
		callsPerMinute = defaultCallsPerMinute
	}

	perSecond := float64(callsPerMinute) / 60.0
	limiter := rate.NewLimiter(rate.Limit(perSecond), 1)

	return &RateLimitTransport{
		Base:    base,
		limiter: limiter,
		maxRate: rate.Limit(perSecond),
		logger:  logger,
	}
}

// RoundTrip implements http.RoundTripper. It waits for the rate limiter before
// forwarding the request, then adjusts the rate based on response headers.
func (t *RateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Wait for rate limiter permission
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}

	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	t.adaptRate(resp)
	return resp, nil
}

// adaptRate adjusts the rate limiter based on PlentyONE's response headers:
//   - X-Plenty-Global-Short-Period-Calls-Left: remaining calls in the current short period
//   - X-Plenty-Global-Short-Period-Decay: seconds until the period resets
//
// When calls-left is low (< 5), the rate is reduced. When calls-left is
// relatively high, the rate is gradually restored toward the configured maximum.
func (t *RateLimitTransport) adaptRate(resp *http.Response) {
	callsLeftStr := resp.Header.Get("X-Plenty-Global-Short-Period-Calls-Left")
	if callsLeftStr == "" {
		return
	}

	callsLeft, err := strconv.Atoi(callsLeftStr)
	if err != nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	currentRate := t.limiter.Limit()

	if callsLeft < 5 {
		// Reduce rate to avoid hitting the limit
		newRate := currentRate * 0.5
		if newRate < 0.1 {
			newRate = 0.1 // minimum: ~6 calls per minute
		}
		t.limiter.SetLimit(newRate)
		t.logger.Debug("rate limiter: reducing rate due to low remaining calls",
			slog.Int("calls_left", callsLeft),
			slog.Float64("old_rate", float64(currentRate)),
			slog.Float64("new_rate", float64(newRate)),
		)
	} else if callsLeft > 20 && currentRate < t.maxRate {
		// Gradually restore rate
		newRate := currentRate * 1.25
		if newRate > t.maxRate {
			newRate = t.maxRate
		}
		t.limiter.SetLimit(newRate)
		t.logger.Debug("rate limiter: increasing rate, quota healthy",
			slog.Int("calls_left", callsLeft),
			slog.Float64("old_rate", float64(currentRate)),
			slog.Float64("new_rate", float64(newRate)),
		)
	}
}
