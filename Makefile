.PHONY: help build test dev stop clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build worker binary
	go build -o bin/worker ./cmd/worker

test: ## Run tests
	go test -v ./...

dev: build ## Start dev environment and run worker
	@./scripts/dev.sh

inspect: ## Inspect compression results
	@./scripts/inspect.sh

stop: ## Stop dev environment
	cd dev && docker-compose down

clean: stop ## Stop services and clean build artifacts
	rm -rf bin/
	cd dev && docker-compose down -v

logs: ## Show dev stack logs
	cd dev && docker-compose logs -f
