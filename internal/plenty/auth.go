package plenty

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/janemig/plentyone/internal/storage/queries"
)

// tokenExpiryBuffer is the duration before actual expiry at which we consider
// a token expired and trigger a refresh.
const tokenExpiryBuffer = 5 * time.Minute

// loginResponse represents the JSON response from PlentyONE's login endpoints.
type loginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
}

// TokenStore manages OAuth tokens with MySQL persistence and concurrency-safe
// refresh via a check-lock-check pattern.
type TokenStore struct {
	db       *queries.Queries
	shopURL  string
	username string
	password string
	mu       sync.Mutex
	logger   *slog.Logger

	// authClient is a plain HTTP client used for login/refresh requests.
	// It does not go through the rate-limited transport chain.
	authClient *http.Client
}

// NewTokenStore creates a new TokenStore for managing OAuth tokens.
// The shopURL is the base URL of the PlentyONE shop (e.g., https://myshop.plentymarkets-cloud01.com).
// Username and password should be read from PLENTYONE_API_USERNAME and PLENTYONE_API_PASSWORD
// environment variables by the caller.
func NewTokenStore(db *queries.Queries, shopURL, username, password string, logger *slog.Logger) *TokenStore {
	return &TokenStore{
		db:       db,
		shopURL:  shopURL,
		username: username,
		password: password,
		logger:   logger,
		authClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetValidToken returns a valid access token, refreshing or logging in as needed.
// It uses a check-lock-check pattern to prevent thundering herd when multiple
// goroutines see an expired token simultaneously.
func (ts *TokenStore) GetValidToken(ctx context.Context) (string, error) {
	// First check without lock (fast path)
	token, err := ts.getCachedToken(ctx)
	if err == nil && token != "" {
		return token, nil
	}

	// Acquire lock for refresh/login
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Re-check after acquiring lock (another goroutine may have refreshed)
	token, err = ts.getCachedToken(ctx)
	if err == nil && token != "" {
		return token, nil
	}

	// Try refresh if we have a refresh token
	stored, dbErr := ts.db.GetOAuthToken(ctx, ts.shopURL)
	if dbErr == nil && stored.RefreshToken != "" {
		token, err = ts.Refresh(ctx, stored.RefreshToken)
		if err == nil {
			return token, nil
		}
		ts.logger.Warn("token refresh failed, falling back to login",
			slog.String("error", err.Error()),
		)
	}

	// Fall back to fresh login
	return ts.Login(ctx)
}

// getCachedToken retrieves the cached token from MySQL and returns it if still valid.
func (ts *TokenStore) getCachedToken(ctx context.Context) (string, error) {
	stored, err := ts.db.GetOAuthToken(ctx, ts.shopURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("no cached token")
		}
		return "", fmt.Errorf("querying token: %w", err)
	}

	if time.Now().After(stored.ExpiresAt.Add(-tokenExpiryBuffer)) {
		return "", fmt.Errorf("token expired or expiring soon")
	}

	return stored.AccessToken, nil
}

// Login authenticates with PlentyONE using username/password credentials.
// It persists the returned tokens to MySQL and returns the access token.
func (ts *TokenStore) Login(ctx context.Context) (string, error) {
	ts.logger.Debug("performing PlentyONE login", slog.String("shop_url", ts.shopURL))

	body, err := json.Marshal(map[string]string{
		"username": ts.username,
		"password": ts.password,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.shopURL+"/rest/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.authClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("decoding login response: %w", err)
	}

	if err := ts.persistToken(ctx, &loginResp); err != nil {
		return "", fmt.Errorf("persisting login token: %w", err)
	}

	ts.logger.Debug("login successful", slog.String("shop_url", ts.shopURL))
	return loginResp.AccessToken, nil
}

// Refresh obtains a new token using an existing refresh token.
// Falls back to Login if the refresh fails (token may be fully expired).
func (ts *TokenStore) Refresh(ctx context.Context, refreshToken string) (string, error) {
	ts.logger.Debug("refreshing PlentyONE token", slog.String("shop_url", ts.shopURL))

	body, err := json.Marshal(map[string]string{
		"refreshToken": refreshToken,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.shopURL+"/rest/login/refresh", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.authClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("decoding refresh response: %w", err)
	}

	if err := ts.persistToken(ctx, &loginResp); err != nil {
		return "", fmt.Errorf("persisting refreshed token: %w", err)
	}

	ts.logger.Debug("token refresh successful", slog.String("shop_url", ts.shopURL))
	return loginResp.AccessToken, nil
}

// persistToken saves the token to MySQL via sqlc UpsertOAuthToken.
func (ts *TokenStore) persistToken(ctx context.Context, resp *loginResponse) error {
	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	return ts.db.UpsertOAuthToken(ctx, queries.UpsertOAuthTokenParams{
		ShopUrl:      ts.shopURL,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresAt:    expiresAt,
	})
}

// AuthTransport is an http.RoundTripper that injects Bearer tokens into requests
// and handles 401 responses by re-authenticating and retrying once.
type AuthTransport struct {
	Base       http.RoundTripper
	TokenStore *TokenStore
}

// RoundTrip implements http.RoundTripper. It obtains a valid token, sets the
// Authorization header, and delegates to the Base transport. If a 401 response
// is received, it forces a fresh login and retries the request once.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.TokenStore.GetValidToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("obtaining auth token: %w", err)
	}

	// Clone the request to avoid mutating the original
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.Base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}

	// On 401, force fresh login and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		t.TokenStore.logger.Debug("received 401, forcing re-login")

		t.TokenStore.mu.Lock()
		newToken, loginErr := t.TokenStore.Login(req.Context())
		t.TokenStore.mu.Unlock()

		if loginErr != nil {
			return nil, fmt.Errorf("re-authentication after 401: %w", loginErr)
		}

		// Retry with new token
		req3 := req.Clone(req.Context())
		req3.Header.Set("Authorization", "Bearer "+newToken)

		// Reset body for retry if possible
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("resetting request body for retry: %w", err)
			}
			req3.Body = body
		}

		return t.Base.RoundTrip(req3)
	}

	return resp, nil
}
