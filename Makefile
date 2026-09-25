# Windows exposes ComSpec even when OS is not forwarded into make's environment.
ifneq ($(ComSpec),)
SHELL := cmd.exe
.SHELLFLAGS := /D /E:ON /V:OFF /S /C
MIGRATE_UP := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/migrate.ps1 -Direction up
MIGRATE_DOWN := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/migrate.ps1 -Direction down
else
SHELL := /bin/sh
.SHELLFLAGS := -eu -c
MIGRATE_UP := ./scripts/migrate.sh up
MIGRATE_DOWN := ./scripts/migrate.sh down
endif

COMPOSE_FILE ?= compose.yaml
COMPOSE := docker compose -f $(COMPOSE_FILE)

.PHONY: help test test-unit test-integration lint fmt build dev-infra-up dev-infra-down \
	api worker web migrate-up migrate-down

help:
	@echo make test              Run available tests
	@echo make lint              Run available linters
	@echo make fmt               Format Go and frontend sources
	@echo make build             Build available applications
	@echo make dev-infra-up      Start local PostgreSQL, RabbitMQ and MinIO
	@echo make dev-infra-down    Stop local infrastructure
	@echo make migrate-up        Apply PostgreSQL migrations
	@echo make migrate-down      Roll back PostgreSQL migrations

test: test-unit test-integration

test-unit:
	cd services/api && go test ./...
	cd services/worker && go test ./...
	cd apps/web && npm test

test-integration:
	@echo 'No integration tests yet; skipping.'

lint:
	cd services/api && go vet ./...
	cd services/worker && go vet ./...
	cd apps/web && npm run lint

ifneq ($(ComSpec),)
fmt:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-ChildItem services/api,services/worker -Filter *.go -Recurse | ForEach-Object { gofmt -w $$_.FullName }"
	cd apps/web && npm run format --if-present
else
fmt:
	find services -name '*.go' -type f -exec gofmt -w {} +
	cd apps/web && npm run format --if-present
endif

build:
	cd services/api && go build ./...
	cd services/worker && go build ./...
	cd apps/web && npm run build

dev-infra-up:
	$(COMPOSE) up -d --wait --wait-timeout 60

dev-infra-down:
	$(COMPOSE) down

api:
	cd services/api && go run ./cmd/api

worker:
	cd services/worker && go run ./cmd/worker

web:
	cd apps/web && npm run dev

migrate-up:
	$(MIGRATE_UP)

migrate-down:
	$(MIGRATE_DOWN)

