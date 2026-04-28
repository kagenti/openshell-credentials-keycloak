package driver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockFetcher struct {
	calls    atomic.Int32
	response *TokenResponse
	err      error
}

func (m *mockFetcher) FetchToken(ctx context.Context, endpoint, clientID, clientSecret string, scopes []string) (*TokenResponse, error) {
	m.calls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestTokenCache_CacheHit(t *testing.T) {
	fetcher := &mockFetcher{
		response: &TokenResponse{
			AccessToken: "token-1",
			ExpiresIn:   300,
			TokenType:   "bearer",
		},
	}

	cache := NewTokenCache(fetcher)
	entry := CredentialEntry{
		ClientID:      "test-client",
		TokenEndpoint: "https://example.com/token",
	}

	// First call: cache miss, should fetch.
	token, _, _, err := cache.Get(context.Background(), "test", entry, "secret")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if token != "token-1" {
		t.Errorf("expected 'token-1', got %q", token)
	}
	if fetcher.calls.Load() != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.calls.Load())
	}

	// Second call: cache hit, should not fetch.
	token, _, _, err = cache.Get(context.Background(), "test", entry, "secret")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if token != "token-1" {
		t.Errorf("expected 'token-1', got %q", token)
	}
	if fetcher.calls.Load() != 1 {
		t.Errorf("expected still 1 fetch call, got %d", fetcher.calls.Load())
	}
}

func TestTokenCache_RefreshExpired(t *testing.T) {
	now := time.Now()
	callCount := 0
	fetcher := &mockFetcher{
		response: &TokenResponse{
			AccessToken: "token-fresh",
			ExpiresIn:   300,
			TokenType:   "bearer",
		},
	}

	cache := NewTokenCache(fetcher)
	cache.nowFunc = func() time.Time { return now }

	entry := CredentialEntry{
		ClientID:      "test-client",
		TokenEndpoint: "https://example.com/token",
	}

	// First call at t=0
	_, _, _, err := cache.Get(context.Background(), "test", entry, "secret")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	callCount++

	// Advance time past expiry (300s) minus margin (30s) = 270s
	cache.nowFunc = func() time.Time { return now.Add(271 * time.Second) }

	fetcher.response = &TokenResponse{
		AccessToken: "token-refreshed",
		ExpiresIn:   300,
		TokenType:   "bearer",
	}

	token, _, _, err := cache.Get(context.Background(), "test", entry, "secret")
	if err != nil {
		t.Fatalf("expired call: %v", err)
	}
	if token != "token-refreshed" {
		t.Errorf("expected 'token-refreshed', got %q", token)
	}
	if fetcher.calls.Load() != 2 {
		t.Errorf("expected 2 fetch calls, got %d", fetcher.calls.Load())
	}
}

func TestTokenCache_ConcurrentAccess(t *testing.T) {
	fetcher := &mockFetcher{
		response: &TokenResponse{
			AccessToken: "token-concurrent",
			ExpiresIn:   300,
			TokenType:   "bearer",
		},
	}

	cache := NewTokenCache(fetcher)
	entry := CredentialEntry{
		ClientID:      "test-client",
		TokenEndpoint: "https://example.com/token",
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := cache.Get(context.Background(), "test", entry, "secret")
			if err != nil {
				t.Errorf("concurrent get: %v", err)
			}
		}()
	}
	wg.Wait()

	// Due to double-check locking, should be exactly 1 fetch.
	calls := fetcher.calls.Load()
	if calls != 1 {
		t.Errorf("expected 1 fetch call with concurrent access, got %d", calls)
	}
}

func TestHTTPTokenFetcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected content-type: %s", ct)
		}

		r.ParseForm()
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("expected grant_type=client_credentials, got %q", r.FormValue("grant_type"))
		}
		if r.FormValue("client_id") != "my-client" {
			t.Errorf("expected client_id=my-client, got %q", r.FormValue("client_id"))
		}
		if r.FormValue("client_secret") != "my-secret" {
			t.Errorf("expected client_secret=my-secret, got %q", r.FormValue("client_secret"))
		}
		if r.FormValue("scope") != "read write" {
			t.Errorf("expected scope='read write', got %q", r.FormValue("scope"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "test-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	fetcher := &HTTPTokenFetcher{Client: srv.Client()}
	resp, err := fetcher.FetchToken(context.Background(), srv.URL, "my-client", "my-secret", []string{"read", "write"})
	if err != nil {
		t.Fatalf("fetch token: %v", err)
	}
	if resp.AccessToken != "test-access-token" {
		t.Errorf("expected 'test-access-token', got %q", resp.AccessToken)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected expires_in=3600, got %d", resp.ExpiresIn)
	}
}

func TestHTTPTokenFetcher_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	fetcher := &HTTPTokenFetcher{Client: srv.Client()}
	_, err := fetcher.FetchToken(context.Background(), srv.URL, "bad-client", "bad-secret", nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestHTTPTokenFetcher_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "",
			"expires_in":   300,
		})
	}))
	defer srv.Close()

	fetcher := &HTTPTokenFetcher{Client: srv.Client()}
	_, err := fetcher.FetchToken(context.Background(), srv.URL, "client", "secret", nil)
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
}
