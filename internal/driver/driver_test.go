package driver

import (
	"context"
	"log/slog"
	"os"
	"testing"

	pb "github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testDriver() *Driver {
	cfg := &Config{
		Credentials: map[string]CredentialEntry{
			"github-api": {
				ClientID:        "team1-github",
				ClientSecretRef: "team1-github-secret",
				TokenEndpoint:   "https://keycloak.example.com/token",
				Scopes:          []string{"repo", "read:org"},
			},
			"kagenti-backend": {
				ClientID:        "team1-kagenti",
				ClientSecretRef: "team1-kagenti-secret",
				TokenEndpoint:   "https://keycloak.example.com/token",
				Scopes:          []string{"kagenti:read"},
			},
		},
	}

	secrets := map[string]string{
		"github-api":      "gh-secret",
		"kagenti-backend": "kagenti-secret",
	}

	fetcher := &mockFetcher{
		response: &TokenResponse{
			AccessToken: "resolved-token",
			ExpiresIn:   300,
			TokenType:   "bearer",
		},
	}

	return New(cfg, secrets, fetcher, testLogger())
}

func TestResolveCredential_Success(t *testing.T) {
	d := testDriver()
	resp, err := d.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{Name: "github-api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "resolved-token" {
		t.Errorf("expected 'resolved-token', got %q", resp.Token)
	}
	if resp.TokenType != "bearer" {
		t.Errorf("expected token_type 'bearer', got %q", resp.TokenType)
	}
	if resp.ExpiresAtMs <= 0 {
		t.Error("expected positive expires_at_ms")
	}
}

func TestResolveCredential_NotFound(t *testing.T) {
	d := testDriver()
	_, err := d.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{Name: "nonexistent"})
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

func TestResolveCredential_EmptyName(t *testing.T) {
	d := testDriver()
	_, err := d.ResolveCredential(context.Background(), &pb.ResolveCredentialRequest{Name: ""})
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

func TestListCredentials(t *testing.T) {
	d := testDriver()
	resp, err := d.ListCredentials(context.Background(), &pb.ListCredentialsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Credentials) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(resp.Credentials))
	}
	// Results should be sorted by name.
	if resp.Credentials[0].Name != "github-api" {
		t.Errorf("expected first credential 'github-api', got %q", resp.Credentials[0].Name)
	}
	if resp.Credentials[1].Name != "kagenti-backend" {
		t.Errorf("expected second credential 'kagenti-backend', got %q", resp.Credentials[1].Name)
	}
	if len(resp.Credentials[0].Scopes) != 2 {
		t.Errorf("expected 2 scopes for github-api, got %d", len(resp.Credentials[0].Scopes))
	}
}
