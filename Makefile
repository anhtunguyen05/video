SHELL := /bin/sh

COMPOSE_FILE ?= deploy/compose/docker-compose.yml
COMPOSE := docker compose -f $(COMPOSE_FILE)

.PHONY: help test test-unit test-integration lint fmt build dev-infra-up dev-infra-down \
	api worker web migrate-up migrate-down

help:
	@printf '%s\n' \
		'make test              Run available tests' \
		'make lint              Run available linters' \
		'make fmt               Format Go and frontend sources' \
		'make build             Build available applications' \
		'make dev-infra-up      Start local PostgreSQL, RabbitMQ and MinIO' \
		'make dev-infra-down    Stop local infrastructure'

test: test-unit test-integration

test-unit:
	@set -e; \
	if [ -f services/api/go.mod ]; then (cd services/api && go test ./...); else echo 'No API Go module yet; skipping API unit tests.'; fi; \
	if [ -f services/worker/go.mod ]; then (cd services/worker && go test ./...); else echo 'No worker Go module yet; skipping worker unit tests.'; fi; \
	if [ -f apps/web/package.json ]; then (cd apps/web && npm test -- --passWithNoTests); else echo 'No web package yet; skipping web unit tests.'; fi

test-integration:
	@echo 'No integration tests yet; skipping.'

lint:
	@set -e; \
	if [ -f services/api/go.mod ]; then (cd services/api && go vet ./...); else echo 'No API Go module yet; skipping API lint.'; fi; \
	if [ -f services/worker/go.mod ]; then (cd services/worker && go vet ./...); else echo 'No worker Go module yet; skipping worker lint.'; fi; \
	if [ -f apps/web/package.json ]; then (cd apps/web && npm run lint); else echo 'No web package yet; skipping web lint.'; fi

fmt:
	@set -e; \
	if command -v gofmt >/dev/null 2>&1; then find services -name '*.go' -type f -exec gofmt -w {} +; fi; \
	if [ -f apps/web/package.json ]; then (cd apps/web && npm run format --if-present); fi

build:
	@set -e; \
	if [ -f services/api/go.mod ]; then (cd services/api && go build ./...); else echo 'No API Go module yet; skipping API build.'; fi; \
	if [ -f services/worker/go.mod ]; then (cd services/worker && go build ./...); else echo 'No worker Go module yet; skipping worker build.'; fi; \
	if [ -f apps/web/package.json ]; then (cd apps/web && npm run build); else echo 'No web package yet; skipping web build.'; fi

dev-infra-up:
	$(COMPOSE) up -d

dev-infra-down:
	$(COMPOSE) down

api:
	@test -f services/api/go.mod || (echo 'API is not implemented in PR 1.' && exit 1)
	cd services/api && go run ./cmd/api

worker:
	@test -f services/worker/go.mod || (echo 'Worker is not implemented in PR 1.' && exit 1)
	cd services/worker && go run ./cmd/worker

web:
	@test -f apps/web/package.json || (echo 'Web app is not implemented in PR 1.' && exit 1)
	cd apps/web && npm run dev

migrate-up:
	@echo 'Migration runner is not configured in PR 1; migrations are versioned in migrations/.'

migrate-down:
	@echo 'Migration runner is not configured in PR 1.'

