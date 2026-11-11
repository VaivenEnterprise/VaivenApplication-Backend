# =============================================================================
# Dockerfile — imagen de la API de VAIVÉN (spec 002, T019; plan P-09; Art. VI)
# Multietapa: compila un binario estático con Go y lo copia a distroless,
# sin shell ni gestor de paquetes, ejecutándose como usuario no root.
#
#   docker build --build-arg APP_VERSION=$(git rev-parse --short HEAD) -t vaiven-api .
#
# La arquitectura sale de TARGETOS/TARGETARCH (amd64 en la VM B2ats_v2;
# arm64 si se usara B2pts_v2, Art. XIII.1).
# =============================================================================

# Debe coincidir con la línea `toolchain` de go.mod.
ARG GO_VERSION=1.26.11

# --- Etapa 1: compilación ---------------------------------------------------
FROM golang:${GO_VERSION} AS build
WORKDIR /src

# Dependencias primero, para aprovechar la caché de capas.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG APP_VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X main.version=${APP_VERSION}" \
    -o /out/api ./cmd/api

# --- Etapa 2: ejecución -----------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
