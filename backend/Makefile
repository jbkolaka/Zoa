# Zoa backend — common tasks.
.PHONY: help run build migrate inspect test vet fmt tidy clean docker-build docker-run

BINARY := bin/zoa-api
DB     ?= app.db

help: ## Show this help
	@grep -E '^[a-z-]+:.*?## .' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

run: ## Run the API server (applies migrations on boot)
	go run ./cmd/api

build: ## Compile the API binary
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/api
	@echo "built $(BINARY)"

migrate: ## Apply migrations and exit
	go run ./cmd/api -migrate-only

inspect: ## Print schema and row counts (dev utility; no sqlite3 CLI needed)
	go run ./cmd/dbinspect

test: ## Run tests
	go test ./... -count=1

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go source
	gofmt -w .

tidy: ## Tidy module dependencies
	go mod tidy

clean: ## Remove build output and the local database
	rm -rf bin $(DB) $(DB)-wal $(DB)-shm

docker-build: ## Build the container image
	docker build -t zoa-api:local .

docker-run: ## Run the container with a persistent volume
	docker run --rm -p 8080:8080 -v zoa-data:/data zoa-api:local
