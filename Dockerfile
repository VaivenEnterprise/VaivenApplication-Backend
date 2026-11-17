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

# Utilidades de administración (check-services, indexes, seed, create-admin;
# spec 012 T009). Viajan en la misma imagen porque MongoDB y Redis no publican
# puertos (Art. XIV.4) y la VM no tiene SSH: se corren en un contenedor de un
# solo uso con `--entrypoint /admin` en la red del entorno (docs/despliegue.md
# §3). La API no las usa; el ENTRYPOINT sigue siendo /api.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w" \
    -o /out/admin ./cmd/admin

# --- Etapa 2: ejecución -----------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=build /out/admin /admin
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
