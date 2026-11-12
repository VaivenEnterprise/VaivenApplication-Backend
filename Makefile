# Makefile — Backend VAIVEN (spec 002, T002)
# En Windows usar Git Bash o WSL; sin make, ejecutar el comando de cada objetivo.

APP_VERSION ?= dev
BIN_DIR     := bin
BIN         := $(BIN_DIR)/api$(shell go env GOEXE)
LDFLAGS     := -s -w -X main.version=$(APP_VERSION)
# -race necesita cgo (un compilador de C). Sin él (p. ej. Windows): make test RACE=
RACE        ?= -race

.PHONY: run up down test test-integration e2e lint lint-api fmt build docs-bundle

run: ## Levanta la API (requiere MongoDB en marcha)
	go run ./cmd/api

up: ## Levanta MongoDB (replica set) con Docker Compose
	docker compose up -d

down: ## Detiene los servicios de Docker Compose
	docker compose down

test: ## Pruebas unitarias con detector de carreras y cobertura
	go test ./... $(RACE) -cover

test-integration: ## Pruebas con MongoDB real (Testcontainers; requiere Docker o Podman)
	go test -tags=integration ./... $(RACE)

# Recorrido de punta a punta (spec 012, RF-01, T001/T002). Corre SOLO en local:
# el router real en proceso, MongoDB con Testcontainers y Cloudinary falso.
# Nunca apunta a staging (crea y modifica datos); staging se verifica con
# smoke.yml y la tarea de despliegue.
E2E_DIR := test/e2e

e2e: ## Recorrido E2E en local (Testcontainers; requiere Docker o Podman); nunca contra staging
	@test -d $(E2E_DIR) || { echo "make e2e: falta $(E2E_DIR) (lo crean T001 y T002 de la spec 012)"; exit 1; }
	go test -tags=e2e -count=1 ./$(E2E_DIR)/... $(RACE)

lint: ## Analisis estatico con golangci-lint
	golangci-lint run

lint-api: ## Valida api/openapi.yaml con Redocly; termina con error si el contrato tiene errores
	$(REDOCLY) lint api/openapi.yaml

fmt: ## Formatea el codigo
	gofmt -s -w .

build: ## Compila bin/api inyectando APP_VERSION
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/api

# Versión fija de Redocly; la misma que usa el CI para detectar desfases.
REDOCLY := npx --yes @redocly/cli@1.34.5

docs-bundle: ## Empaqueta api/ en internal/adapter/http/docs/openapi.yaml (Swagger UI en /docs)
	$(REDOCLY) bundle api/openapi.yaml -o internal/adapter/http/docs/openapi.yaml
