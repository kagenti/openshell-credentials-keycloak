package grpctest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1"
	"github.com/kagenti/openshell-credentials-keycloak/internal/driver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func startMockKeycloak(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"unsupported_grant_type"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-access-token-" + r.FormValue("client_id"),
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	}))
}

func startTestServer(t *testing.T, keycloakURL string) (pb.CredentialsDriverClient, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "grpctest-creds")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "creds.sock")

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on UDS: %v", err)
	}

	// Write test config and secrets.
	configDir := filepath.Join(tmpDir, "config")
	secretsDir := filepath.Join(tmpDir, "secrets")
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(secretsDir, 0755)

	configYAML := `
credentials:
  test-api:
    client_id: test-client
    client_secret_ref: test-secret
    token_endpoint: ` + keycloakURL + `
    scopes: ["read", "write"]
  other-api:
    client_id: other-client
    client_secret_ref: other-secret
    token_endpoint: ` + keycloakURL + `
`
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte(configYAML), 0644)
	os.WriteFile(filepath.Join(secretsDir, "test-secret"), []byte("test-secret-value"), 0644)
	os.WriteFile(filepath.Join(secretsDir, "other-secret"), []byte("other-secret-value"), 0644)

	cfg, err := driver.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	secrets, err := cfg.ResolveSecrets(secretsDir)
	if err != nil {
		t.Fatalf("resolve secrets: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	fetcher := &driver.HTTPTokenFetcher{Client: http.DefaultClient}
	drv := driver.New(cfg, secrets, fetcher, logger)

	srv := grpc.NewServer()
	pb.RegisterCredentialsDriverServer(srv, drv)

	go func() {
		_ = srv.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		t.Fatalf("dial UDS: %v", err)
	}

	client := pb.NewCredentialsDriverClient(conn)

	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
	}

	return client, cleanup
}

func TestGRPC_ResolveCredential(t *testing.T) {
	kc := startMockKeycloak(t)
	defer kc.Close()

	client, cleanup := startTestServer(t, kc.URL)
	defer cleanup()

	resp, err := client.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{
		Name: "test-api",
	})
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resp.Token != "mock-access-token-test-client" {
		t.Errorf("expected 'mock-access-token-test-client', got %q", resp.Token)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got %q", resp.TokenType)
	}
	if resp.ExpiresAtMs <= 0 {
		t.Error("expected positive expires_at_ms")
	}
}

func TestGRPC_ResolveCredential_NotFound(t *testing.T) {
	kc := startMockKeycloak(t)
	defer kc.Close()

	client, cleanup := startTestServer(t, kc.URL)
	defer cleanup()

	_, err := client.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{
		Name: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent credential")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

func TestGRPC_ResolveCredential_EmptyName(t *testing.T) {
	kc := startMockKeycloak(t)
	defer kc.Close()

	client, cleanup := startTestServer(t, kc.URL)
	defer cleanup()

	_, err := client.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{
		Name: "",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestGRPC_ListCredentials(t *testing.T) {
	kc := startMockKeycloak(t)
	defer kc.Close()

	client, cleanup := startTestServer(t, kc.URL)
	defer cleanup()

	resp, err := client.ListCredentials(context.Background(), &pb.ListCredentialsRequest{})
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(resp.Credentials) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(resp.Credentials))
	}
	// Sorted by name.
	if resp.Credentials[0].Name != "other-api" {
		t.Errorf("expected first 'other-api', got %q", resp.Credentials[0].Name)
	}
	if resp.Credentials[1].Name != "test-api" {
		t.Errorf("expected second 'test-api', got %q", resp.Credentials[1].Name)
	}
}

func TestGRPC_ResolveCredential_CachesToken(t *testing.T) {
	callCount := 0
	kc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"expires_in":   300,
			"token_type":   "Bearer",
		})
	}))
	defer kc.Close()

	client, cleanup := startTestServer(t, kc.URL)
	defer cleanup()

	// First call.
	_, err := client.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{Name: "test-api"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call should hit cache.
	_, err = client.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{Name: "test-api"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 Keycloak call (cached), got %d", callCount)
	}
}
