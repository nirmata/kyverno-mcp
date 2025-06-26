.DEFAULT_GOAL := help

BIN_DIR        := ./bin
CMD_DIR        := ./cmd
BINARY_NAME    := kyverno-mcp
BINARY_PATH    := $(BIN_DIR)/$(BINARY_NAME)

GOPATH_BIN     := $(shell go env GOPATH 2>/dev/null)/bin
ifeq ($(strip $(GOPATH_BIN)),/bin)
  GOPATH_BIN := $(HOME)/go/bin
endif
export GOPATH_BIN

VERSION       ?= $(shell git describe --dirty --always 2>/dev/null || echo dev)

GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)

ifneq ($(wildcard .env),)
  include .env
  ENV_VARS_TO_EXPORT := $(shell awk -F= '{print $$1}' .env | xargs)
  export $(ENV_VARS_TO_EXPORT)
endif

help: ## Show this help
	@echo "Kyverno-MCP  –  available targets:"
	@awk 'BEGIN{FS=":.*?## "}/^[a-zA-Z0-9_-]+:.*## /{printf "  %-18s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

build: fmt vet tidy ## Build binary for current platform
	@echo "Building $(BINARY_NAME) ($(GOOS)/$(GOARCH))..."
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="-s -w -X main.VERSION=$(VERSION)" \
	  -o $(BINARY_PATH) $(CMD_DIR)

cross: ## Cross-compile – override GOOS / GOARCH:  make cross GOOS=darwin GOARCH=arm64
	$(MAKE) build GOOS=$(GOOS) GOARCH=$(GOARCH)

run: ## Run from source
	@echo "Running from source..."
	go run $(CMD_DIR)

inspect: build ## Run the built binary through MCP Inspector
	@echo "Launching MCP Inspector..."
	npx @modelcontextprotocol/inspector $(BINARY_PATH) --kubeconfig $$HOME/.kube/config

fmt: ## go fmt
	@echo "go fmt ..."
	go fmt ./...

vet: ## go vet
	@echo "go vet ..."
	go vet ./...

tidy: ## go mod tidy
	@echo "go mod tidy ..."
	go mod tidy

check: build ## fmt + vet + tidy + build
	@echo "✔ All checks passed."

deps: ## go mod download
	@echo "Downloading modules..."
	go mod download

update-deps: ## go get -u ./...  + tidy
	@echo "Updating all modules to latest..."
	go get -u ./...
	$(MAKE) tidy

install: build ## Copy binary into GOPATH/bin
	@echo "Installing to $(GOPATH_BIN)..."
	mkdir -p $(GOPATH_BIN)
	cp $(BINARY_PATH) $(GOPATH_BIN)/

KO_PLATFORMS ?= linux/amd64,linux/arm64
KO_TAG      ?= latest
KO_DOCKER_REPO ?= ghcr.io/nirmata/kyverno-mcp

ko-build:
	@echo "ko build ($(KO_PLATFORMS))"
	ko build ./cmd \
	--platform=$(KO_PLATFORMS) \
	--tags=$(KO_TAG)

ko-push:
	@echo "ko build --push ($(KO_PLATFORMS))"
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) \
	ko build ./cmd \
	--platform=$(KO_PLATFORMS) \
	--tags=$(KO_TAG) \
	--push

clean:
	@echo "Cleaning…"
	rm -rf $(BIN_DIR)

.PHONY: help build run cross inspect fmt vet tidy check clean deps update-deps \
        install ko-build ko-push