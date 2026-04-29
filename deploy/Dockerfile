FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /openshell-credentials-keycloak ./cmd/driver/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /openshell-credentials-keycloak /openshell-credentials-keycloak
ENTRYPOINT ["/openshell-credentials-keycloak"]
