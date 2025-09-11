# AuroraBoot Makefile
# Development and build targets for AuroraBoot

.PHONY: help dev build-js build-go build clean clean-js clean-go run-docker build-docker test

# Default target
help: ## Show this help message
	@echo "AuroraBoot Development Makefile"
	@echo "================================"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Usage examples:"
	@echo "  make dev                           # Run with default settings"
	@echo "  make dev ARGS=\"--builds-dir /builds\"  # Run with custom builds directory"
	@echo "  make dev ARGS=\"--address :9090 --create-worker\"  # Run with custom address and worker"
	@echo ""
	@echo "Available web command flags:"
	@echo "  --address string     Listen address (default: :8080)"
	@echo "  --artifact-dir string Artifact directory (default: /tmp/artifacts)"
	@echo "  --builds-dir string  Directory to store build jobs (default: /tmp/kairos-builds)"
	@echo "  --create-worker      Start a local worker in a goroutine"

# Development target - builds everything and runs with local binary
dev: build-js build-go ## Build JS assets, compile Go binary, and run Docker image with mounted binary
	@echo "Starting AuroraBoot in development mode..."
	@mkdir -p builds
	docker run --rm \
		--privileged \
		--net host \
		--entrypoint /usr/sbin/auroraboot \
		-v $(PWD)/auroraboot:/usr/sbin/auroraboot \
		-v $(PWD)/builds:/builds \
		-v /dev:/dev \
		-v /var/run/docker.sock:/var/run/docker.sock \
		quay.io/kairos/auroraboot web --builds-dir /builds --create-worker $(ARGS) --default-kairos-init-version v0.4.9

# Build JavaScript assets (only if needed)
build-js: internal/web/app/package.json ## Build JavaScript and CSS assets
	@echo "Installing JS dependencies..."
	npm install --prefix internal/web/app
	@echo "Bundling JavaScript..."
	npx --prefix internal/web/app esbuild ./internal/web/app/index.js --bundle --outfile=internal/web/app/bundle.js
	@echo "Building CSS with Tailwind..."
	npx --prefix internal/web/app tailwindcss -i ./internal/web/app/assets/css/tailwind.css -o internal/web/app/output.css --minify
	@echo "JS assets built successfully!"

# Build Go binary
build-go: ## Build the Go binary
	@echo "Building Go binary..."
	go build -ldflags "-X main.version=v0.0.0" -o auroraboot
	@echo "Go binary built successfully!"

# Build everything
build: build-js build-go ## Build all assets and Go binary

# Clean JavaScript build artifacts (but keep node_modules)
clean-js: ## Clean JavaScript build artifacts (keeps node_modules)
	@echo "Cleaning JS build artifacts..."
	rm -f internal/web/app/bundle.js
	rm -f internal/web/app/output.css
	@echo "JS artifacts cleaned!"

# Clean JavaScript dependencies (removes node_modules)
clean-js-deps: ## Clean JavaScript dependencies (removes node_modules)
	@echo "Cleaning JS dependencies..."
	rm -rf internal/web/app/node_modules
	rm -f internal/web/app/package-lock.json
	@echo "JS dependencies cleaned!"

# Clean Go build artifacts
clean-go: ## Clean Go build artifacts
	@echo "Cleaning Go build artifacts..."
	rm -f auroraboot
	@echo "Go artifacts cleaned!"

# Clean everything (including node_modules)
clean: clean-js-deps clean-go ## Clean all build artifacts including dependencies

# Clean build artifacts only (keep dependencies)
clean-build: clean-js clean-go ## Clean build artifacts only (keep dependencies)

# Run the Docker image (assumes binary is already built)
run-docker: ## Run the Docker image with mounted binary
	@if [ ! -f auroraboot ]; then echo "Error: auroraboot binary not found. Run 'make build-go' first."; exit 1; fi
	@mkdir -p builds
	docker run --rm \
		--net host \
		-v $(PWD)/auroraboot:/usr/sbin/auroraboot \
		-v $(PWD)/builds:/builds \
		quay.io/kairos/auroraboot web --builds-dir /builds --create-worker $(ARGS)

# Build Docker image
build-docker: ## Build the Docker image
	docker build -t auroraboot:local .

# Run tests
test: ## Run tests
	go test ./...

# Install dependencies (for development)
install-deps: ## Install development dependencies
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Installing JS dependencies..."
	npm install --prefix internal/web/app
	@echo "Dependencies installed!"

# Format code
fmt: ## Format Go code
	go fmt ./...

# Lint code
lint: ## Lint Go code
	golangci-lint run

# Generate swagger docs
swagger: ## Generate swagger documentation
	swag init -g main.go --output internal/web/app --parseDependency --parseInternal --parseDepth 1 --parseVendor

# Quick development cycle (build and run)
quick: build-js build-go run-docker ## Quick build and run cycle


# Test targets
test-js: build-js build-go ## Run JavaScript E2E tests (starts server, runs tests, stops server)
	@echo "Starting AuroraBoot server for testing..."
	@mkdir -p builds
	@# Start server in background and capture PID
	@docker run --rm -d \
		--name auroraboot-test-server \
		--net host \
		-v $(PWD)/auroraboot:/usr/bin/auroraboot \
		-v $(PWD)/builds:/builds \
		quay.io/kairos/auroraboot web --builds-dir /builds --create-worker > /tmp/auroraboot-test.pid || true
	@echo "Waiting for server to be ready..."
	@# Wait for server to be ready (up to 30 seconds)
	@for i in $$(seq 1 30); do \
		if curl -s http://localhost:8080 > /dev/null 2>&1; then \
			echo "Server is ready!"; \
			break; \
		fi; \
		echo "Waiting for server... ($$i/30)"; \
		sleep 1; \
	done
	@# Run the tests with proper cleanup
	@echo "Running JavaScript E2E tests..."
	@cd e2e/web && npm install && npm test; \
	TEST_EXIT_CODE=$$?; \
	echo "Stopping test server..."; \
	docker stop auroraboot-test-server > /dev/null 2>&1 || true; \
	rm -f /tmp/auroraboot-test.pid; \
	echo "Tests completed!"; \
	exit $$TEST_EXIT_CODE

test-js-open: build-js build-go ## Run JavaScript E2E tests in interactive mode (starts server, opens Cypress, stops server)
	@echo "Starting AuroraBoot server for testing..."
	@mkdir -p builds
	@# Start server in background
	@docker run --rm -d \
		--name auroraboot-test-server \
		--net host \
		-v $(PWD)/auroraboot:/usr/bin/auroraboot \
		-v $(PWD)/builds:/builds \
		quay.io/kairos/auroraboot web --builds-dir /builds --create-worker > /tmp/auroraboot-test.pid || true
	@echo "Waiting for server to be ready..."
	@# Wait for server to be ready (up to 30 seconds)
	@for i in $$(seq 1 30); do \
		if curl -s http://localhost:8080 > /dev/null 2>&1; then \
			echo "Server is ready!"; \
			break; \
		fi; \
		echo "Waiting for server... ($$i/30)"; \
		sleep 1; \
	done
	@# Run the tests in interactive mode with proper cleanup
	@echo "Opening Cypress in interactive mode..."
	@cd e2e/web && npm install && npm run test:open; \
	TEST_EXIT_CODE=$$?; \
	echo "Stopping test server..."; \
	docker stop auroraboot-test-server > /dev/null 2>&1 || true; \
	rm -f /tmp/auroraboot-test.pid; \
	echo "Interactive tests completed!"; \
	exit $$TEST_EXIT_CODE

test-js-stop: ## Stop any running test server
	@echo "Stopping test server..."
	@docker stop auroraboot-test-server > /dev/null 2>&1 || true
	@rm -f /tmp/auroraboot-test.pid
	@echo "Test server stopped!"

