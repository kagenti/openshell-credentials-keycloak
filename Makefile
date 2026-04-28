.PHONY: proto build test test-unit test-grpc test-all clean run

BINARY := openshell-credentials-keycloak
SOCKET := /tmp/openshell-credentials.sock

proto:
	protoc \
		--go_out=gen --go_opt=paths=source_relative \
		--go_opt=Mcredentials_driver.proto=github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1 \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mcredentials_driver.proto=github.com/kagenti/openshell-credentials-keycloak/gen/credentialsv1 \
		-I proto proto/credentials_driver.proto
	mkdir -p gen/credentialsv1
	mv gen/credentials_driver*.go gen/credentialsv1/ 2>/dev/null || true

build:
	go build -o $(BINARY) ./cmd/driver/

test-unit:
	go test ./internal/driver/ -timeout 30s -v

test-grpc:
	go test ./internal/grpctest/ -timeout 30s -v

test: test-unit test-grpc

test-all: test

clean:
	rm -f $(BINARY) $(SOCKET)

run: build
	./$(BINARY) --socket $(SOCKET)
