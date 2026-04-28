# openshell-credentials-keycloak

OpenShell credentials driver that resolves credential placeholders to OAuth2 access tokens via Keycloak's `client_credentials` grant.

Runs as a sidecar container in the OpenShell gateway pod, communicating over a Unix domain socket.

## Build

```bash
make build
```

## Test

```bash
make test
```

## Run

```bash
./openshell-credentials-keycloak \
  --socket /run/drivers/credentials.sock \
  --config /etc/openshell/credentials.yaml \
  --secrets-dir /etc/openshell/secrets
```

## Configuration

```yaml
credentials:
  github-api:
    client_id: team1-github
    client_secret_ref: team1-github-secret
    token_endpoint: https://keycloak.example.com/realms/openshell/protocol/openid-connect/token
    scopes: ["repo", "read:org"]
```

Client secrets are read from files in `--secrets-dir`. Each credential's `client_secret_ref` maps to a filename in that directory (standard Kubernetes Secret volume mount pattern).

## Docker

```bash
docker build -f deploy/Dockerfile -t openshell-credentials-keycloak:latest .
```

## License

Apache-2.0
