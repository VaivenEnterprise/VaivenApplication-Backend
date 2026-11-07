# Makefile — Backend VAIVEN (spec 002, T002)
# En Windows usar Git Bash o WSL; sin make, ejecutar el comando de cada objetivo.

APP_VERSION ?= dev
BIN_DIR     := bin
BIN         := $(BIN_DIR)/api$(shell go env GOEXE)
LDFLAGS     := -s -w -X main.version=$(APP_VERSION)
# -race necesita cgo (un compilador de C). Sin él (p. ej. Windows): make test RACE=
RACE        ?= -race

.PHONY: run up down test test-integration lint fmt build

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

lint: ## Analisis estatico con golangci-lint
	golangci-lint run

fmt: ## Formatea el codigo
	gofmt -s -w .

build: ## Compila bin/api inyectando APP_VERSION
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/api
