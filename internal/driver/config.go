package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type CredentialEntry struct {
	ClientID        string   `yaml:"client_id"`
	ClientSecretRef string   `yaml:"client_secret_ref"`
	TokenEndpoint   string   `yaml:"token_endpoint"`
	Scopes          []string `yaml:"scopes"`
}

type Config struct {
	Credentials map[string]CredentialEntry `yaml:"credentials"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if len(cfg.Credentials) == 0 {
		return nil, fmt.Errorf("config %s: no credentials defined", path)
	}

	for name, entry := range cfg.Credentials {
		if entry.ClientID == "" {
			return nil, fmt.Errorf("credential %q: client_id is required", name)
		}
		if entry.ClientSecretRef == "" {
			return nil, fmt.Errorf("credential %q: client_secret_ref is required", name)
		}
		if entry.TokenEndpoint == "" {
			return nil, fmt.Errorf("credential %q: token_endpoint is required", name)
		}
	}

	return &cfg, nil
}

// ResolveSecrets reads client secrets from files in secretsDir. Each
// credential's client_secret_ref maps to a file name in the directory.
// Returns a map of credential name to resolved secret value.
func (c *Config) ResolveSecrets(secretsDir string) (map[string]string, error) {
	secrets := make(map[string]string, len(c.Credentials))

	for name, entry := range c.Credentials {
		path := filepath.Join(secretsDir, entry.ClientSecretRef)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("credential %q: read secret %s: %w", name, path, err)
		}
		secrets[name] = strings.TrimSpace(string(data))
	}

	return secrets, nil
}
