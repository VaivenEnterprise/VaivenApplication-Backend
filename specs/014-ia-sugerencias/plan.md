# Plan 014 — Sugerencias con IA (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A

## 1. Archivos
```
api/paths/ai.yaml
internal/suggest/{domain.go, usecase.go, usecase_test.go}        (puerto Suggester, reglas)
internal/suggest/adapter/provider/{client.go, client_test.go}    (HTTP al proveedor)
internal/suggest/adapter/mongo/usage.go                          (contador diario con TTL)
internal/suggest/adapter/http/{handler.go, dto.go, routes.go}
cmd/api/modules.go (una línea) · config (AI_ENABLED, AI_PROVIDER, AI_API_KEY, AI_DAILY_LIMIT)
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Interfaz `Suggester` en el dominio; el proveedor es un adaptador reemplazable | Arquitectura limpia |
| P-02 | Entrada solo `publicId`; la URL de la miniatura se construye en el servidor | Seguridad |
| P-03 | Respuesta del modelo en JSON estructurado y validada (categoría ∈ POL-14, título ≤ 80, filtro de contenido) | VI |
| P-04 | Contador en colección `ai_usage` `{userId, day, count}` con índice único y TTL de 3 días | Modelo Mongo |
| P-05 | Tiempo de espera de 10 s y un solo reintento; sin cola ni trabajos en segundo plano | — |
| P-06 | Sin SDK del proveedor: `net/http` y `encoding/json` | II |

## 3. Pruebas
Unitarias con un `Suggester` falso; adaptador contra `httptest.Server`; integración del endpoint con proveedor falso (límite, imagen ajena, caída, filtro).

## 4. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Sin dependencias nuevas |
| VI | Clave solo en variable de entorno; sin datos personales en registros |
| XIII | Módulo nuevo `suggest`; una línea en `modules.go` |
