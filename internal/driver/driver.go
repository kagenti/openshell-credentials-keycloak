// Package driver implements the OpenShell CredentialsDriver gRPC service for
// Keycloak. It resolves credential placeholders to OAuth2 access tokens using
// the client_credentials grant type.
package driver

import (
	"context"
	"log/slog"
	"sort"

	pb "github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Driver struct {
	pb.UnimplementedCredentialsDriverServer

	config  *Config
	secrets map[string]string
	cache   *TokenCache
	logger  *slog.Logger
}

func New(config *Config, secrets map[string]string, fetcher TokenFetcher, logger *slog.Logger) *Driver {
	return &Driver{
		config:  config,
		secrets: secrets,
		cache:   NewTokenCache(fetcher),
		logger:  logger,
	}
}

func (d *Driver) ResolveCredential(ctx context.Context, req *pb.ResolveCredentialRequest) (*pb.ResolveCredentialResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "credential name is required")
	}

	entry, ok := d.config.Credentials[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "credential %q not configured", req.Name)
	}

	secret, ok := d.secrets[req.Name]
	if !ok {
		return nil, status.Errorf(codes.Internal, "secret not resolved for credential %q", req.Name)
	}

	token, tokenType, expiresAt, err := d.cache.Get(ctx, req.Name, entry, secret)
	if err != nil {
		d.logger.Error("token fetch failed", "credential", req.Name, "error", err)
		return nil, status.Errorf(codes.Unavailable, "failed to resolve credential %q: %v", req.Name, err)
	}

	d.logger.Info("credential resolved", "name", req.Name, "expires_at", expiresAt)

	return &pb.ResolveCredentialResponse{
		Token:       token,
		ExpiresAtMs: expiresAt.UnixMilli(),
		TokenType:   tokenType,
	}, nil
}

func (d *Driver) ListCredentials(ctx context.Context, req *pb.ListCredentialsRequest) (*pb.ListCredentialsResponse, error) {
	creds := make([]*pb.CredentialInfo, 0, len(d.config.Credentials))

	for name, entry := range d.config.Credentials {
		creds = append(creds, &pb.CredentialInfo{
			Name:   name,
			Scopes: entry.Scopes,
		})
	}

	sort.Slice(creds, func(i, j int) bool {
		return creds[i].Name < creds[j].Name
	})

	return &pb.ListCredentialsResponse{
		Credentials: creds,
	}, nil
}
