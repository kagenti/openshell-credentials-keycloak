// Command driver starts the OpenShell credentials driver for Keycloak. It
// listens on a Unix domain socket and serves the CredentialsDriver gRPC service
// that the OpenShell gateway connects to.
package main

import (
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1"
	"github.com/kagenti/openshell-credentials-keycloak/internal/driver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	socketPath := flag.String("socket", "/run/drivers/credentials.sock",
		"Unix domain socket path for the gRPC server")
	configPath := flag.String("config", "/etc/openshell/credentials.yaml",
		"Path to the credentials configuration file")
	secretsDir := flag.String("secrets-dir", "/etc/openshell/secrets",
		"Directory containing client secret files")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := driver.LoadConfig(*configPath)
	if err != nil {
		logger.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	secrets, err := cfg.ResolveSecrets(*secretsDir)
	if err != nil {
		logger.Error("failed to resolve secrets", "dir", *secretsDir, "error", err)
		os.Exit(1)
	}

	logger.Info("loaded credentials config",
		"credentials", len(cfg.Credentials),
		"config", *configPath,
	)

	// Clean up stale socket from a previous run.
	os.Remove(*socketPath)

	lis, err := net.Listen("unix", *socketPath)
	if err != nil {
		logger.Error("failed to listen", "socket", *socketPath, "error", err)
		os.Exit(1)
	}

	if err := os.Chmod(*socketPath, 0777); err != nil {
		logger.Warn("failed to chmod socket", "error", err)
	}

	fetcher := &driver.HTTPTokenFetcher{Client: http.DefaultClient}
	drv := driver.New(cfg, secrets, fetcher, logger)

	srv := grpc.NewServer()
	pb.RegisterCredentialsDriverServer(srv, drv)
	reflection.Register(srv)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		s := <-sig
		logger.Info("received signal, shutting down", "signal", s)
		srv.GracefulStop()
	}()

	logger.Info("credentials driver listening", "socket", *socketPath)
	if err := srv.Serve(lis); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
