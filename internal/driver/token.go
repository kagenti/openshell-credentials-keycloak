package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type cachedToken struct {
	token     string
	tokenType string
	expiresAt time.Time
}

// TokenFetcher abstracts the HTTP call to the token endpoint for testability.
type TokenFetcher interface {
	FetchToken(ctx context.Context, endpoint, clientID, clientSecret string, scopes []string) (*TokenResponse, error)
}

// HTTPTokenFetcher calls a real OAuth2 token endpoint.
type HTTPTokenFetcher struct {
	Client *http.Client
}

func (f *HTTPTokenFetcher) FetchToken(ctx context.Context, endpoint, clientID, clientSecret string, scopes []string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	if len(scopes) > 0 {
		data.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access_token in response")
	}

	return &tokenResp, nil
}

// TokenCache caches tokens per credential name, refreshing them before expiry.
type TokenCache struct {
	mu            sync.RWMutex
	cache         map[string]*cachedToken
	fetcher       TokenFetcher
	refreshMargin time.Duration
	nowFunc       func() time.Time // for testing
}

func NewTokenCache(fetcher TokenFetcher) *TokenCache {
	return &TokenCache{
		cache:         make(map[string]*cachedToken),
		fetcher:       fetcher,
		refreshMargin: 30 * time.Second,
		nowFunc:       time.Now,
	}
}

func (c *TokenCache) Get(ctx context.Context, name string, entry CredentialEntry, clientSecret string) (string, string, time.Time, error) {
	c.mu.RLock()
	cached, ok := c.cache[name]
	c.mu.RUnlock()

	now := c.nowFunc()
	if ok && now.Before(cached.expiresAt.Add(-c.refreshMargin)) {
		return cached.token, cached.tokenType, cached.expiresAt, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	cached, ok = c.cache[name]
	if ok && now.Before(cached.expiresAt.Add(-c.refreshMargin)) {
		return cached.token, cached.tokenType, cached.expiresAt, nil
	}

	resp, err := c.fetcher.FetchToken(ctx, entry.TokenEndpoint, entry.ClientID, clientSecret, entry.Scopes)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("fetch token for %q: %w", name, err)
	}

	expiresAt := now.Add(time.Duration(resp.ExpiresIn) * time.Second)
	tokenType := resp.TokenType
	if tokenType == "" {
		tokenType = "bearer"
	}

	c.cache[name] = &cachedToken{
		token:     resp.AccessToken,
		tokenType: tokenType,
		expiresAt: expiresAt,
	}

	return resp.AccessToken, tokenType, expiresAt, nil
}
