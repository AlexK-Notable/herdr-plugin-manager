.DEFAULT_GOAL := help

.PHONY: help
help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build herdr-plugin-manager and bin/herdr-panel, as herdr does on install
	@go build -o herdr-plugin-manager .
	@go build -o bin/herdr-panel github.com/AlexK-Notable/herdr-panel/cmd/herdr-panel

.PHONY: check
check: build ## vet, test and validate the panel
	@go vet ./...
	@go test -race -count=1 ./...
	@bin/herdr-panel validate panel.toml
