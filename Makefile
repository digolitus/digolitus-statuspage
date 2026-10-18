SHELL := /bin/sh

COMPOSE_FILE := deploy/docker-compose.yml

.PHONY: up down logs ps test lint build scan clean

up:
    docker compose -f $(COMPOSE_FILE) up --build

down:
    docker compose -f $(COMPOSE_FILE) down

logs:
    docker compose -f $(COMPOSE_FILE) logs -f

ps:
    docker compose -f $(COMPOSE_FILE) ps

test:
    cd apps/backend && go test ./...
    cd apps/frontend && npm test -- --run

lint:
    cd apps/backend && go vet ./...
    cd apps/frontend && npm run lint

build:
    cd apps/backend && go build ./...
    cd apps/frontend && npm run build

scan:
    @if command -v trivy >/dev/null 2>&1; then \
        trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL .; \
    else \
        echo "Trivy não está instalado. Consulte a documentação para instalar: https://aquasecurity.github.io/trivy/"; \
        exit 1; \
    fi

clean:
    docker compose -f $(COMPOSE_FILE) down --remove-orphans
    rm -rf apps/frontend/dist apps/frontend/coverage
    rm -f apps/backend/coverage.out
