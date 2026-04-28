FROM golang:1.23-bookworm AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /openshell-credentials-keycloak ./cmd/driver/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /openshell-credentials-keycloak /openshell-credentials-keycloak
ENTRYPOINT ["/openshell-credentials-keycloak"]
