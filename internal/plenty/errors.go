package plenty

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// APIError represents an error response from the PlentyONE REST API.
type APIError struct {
	StatusCode       int                 `json:"status_code"`
	Message          string              `json:"message"`
	ValidationErrors map[string][]string `json:"validation_errors,omitempty"`
	RawBody          string              `json:"raw_body,omitempty"`
	RetryAfter       int                 `json:"retry_after,omitempty"`
	RequestID        string              `json:"request_id,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("plentyONE API error %d: %s", e.StatusCode, e.Message)
}

// IsRateLimitError returns true if the error is a 429 Too Many Requests.
func IsRateLimitError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}

// IsServerError returns true if the error is a 5xx server error.
func IsServerError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	return false
}

// IsRetryable returns true if the error is a rate limit or server error.
func IsRetryable(err error) bool {
	return IsRateLimitError(err) || IsServerError(err)
}

// parseErrorResponse reads an HTTP error response and constructs an APIError.
// It handles JSON responses with various shapes as well as plain text/HTML.
func parseErrorResponse(resp *http.Response) *APIError {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
	}

	// Parse Retry-After header (integer seconds) for 429 responses
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			apiErr.RetryAfter = seconds
		}
	}

	// Parse request ID if present
	if reqID := resp.Header.Get("X-Request-Id"); reqID != "" {
		apiErr.RequestID = reqID
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // limit to 64KB
	if err != nil {
		apiErr.Message = fmt.Sprintf("failed to read error body: %v", err)
		return apiErr
	}
	apiErr.RawBody = string(body)

	// Try to parse as JSON
	var jsonBody map[string]json.RawMessage
	if err := json.Unmarshal(body, &jsonBody); err == nil {
		// Try "error" field
		if raw, ok := jsonBody["error"]; ok {
			var msg string
			if json.Unmarshal(raw, &msg) == nil {
				apiErr.Message = msg
			}
		}

		// Try "message" field (may override or supplement "error")
		if raw, ok := jsonBody["message"]; ok {
			var msg string
			if json.Unmarshal(raw, &msg) == nil {
				if apiErr.Message == "" {
					apiErr.Message = msg
				}
			}
		}

		// Try "validation_errors" field
		if raw, ok := jsonBody["validation_errors"]; ok {
			var valErrs map[string][]string
			if json.Unmarshal(raw, &valErrs) == nil {
				apiErr.ValidationErrors = valErrs
				if apiErr.Message == "" {
					apiErr.Message = "validation failed"
				}
			}
		}

		if apiErr.Message == "" {
			apiErr.Message = string(body)
		}
	} else {
		// Not JSON -- use raw body as message (HTML, plain text, etc.)
		if len(body) > 500 {
			apiErr.Message = string(body[:500]) + "..."
		} else {
			apiErr.Message = string(body)
		}
	}

	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}

	return apiErr
}
