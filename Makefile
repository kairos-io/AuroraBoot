# AuroraBoot Makefile
# Development and build targets for AuroraBoot

.PHONY: help dev ui-install ui-build build-go build clean clean-ui clean-go run-docker build-docker test openapi fmt lint install-deps

# Default target
help: ## Show this help message
	@echo "AuroraBoot Development Makefile"
	@echo "================================"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Common workflows:"
	@echo "  make build                          # Build React UI + Go binary"
	@echo "  make dev                            # Build and run server in Docker"
	@echo "  make openapi                        # Regenerate Swagger spec from annotations"

# Development target - builds everything and runs the server in Docker
dev: build ## Build everything and run AuroraBoot server in Docker
	@echo "Starting AuroraBoot server in development mode..."
	@mkdir -p data
	docker run --rm \
		--privileged \
		--net host \
		--entrypoint /usr/sbin/auroraboot \
		-v $(PWD)/auroraboot:/usr/sbin/auroraboot \
		-v $(PWD)/data:/data \
		-v /dev:/dev \
		-v /var/run/docker.sock:/var/run/docker.sock \
		quay.io/kairos/auroraboot web --data-dir /data $(ARGS)

# Install React UI dependencies
ui-install: ## Install React UI npm dependencies
	@echo "Installing UI dependencies..."
	cd ui && npm install

# Build React UI bundle. vite.config.ts already writes to internal/ui/dist
# so the Go build can pick it up via go:embed.
ui-build: ui-install ## Build the React UI bundle
	@echo "Building React UI..."
	cd ui && npm run build
	@echo "UI built to internal/ui/dist"

# Build Go binary
build-go: ## Build the Go binary
	@echo "Building Go binary..."
	go build -ldflags "-X main.version=v0.0.0" -o auroraboot
	@echo "Go binary built successfully!"

# Build everything
build: ui-build build-go ## Build React UI and Go binary

# Clean UI build artifacts (keeps node_modules)
clean-ui: ## Clean UI build artifacts (keeps node_modules)
	@echo "Cleaning UI build artifacts..."
	rm -rf internal/ui/dist
	rm -rf ui/dist
	@echo "UI artifacts cleaned!"

# Clean UI dependencies (removes node_modules)
clean-ui-deps: ## Clean UI dependencies (removes node_modules)
	@echo "Cleaning UI dependencies..."
	rm -rf ui/node_modules
	@echo "UI dependencies cleaned!"

# Clean Go build artifacts
clean-go: ## Clean Go build artifacts
	@echo "Cleaning Go build artifacts..."
	rm -f auroraboot
	@echo "Go artifacts cleaned!"

# Clean everything (including node_modules)
clean: clean-ui-deps clean-ui clean-go ## Clean all build artifacts including dependencies

# Build Docker image
build-docker: ## Build the Docker image
	docker build -t auroraboot:local .

# Run tests (UI dist is embedded via go:embed, so it must exist before `go test`)
test: ui-build ## Run Go tests
	go test ./...

# Install development dependencies
install-deps: ## Install development dependencies
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Installing UI dependencies..."
	cd ui && npm install
	@echo "Dependencies installed!"

# Format code
fmt: ## Format Go code
	go fmt ./...

# Lint code
lint: ## Lint Go code
	golangci-lint run

# Generate OpenAPI spec from swag annotations on internal/cmd/web.go
openapi: ## Regenerate Swagger/OpenAPI documentation
	@which swag >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g internal/cmd/web.go --output docs --parseDependency --parseInternal --parseDepth 2

# Backwards-compat alias
swagger: openapi
