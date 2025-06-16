# Makefile for kyverno-mcp
#
# This Makefile provides a set of commands to build, run,
# and manage the kyverno-mcp project.

# Default target to run when no target is specified.
.DEFAULT_GOAL := help

# --- Variables ---
# Define common variables to avoid repetition and ease maintenance.
BIN_DIR      := ./bin
CMD_DIR      := ./cmd
BINARY_NAME  := kyverno-mcp
BINARY_PATH  := $(BIN_DIR)/$(BINARY_NAME)

# Attempt to determine GOPATH/bin for installation.
# Fallback to a common default if `go env GOPATH` fails or is empty.
GOPATH_BIN   := $(shell go env GOPATH)/bin
ifeq ($(GOPATH_BIN),/bin)
	GOPATH_BIN := $(HOME)/go/bin
endif

# --- Environment Variables from .env ---
# If a .env file exists, include it. This makes variables defined in .env
# (e.g., API_KEY=123) available as Make variables.
# Then, export these variables so they are available in the environment
# for shell commands executed by Make recipes.
ifneq ($(wildcard .env),)
	include .env
	# Extract variable names from .env and export them.
	# This assumes .env contains lines like VAR=value.
	ENV_VARS_TO_EXPORT := $(shell awk -F= '{print $$1}' .env | xargs)
	export $(ENV_VARS_TO_EXPORT)
endif

# --- Help Target ---
# Displays a list of available targets and their descriptions.
# Descriptions are extracted from comments following '##'.
help:
	@echo "kyverno-mcp Makefile"
	@echo "-------------------"
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# --- Build Tasks ---
build: ## Build single binary for the current platform
	@echo "λ Building $(BINARY_NAME) for current platform..."
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY_PATH) $(CMD_DIR)

# --- Run Tasks ---
run: ## Run the application
	@echo "λ Running $(BINARY_NAME) from source..."
	go run $(CMD_DIR)

# --- Code Quality Tasks ---
fmt: ## Format code using go fmt
	@echo "λ Formatting code (using go fmt)..."
	go fmt ./...

vet: ## Run go vet using go vet
	@echo "λ Running go vet (using go vet)..."
	go vet ./...

tidy: ## Tidy go modules using go mod tidy
	@echo "λ Tidying go modules (using go mod tidy)..."
	rm -f go.sum
	go mod tidy

# --- Combined Tasks ---
# 'check' depends on other verification tasks. They will run as prerequisites.
check: fmt vet tidy build ## Run all verification checks
	@echo "λ All checks completed."

# --- Maintenance Tasks ---
clean: ## Clean build artifacts
	@echo "λ Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	go mod tidy

deps: ## Download Go module dependencies
	@echo "λ Downloading Go module dependencies..."
	go mod download

update-deps: ## Update Go module dependencies and then tidy
	@echo "λ Updating Go module dependencies..."
	go get -u ./...
	@echo "λ Tidying modules after update..."
	$(MAKE) tidy

# --- Installation ---
# 'install' depends on the 'build' target.
install: build ## Install the binary to $(GOPATH_BIN)
	@echo "λ Installing $(BINARY_NAME) to $(GOPATH_BIN)..."
	cp $(BINARY_PATH) $(GOPATH_BIN)/
	@echo "$(BINARY_NAME) installed."