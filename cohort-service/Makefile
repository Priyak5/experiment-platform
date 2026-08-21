.PHONY: build test test-short lint up down run tidy fmt

GO ?= go

build:
	$(GO) build ./...

test:
	$(GO) test ./...

test-short:
	$(GO) test -short ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; running go vet only"; \
		$(GO) vet ./...; \
	fi

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

DC ?= docker-compose

up:
	$(DC) up -d
	@echo "waiting for services to be healthy..."
	@for i in $$(seq 1 30); do \
		if $(DC) ps --format json 2>/dev/null | grep -q '"Health":"healthy"'; then break; fi; \
		sleep 1; \
	done
	@$(DC) ps

down:
	$(DC) down -v

run:
	$(GO) run ./cmd/server
