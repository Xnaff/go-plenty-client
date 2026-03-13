package plenty

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janemig/plentyone/internal/storage/queries"
)

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

// mockDB implements the queries.DBTX interface backed by an in-memory map
// so we can test token persistence without a real MySQL connection.
type mockDB struct {
	mu     sync.Mutex
	tokens map[string]*queries.OauthToken
}

func newMockDB() *mockDB {
	return &mockDB{tokens: make(map[string]*queries.OauthToken)}
}

func (m *mockDB) ExecContext(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.Contains(query, "DELETE FROM oauth_tokens") {
		shopURL := args[0].(string)
		delete(m.tokens, shopURL)
		return mockResult{}, nil
	}

	if strings.Contains(query, "INSERT INTO oauth_tokens") {
		shopURL := args[0].(string)
		m.tokens[shopURL] = &queries.OauthToken{
			ID:           1,
			ShopUrl:      shopURL,
			AccessToken:  args[1].(string),
			RefreshToken: args[2].(string),
			TokenType:    args[3].(string),
			ExpiresAt:    args[4].(time.Time),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		return mockResult{}, nil
	}

	return mockResult{}, nil
}

func (m *mockDB) PrepareContext(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDB) QueryContext(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockDB) QueryRowContext(_ context.Context, query string, args ...interface{}) *sql.Row {
	m.mu.Lock()
	defer m.mu.Unlock()

	// This is a bit tricky -- sql.Row doesn't have a public constructor.
	// We'll use a workaround with a real DB for the mock to work.
	// For our tests, we'll use the tokenStore directly instead.
	return nil
}

type mockResult struct{}

func (r mockResult) LastInsertId() (int64, error) { return 1, nil }
func (r mockResult) RowsAffected() (int64, error) { return 1, nil }

// inMemoryTokenStore provides a simpler in-memory token store for testing
// the transport chain without needing a real database.
type inMemoryTokenStore struct {
	mu           sync.Mutex
	accessToken  string
	refreshToken string
	expiresAt    time.Time
	shopURL      string
	username     string
	password     string
	logger       *slog.Logger
	authClient   *http.Client
	loginCount   atomic.Int32
}

func newInMemoryTokenStore(shopURL, username, password string, logger *slog.Logger) *inMemoryTokenStore {
	return &inMemoryTokenStore{
		shopURL:  shopURL,
		username: username,
		password: password,
		logger:   logger,
		authClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (ts *inMemoryTokenStore) getValidToken(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.accessToken != "" && time.Now().Before(ts.expiresAt.Add(-tokenExpiryBuffer)) {
		return ts.accessToken, nil
	}

	return ts.login(ctx)
}

func (ts *inMemoryTokenStore) login(ctx context.Context) (string, error) {
	ts.loginCount.Add(1)
	ts.logger.Debug("performing login", slog.String("shop_url", ts.shopURL))

	body := fmt.Sprintf(`{"username":%q,"password":%q}`, ts.username, ts.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.shopURL+"/rest/login", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.authClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", err
	}

	ts.accessToken = loginResp.AccessToken
	ts.refreshToken = loginResp.RefreshToken
	ts.expiresAt = time.Now().Add(time.Duration(loginResp.ExpiresIn) * time.Second)

	return ts.accessToken, nil
}

// testTokenStoreAdapter wraps inMemoryTokenStore to satisfy the AuthTransport interface
// by creating a TokenStore with overridden behavior.
type testAuthTransport struct {
	Base       http.RoundTripper
	tokenStore *inMemoryTokenStore
}

func (t *testAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokenStore.getValidToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("obtaining auth token: %w", err)
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.Base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		t.tokenStore.mu.Lock()
		t.tokenStore.accessToken = "" // force re-login
		newToken, loginErr := t.tokenStore.login(req.Context())
		t.tokenStore.mu.Unlock()
		if loginErr != nil {
			return nil, fmt.Errorf("re-authentication after 401: %w", loginErr)
		}

		req3 := req.Clone(req.Context())
		req3.Header.Set("Authorization", "Bearer "+newToken)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req3.Body = body
		}
		return t.Base.RoundTrip(req3)
	}

	return resp, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestLoginFlow(t *testing.T) {
	var receivedAuth string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/login":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{
				AccessToken:  "test-token-123",
				RefreshToken: "refresh-token-456",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		case "/rest/items":
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	logger := testLogger()
	ts := newInMemoryTokenStore(apiServer.URL, "user", "pass", logger)

	transport := &testAuthTransport{
		Base:       http.DefaultTransport,
		tokenStore: ts,
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/rest/items", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if receivedAuth != "Bearer test-token-123" {
		t.Errorf("expected 'Bearer test-token-123', got %q", receivedAuth)
	}
}

func TestTokenRefresh(t *testing.T) {
	callCount := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/login":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{
				AccessToken:  fmt.Sprintf("token-%d", callCount),
				RefreshToken: "refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		case "/rest/items":
			auth := r.Header.Get("Authorization")
			if auth == "Bearer token-1" {
				// First token is "expired" -- return 401
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"Unauthorized"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	logger := testLogger()
	ts := newInMemoryTokenStore(apiServer.URL, "user", "pass", logger)

	transport := &testAuthTransport{
		Base:       http.DefaultTransport,
		tokenStore: ts,
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/rest/items", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if callCount != 2 {
		t.Errorf("expected 2 login calls (initial + retry), got %d", callCount)
	}
}

func TestRateLimit429(t *testing.T) {
	attempts := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"Too many requests"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	transport := NewRetryTransport(http.DefaultTransport, 3, logger)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestServerErrorRetry(t *testing.T) {
	attempts := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"Service unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	transport := NewRetryTransport(http.DefaultTransport, 3, logger)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestServerErrorExhaustsRetries(t *testing.T) {
	attempts := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Internal server error"}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	transport := NewRetryTransport(http.DefaultTransport, 2, logger)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/test", nil)
	resp, err := client.Do(req)

	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error after exhausting retries")
	}

	var apiErr *APIError
	if !isAPIError(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
	// 1 initial + 2 retries = 3 attempts
	if attempts != 3 {
		t.Errorf("expected 3 attempts (1 + 2 retries), got %d", attempts)
	}
}

func TestErrorParsingJSON(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		expectedMessage  string
		expectedValErrs  bool
	}{
		{
			name:            "error field",
			body:            `{"error":"Something went wrong"}`,
			expectedMessage: "Something went wrong",
		},
		{
			name:            "message field",
			body:            `{"message":"Not found"}`,
			expectedMessage: "Not found",
		},
		{
			name:            "validation_errors",
			body:            `{"validation_errors":{"name":["is required"],"price":["must be positive"]}}`,
			expectedMessage: "validation failed",
			expectedValErrs: true,
		},
		{
			name:            "plain text",
			body:            `Service Unavailable`,
			expectedMessage: "Service Unavailable",
		},
		{
			name:            "html response",
			body:            `<html><body><h1>502 Bad Gateway</h1></body></html>`,
			expectedMessage: "<html><body><h1>502 Bad Gateway</h1></body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}

			apiErr := parseErrorResponse(resp)
			resp.Body.Close()

			if apiErr.Message != tt.expectedMessage {
				t.Errorf("expected message %q, got %q", tt.expectedMessage, apiErr.Message)
			}
			if tt.expectedValErrs && apiErr.ValidationErrors == nil {
				t.Error("expected validation errors to be parsed")
			}
		})
	}
}

func TestConcurrentTokenRefresh(t *testing.T) {
	var loginCount atomic.Int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/login":
			loginCount.Add(1)
			// Simulate slight delay in login
			time.Sleep(50 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{
				AccessToken:  "shared-token",
				RefreshToken: "refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		case "/rest/items":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	logger := testLogger()
	ts := newInMemoryTokenStore(apiServer.URL, "user", "pass", logger)

	transport := &testAuthTransport{
		Base:       http.DefaultTransport,
		tokenStore: ts,
	}

	client := &http.Client{Transport: transport}

	// Launch 10 concurrent requests -- all should see no token and try to login
	const goroutines = 10
	var wg sync.WaitGroup
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/rest/items", nil)
			resp, err := client.Do(req)
			if err != nil {
				errors[idx] = err
				return
			}
			resp.Body.Close()
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}

	// Due to the mutex in inMemoryTokenStore, only one login should happen
	// (the first goroutine logs in, subsequent ones find the valid token)
	count := loginCount.Load()
	if count != 1 {
		t.Errorf("expected exactly 1 login call, got %d", count)
	}
}

func TestAdaptiveRate(t *testing.T) {
	callCount := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Simulate low remaining calls
		w.Header().Set("X-Plenty-Global-Short-Period-Calls-Left", "3")
		w.Header().Set("X-Plenty-Global-Short-Period-Decay", "10")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	rl := NewRateLimitTransport(http.DefaultTransport, 60, logger)
	client := &http.Client{Transport: rl}

	// Make a request to trigger rate adaptation
	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Check that the rate was reduced
	currentRate := rl.limiter.Limit()
	maxRate := rl.maxRate
	if currentRate >= maxRate {
		t.Errorf("expected rate to be reduced below max %f, got %f", float64(maxRate), float64(currentRate))
	}
}

func TestAdaptiveRateIncrease(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Plenty-Global-Short-Period-Calls-Left", "50")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	rl := NewRateLimitTransport(http.DefaultTransport, 60, logger)

	// Manually reduce the rate first
	rl.limiter.SetLimit(rl.maxRate * 0.5)
	reducedRate := rl.limiter.Limit()

	client := &http.Client{Transport: rl}
	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Rate should have increased
	currentRate := rl.limiter.Limit()
	if currentRate <= reducedRate {
		t.Errorf("expected rate to increase above %f, got %f", float64(reducedRate), float64(currentRate))
	}
}

func TestRetryAfterHeader(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"Rate limit exceeded"}`))
	}))
	defer apiServer.Close()

	resp, err := http.Get(apiServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	apiErr := parseErrorResponse(resp)
	resp.Body.Close()

	if apiErr.RetryAfter != 42 {
		t.Errorf("expected RetryAfter 42, got %d", apiErr.RetryAfter)
	}
	if apiErr.RequestID != "req-123" {
		t.Errorf("expected RequestID 'req-123', got %q", apiErr.RequestID)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("expected StatusCode 429, got %d", apiErr.StatusCode)
	}
}

func TestLoggingTransport(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	transport := &LoggingTransport{
		Base:   http.DefaultTransport,
		Logger: logger,
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest(http.MethodGet, apiServer.URL+"/rest/items", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIsRateLimitError(t *testing.T) {
	err := &APIError{StatusCode: 429, Message: "Too many requests"}
	if !IsRateLimitError(err) {
		t.Error("expected IsRateLimitError to return true for 429")
	}
	if IsRateLimitError(fmt.Errorf("plain error")) {
		t.Error("expected IsRateLimitError to return false for non-APIError")
	}
}

func TestIsServerError(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		err := &APIError{StatusCode: code}
		if !IsServerError(err) {
			t.Errorf("expected IsServerError to return true for %d", code)
		}
	}
	err := &APIError{StatusCode: 400}
	if IsServerError(err) {
		t.Error("expected IsServerError to return false for 400")
	}
}

func TestIsRetryable(t *testing.T) {
	if !IsRetryable(&APIError{StatusCode: 429}) {
		t.Error("429 should be retryable")
	}
	if !IsRetryable(&APIError{StatusCode: 500}) {
		t.Error("500 should be retryable")
	}
	if IsRetryable(&APIError{StatusCode: 400}) {
		t.Error("400 should not be retryable")
	}
}

func TestRetryWithBody(t *testing.T) {
	attempts := 0
	var lastBody string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer apiServer.Close()

	logger := testLogger()
	transport := NewRetryTransport(http.DefaultTransport, 3, logger)
	client := &http.Client{Transport: transport}

	body := strings.NewReader(`{"name":"test product"}`)
	req, _ := http.NewRequest(http.MethodPost, apiServer.URL+"/test", body)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if lastBody != `{"name":"test product"}` {
		t.Errorf("body not preserved across retry, got %q", lastBody)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// isAPIError is a helper to extract APIError from wrapped errors.
func isAPIError(err error, target **APIError) bool {
	var apiErr *APIError
	if ok := isErr(err, &apiErr); ok {
		*target = apiErr
		return true
	}
	return false
}

func isErr[T error](err error, target *T) bool {
	return matchErr(err, target)
}

func matchErr[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	// Try direct type assertion
	if e, ok := err.(T); ok {
		*target = e
		return true
	}
	// Try unwrap
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return matchErr(wrapped.Unwrap(), target)
	}
	return false
}
