package driver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
credentials:
  github-api:
    client_id: team1-github
    client_secret_ref: team1-github-secret
    token_endpoint: https://keycloak.example.com/realms/openshell/protocol/openid-connect/token
    scopes: ["repo", "read:org"]
  kagenti-backend:
    client_id: team1-kagenti
    client_secret_ref: team1-kagenti-secret
    token_endpoint: https://keycloak.example.com/realms/openshell/protocol/openid-connect/token
    scopes: ["kagenti:read"]
`), 0644)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Credentials) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(cfg.Credentials))
	}

	gh := cfg.Credentials["github-api"]
	if gh.ClientID != "team1-github" {
		t.Errorf("expected client_id 'team1-github', got %q", gh.ClientID)
	}
	if len(gh.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(gh.Scopes))
	}
}

func TestLoadConfig_MissingClientID(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
credentials:
  broken:
    client_secret_ref: some-secret
    token_endpoint: https://keycloak.example.com/token
`), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing client_id")
	}
}

func TestLoadConfig_EmptyCredentials(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`credentials: {}`), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`not: valid: yaml: [}`), 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestResolveSecrets(t *testing.T) {
	dir := t.TempDir()

	// Write config
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
credentials:
  test-cred:
    client_id: my-client
    client_secret_ref: my-secret-file
    token_endpoint: https://keycloak.example.com/token
`), 0644)

	// Write secret file
	secretsDir := filepath.Join(dir, "secrets")
	os.MkdirAll(secretsDir, 0755)
	os.WriteFile(filepath.Join(secretsDir, "my-secret-file"), []byte("super-secret-value\n"), 0644)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	secrets, err := cfg.ResolveSecrets(secretsDir)
	if err != nil {
		t.Fatalf("resolve secrets: %v", err)
	}

	if secrets["test-cred"] != "super-secret-value" {
		t.Errorf("expected 'super-secret-value', got %q", secrets["test-cred"])
	}
}

func TestResolveSecrets_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
credentials:
  test-cred:
    client_id: my-client
    client_secret_ref: nonexistent-secret
    token_endpoint: https://keycloak.example.com/token
`), 0644)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	_, err = cfg.ResolveSecrets(filepath.Join(dir, "empty-secrets"))
	if err == nil {
		t.Fatal("expected error for missing secret file")
	}
}
