# cnpg-ui — Makefile
# Targets: build, test, lint, generate, run

BINARY     := cnpg-ui
BINARY_OUT := bin/$(BINARY)
MODULE     := github.com/dbonne/cnpg-ui
CMD_PKG    := ./cmd/cnpg-ui

# Go toolchain flags
GOFLAGS  :=
LDFLAGS  := -trimpath
BUILD_FLAGS := -v $(LDFLAGS)

# Tool versions (pinned via go install)
GOLANGCI_LINT_VERSION := v1.64.8
OAPI_CODEGEN_VERSION  := v2.4.1

.PHONY: all build test lint generate clean run help

## all: Build the binary (default target)
all: build

## build: Compile the binary to bin/cnpg-ui
build:
	@mkdir -p bin
	go build $(BUILD_FLAGS) -o $(BINARY_OUT) $(CMD_PKG)

## test: Run all unit tests with race detector and coverage
test:
	go test -race -cover ./...

## test-coverage: Run tests and produce an HTML coverage report
test-coverage:
	go test -race -coverprofile=cover.out -covermode=atomic ./...
	go tool cover -html=cover.out -o cover.html
	@echo "Coverage report: cover.html"

## lint: Run golangci-lint (install with: make lint-install)
lint:
	golangci-lint run ./...

## lint-install: Install golangci-lint at the pinned version
lint-install:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

## generate: Run oapi-codegen to regenerate API types from api/openapi.yaml
generate:
	go generate ./...

## oapi-codegen-install: Install oapi-codegen at the pinned version
oapi-codegen-install:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)

## run: Run the server locally (reads CNPG_UI_* env vars)
run: build
	./$(BINARY_OUT)

## clean: Remove build artefacts
clean:
	rm -rf bin cover.out cover.html

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
