# Makefile for Message Aggregator

.PHONY: build-frontend build-backend build-all clean dev-backend dev-frontend docker-build

# Paths
FRONTEND_DIR = frontend
BACKEND_DIR = backend
BUILD_DIR = build

# Backend variables
BINARY_NAME = message-router
MAIN_FILE = main.go

# Default target
all: build-all

# Build frontend
build-frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm install --force && npm run build

# Build backend with embedded frontend
build-backend:
	@echo "Building backend..."
	mkdir -p $(BUILD_DIR)
	rm -rf $(BACKEND_DIR)/dist
	cp -r $(FRONTEND_DIR)/dist $(BACKEND_DIR)/dist
	cd $(BACKEND_DIR) && GOPROXY=$(GOPROXY) go build -o ../$(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# Build both
build-all: build-frontend build-backend
	@echo "Build complete! Binary located at $(BUILD_DIR)/$(BINARY_NAME)"

# Build tools
gen-session:
	@echo "Running session generator..."
	cd $(BACKEND_DIR) && go run cmd/gen-session/main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(BUILD_DIR)

# Run backend in dev mode
dev-backend:
	cd $(BACKEND_DIR) && go run $(MAIN_FILE)

# Run frontend in dev mode
dev-frontend:
	cd $(FRONTEND_DIR) && npm run dev

# Docker build
GOPROXY ?= https://goproxy.io,direct
NPM_REGISTRY ?= https://registry.npmmirror.com
docker-build:
	docker build --build-arg GOPROXY=$(GOPROXY) --build-arg NPM_REGISTRY=$(NPM_REGISTRY) -t message-router:latest .
