package plenty

import (
	"log/slog"
	"net/http"
	"time"
)

// LoggingTransport is an http.RoundTripper that logs HTTP request and response
// details at slog debug level. It logs method, URL path (not full URL to avoid
// leaking tokens in query params), status code, duration, and content length.
type LoggingTransport struct {
	Base   http.RoundTripper
	Logger *slog.Logger
}

// RoundTrip implements http.RoundTripper. It records the start time, delegates
// to the Base transport, and logs the request/response details.
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := t.Base.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		t.Logger.Warn("HTTP request failed",
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	t.Logger.Debug("HTTP request completed",
		slog.String("method", req.Method),
		slog.String("path", req.URL.Path),
		slog.Int("status", resp.StatusCode),
		slog.Duration("duration", duration),
		slog.Int64("content_length", resp.ContentLength),
	)

	return resp, nil
}
