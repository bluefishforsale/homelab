.PHONY: all build test test-verbose test-coverage lint clean run deps \
        docker-build docker-test docker-up docker-down docker-logs \
        docker-build-linux docker-build-multi docker-restart docker-test-integration docker-clean \
        build-linux build-linux-arm build-darwin build-darwin-arm build-all

# Build settings
BINARY_NAME=ai-corp
GO=go
IMAGE_NAME=ai-corp
IMAGE_TAG=latest

all: deps test build

# Install dependencies
deps:
	$(GO) mod tidy
	$(GO) mod download

# Build the binary
build:
	CGO_ENABLED=0 $(GO) build -o $(BINARY_NAME) .

# Run all tests
test:
	$(GO) test -v ./...

# Run tests with verbose output
test-verbose:
	$(GO) test -v -count=1 ./...

# Run tests with coverage
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run tests for a specific package
test-config:
	$(GO) test -v -run TestLoadConfig ./...

test-types:
	$(GO) test -v -run Test.*JSON ./...

test-providers:
	$(GO) test -v -run TestProvider ./...
	$(GO) test -v -run TestDummy ./...
	$(GO) test -v -run TestOpenAI ./...

test-handlers:
	$(GO) test -v -run TestHandler ./...
	$(GO) test -v -run TestCORS ./...
	$(GO) test -v -run TestLogging ./...

test-orchestrator:
	$(GO) test -v -run TestOrchestrator ./...

test-board:
	$(GO) test -v -run TestBoard ./...
	$(GO) test -v -run TestVote ./...
	$(GO) test -v -run TestTally ./...

test-scheduler:
	$(GO) test -v -run TestSchedul ./...
	$(GO) test -v -run TestTask ./...
	$(GO) test -v -run TestNext ./...

test-storage:
	$(GO) test -v -run TestStorage ./...
	$(GO) test -v -run TestLocal ./...

test-org:
	$(GO) test -v -run TestOrganization ./...
	$(GO) test -v -run TestEmployee ./...
	$(GO) test -v -run TestQuality ./...
	$(GO) test -v -run TestCreate ./...
	$(GO) test -v -run TestFormat ./...

# Run quick tests (skip slow tests)
test-quick:
	$(GO) test -v -short ./...

# Run the application
run: build
	./$(BINARY_NAME)

# Run with hot reload (requires air)
dev:
	air

# Lint the code (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format code
fmt:
	$(GO) fmt ./...
	goimports -w .

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	$(GO) clean

# Check for race conditions
test-race:
	$(GO) test -race -v ./...

# Benchmark tests
bench:
	$(GO) test -bench=. -benchmem ./...

# ============================================
# Docker targets for development and deployment
# ============================================

# Build Docker image for current platform (macOS dev)
docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Build Docker image for Linux amd64 (production target)
docker-build-linux:
	docker build --platform linux/amd64 -t $(IMAGE_NAME):$(IMAGE_TAG)-linux .

# Build multi-architecture image (requires buildx)
docker-build-multi:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		--push .

# Run tests in Docker container
docker-test:
	docker build --target tester -t $(IMAGE_NAME):test .
	@echo "Tests passed in Docker container"

# Start development stack (from project root)
docker-up:
	cd .. && docker compose -f docker-compose.dev.yml up --build -d

# Stop development stack
docker-down:
	cd .. && docker compose -f docker-compose.dev.yml down

# View logs
docker-logs:
	cd .. && docker compose -f docker-compose.dev.yml logs -f ai-corp

# Rebuild and restart just the app container
docker-restart:
	cd .. && docker compose -f docker-compose.dev.yml up --build -d ai-corp

# Run tests with full stack
docker-test-integration:
	cd .. && docker compose -f docker-compose.dev.yml --profile test up --build test

# Clean Docker artifacts
docker-clean:
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) $(IMAGE_NAME):test 2>/dev/null || true
	cd .. && docker compose -f docker-compose.dev.yml down -v

# ============================================
# Cross-compilation targets (without Docker)
# ============================================

# Build for Linux amd64 (typical server)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o $(BINARY_NAME)-linux-amd64 .

# Build for Linux arm64 (Raspberry Pi, ARM servers)
build-linux-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags="-s -w" -o $(BINARY_NAME)-linux-arm64 .

# Build for macOS amd64
build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="-s -w" -o $(BINARY_NAME)-darwin-amd64 .

# Build for macOS arm64 (Apple Silicon)
build-darwin-arm:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o $(BINARY_NAME)-darwin-arm64 .

# Build all platforms
build-all: build-linux build-linux-arm build-darwin build-darwin-arm

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "  Local Development:"
	@echo "    deps              - Download and tidy dependencies"
	@echo "    build             - Build binary for current platform"
	@echo "    test              - Run all tests"
	@echo "    test-verbose      - Run tests with verbose output"
	@echo "    test-coverage     - Run tests with coverage report"
	@echo "    test-quick        - Run quick tests (skip slow)"
	@echo "    test-race         - Run tests with race detector"
	@echo "    run               - Build and run the application"
	@echo "    lint              - Run linter"
	@echo "    fmt               - Format code"
	@echo "    clean             - Remove build artifacts"
	@echo ""
	@echo "  Docker Development (macOS):"
	@echo "    docker-build      - Build Docker image for current platform"
	@echo "    docker-test       - Run tests in Docker container"
	@echo "    docker-up         - Start development stack"
	@echo "    docker-down       - Stop development stack"
	@echo "    docker-logs       - View app container logs"
	@echo "    docker-restart    - Rebuild and restart app container"
	@echo "    docker-clean      - Remove Docker images and volumes"
	@echo ""
	@echo "  Production Build:"
	@echo "    docker-build-linux- Build Docker image for Linux amd64"
	@echo "    docker-build-multi- Build multi-arch image (requires buildx)"
	@echo "    build-linux       - Build binary for Linux amd64"
	@echo "    build-linux-arm   - Build binary for Linux arm64"
	@echo "    build-all         - Build binaries for all platforms"
