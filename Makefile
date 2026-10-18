SHELL := /bin/sh

COMPOSE := docker compose -f deploy/docker-compose.yml
# Integration tests need a PostgreSQL where the user can create databases.
TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable

.PHONY: up down logs ps migrate db-reset test test-backend test-integration test-frontend lint build scan clean

## Start the stack (frontend, backend, PostgreSQL) on http://localhost:8080.
up:
	$(COMPOSE) up --build

## Stop containers. Data in the PostgreSQL volume is kept.
down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

## Apply pending database migrations (also applied at backend startup by default).
migrate:
	$(COMPOSE) run --rm backend migrate

## DESTRUCTIVE: stop the stack and delete the local database volume.
db-reset:
	@printf 'This deletes all local status page data. Type "yes" to continue: '; \
	read answer; [ "$$answer" = "yes" ] || { echo "Aborted."; exit 1; }
	$(COMPOSE) down --volumes

test: test-backend test-frontend

## Unit tests and contract checks; PostgreSQL integration tests are skipped.
test-backend:
	cd apps/backend && go test ./...

## Full backend suite against PostgreSQL (fails instead of skipping).
test-integration:
	cd apps/backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" REQUIRE_DB_TESTS=1 go test -race ./...

test-frontend:
	cd apps/frontend && npm test -- --run

lint:
	cd apps/backend && test -z "$$(gofmt -l .)" && go vet ./...
	cd apps/frontend && npm run lint

build:
	cd apps/backend && go build ./...
	cd apps/frontend && npm run build

scan:
	@if command -v trivy >/dev/null 2>&1; then \
		trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --ignore-unfixed .; \
	else \
		echo "Trivy não está instalado: https://trivy.dev/latest/getting-started/installation/"; \
		exit 1; \
	fi

## Remove containers and build outputs. Does not delete the database volume.
clean:
	$(COMPOSE) down --remove-orphans
	rm -rf apps/frontend/dist apps/frontend/coverage apps/backend/coverage.out
