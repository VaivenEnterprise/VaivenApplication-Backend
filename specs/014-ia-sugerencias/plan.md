# Plan 014 — IA generativa: sugerencias y asistente de proyectos (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A · Parte B agregada el 2026-11-16 (pendiente de aprobación de BE-A)

## 1. Archivos
```
api/paths/ai.yaml                                                  (A y B)
internal/ai/domain/{schema.go, schema_test.go}                     (validación de la salida del modelo; capa domain, sin infraestructura)
internal/ai/adapter/gemini/{client.go, client_test.go}             (HTTP a Gemini: visión y JSON con esquema)
internal/ai/adapter/ollama/{client.go, client_test.go}             (solo desarrollo y demostración local)
internal/ai/adapter/mongo/usage.go (+ test)                        (contador diario común, TTL)
internal/suggest/usecase/{suggest.go, suggest_test.go}             (parte A; capa usecase vigilada por la prueba de arquitectura)
internal/suggest/adapter/http/{handler.go, dto.go, routes.go}
internal/assistant/domain/{prompt.go, prompt_test.go}              (parte B: instrucciones fijas, bloque de datos, historial, clave de caché)
internal/assistant/usecase/{assistant.go, assistant_test.go}        (parte B: caso de uso y puertos)
internal/assistant/adapter/mongo/conversations.go (+ test)         (ai_conversations, TTL 24 h)
internal/assistant/adapter/search/finder.go (+ test)               (usa el caso de uso de búsqueda de la 006)
internal/assistant/adapter/http/{handler.go, dto.go, routes.go}
cmd/api/modules.go (dos líneas) · config (AI_ENABLED, AI_PROVIDER, AI_API_KEY, AI_BASE_URL, AI_DAILY_LIMIT,
  ASSISTANT_ENABLED, ASSISTANT_DAILY_LIMIT, ASSISTANT_RADIUS_KM)
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | **Cada caso de uso declara su puerto** (Art. III.3): `suggest.Describer` y `assistant.Planner`, ambos con el método `Generate(ctx, aidomain.Request) ([]byte, error)` sobre tipos de `internal/ai/domain` (petición, turnos y esquema de salida). El adaptador `internal/ai/adapter/gemini` implementa `Generate` una sola vez; Ollama (opcional, solo local) igual. Los casos de uso nunca importan el adaptador | III |
| P-02 | Parte A: entrada solo `publicId`; la URL de la miniatura se construye en el servidor | Seguridad |
| P-03 | Salida del modelo en JSON con esquema, validada en `internal/ai/schema.go`: categoría ∈ POL-14, longitudes, conteos; campos extra descartados | VI |
| P-04 | Contador común `ai_usage` `{userId, feature, day, count, createdAt, updatedAt}` con índice único y TTL de 3 días (`feature` = `suggest` o `assistant`) | §6 |
| P-11 | Idioma: las instrucciones fijan español de México y la validación de salida rechaza campos vacíos o fuera de esquema | V.4 |
| P-05 | Tiempo de espera de 10 s (A) y 12 s (B), un solo reintento; sin cola ni trabajos en segundo plano | — |
| P-06 | Sin SDK del proveedor: `net/http` y `encoding/json` | II |
| P-07 | **Separación modelo/datos:** el modelo solo devuelve `needs`, `steps` y `safetyNotes`; las coincidencias salen de `assistant.Finder`, un puerto implementado con el caso de uso de búsqueda de la 006, que ya aplica los filtros de visibilidad y el redondeo de ubicación. El modelo nunca ve objetos, personas ni ubicaciones | VI, privacidad |
| P-08 | Instrucciones del sistema fijas en `prompt.go`. El mensaje de la persona y los turnos previos van en un bloque de datos delimitado, y el esquema de salida no tiene campos de texto libre fuera de `summary`, `steps` y `safetyNotes`, que pasan por `moderation.Checker` | VI |
| P-09 | Conversación en MongoDB (`ai_conversations`, últimos 6 turnos, TTL 24 h): es dato de la persona, no caché. Redis solo guarda la caché de la respuesta del modelo al primer mensaje normalizado (TTL corto de 1 h, clave `vaiven:<entorno>:ai:plan:<hash>`) y el límite por minuto; si Redis cae, se llama al modelo | XIV |
| P-10 | Búsquedas de coincidencias en paralelo con un máximo de 4 simultáneas y su propio tiempo de espera, para cumplir RNF-01 | — |

## 3. Pruebas
- Unitarias con `Describer`, `Planner` y `Finder` falsos: esquema, recorte a 8 necesidades, negativa fuera de alcance, inyección de instrucciones, la ubicación no llega al proveedor, caché.
- Adaptadores Gemini y Ollama contra `httptest.Server`.
- Repositorio de conversaciones con MongoDB real (Testcontainers): TTL, dueño y máximo de turnos.
- Integración de los dos endpoints con proveedor falso y datos de búsqueda reales: límites, imagen ajena, caída, filtro y CA-07 a CA-14.

## 4. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Sin dependencias nuevas |
| IV | Errores con el envoltorio estándar; contrato primero (`ai.yaml`) |
| VI | Clave solo en variable de entorno; sin datos personales en registros ni hacia el modelo |
| XII | Módulos `suggest` y `assistant`, una línea cada uno en `modules.go`; usa los puertos compartidos `moderation.Checker`, `images.Service`, `cache.Cache` y `ratelimit.Limiter` |
| III | Puertos declarados por cada caso de uso; `domain`/`usecase` sin `net/http`, `bson` ni `mongo` |
| V | Importes en centavos (tarjeta de la 006); textos en es-MX |
| VII | Pruebas en cada tarea con lógica; ≥ 80 % en `domain` y `usecase`; integración con testcontainers |
| VIII | Sin dependencias nuevas ni SDK; Ollama opcional y solo local |
| XIII.1 | Costo cero: Gemini en capa gratuita; Ollama no corre en la VM |
| XIV | Redis solo para caché y límite de tasa; la conversación vive en MongoDB |
