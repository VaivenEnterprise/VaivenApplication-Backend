# Guion de demostración — VAIVÉN backend

> **Borrador provisional (2026-11-09, spec 012).** Lo completa y cierra la tarea T010.
> **Pendiente de confirmar por el equipo:** presentador principal, duración total y si se muestra la app móvil en vivo.
> Este documento **nunca** lleva contraseñas: solo correos de las cuentas del `seed`. Las contraseñas viven en el archivo de `SEED_CREDENTIALS_FILE` de quien presenta, fuera del repositorio.

## Reparto provisional

| Bloque | Responsable provisional |
|---|---|
| 1. Presentación del sistema | Por confirmar (presentador principal) |
| 2. Arquitectura | BE-A |
| 3. Flujo funcional | Por confirmar |
| 4. Persistencia | BE-B |
| 5. Seguridad | BE-A |
| 6. Pruebas (incluye rendimiento) | BE-B |
| 7. Despliegue | BE-B |
| 8. Conclusiones | Por confirmar (presentador principal) |

## Antes de empezar (checklist)

- [ ] Staging responde: `/healthz` y `/readyz` en 200 por HTTPS.
- [ ] `go run ./cmd/admin check-services` en `OK` y `go run ./cmd/admin indexes` sin faltantes.
- [ ] `seed` ejecutado en staging (T009); el archivo de credenciales está a mano, fuera del repositorio.
- [ ] Swagger UI (`/docs`) abre, si está habilitado en staging.
- [ ] Resultados recientes a mano: CI en verde, cobertura y `docs/rendimiento.md`.

## 1. Presentación del sistema
- Qué es VAIVÉN: préstamo y renta de objetos entre personas en México (MXN).
- Problema que resuelve y a quién va dirigido.
- Qué se va a mostrar y en qué orden.

## 2. Arquitectura
- Clean Architecture: `domain → usecase → adapter → cmd`; la prueba `architecture_test.go` impide dependencias prohibidas.
- Módulos y puertos compartidos (`notify.Notifier`, `moderation.Checker`, `chat.Opener`, `images.Service`).
- API REST con contrato primero (`api/openapi.yaml`) y formato de error único (Art. IV).
- Infraestructura: VM de Azure con Docker Compose (API, MongoDB y Redis), HTTPS servido por la propia API.

## 3. Flujo funcional
Recorrido con las cuentas del `seed` (los correos se anotan aquí al cerrar T005):

| Paso | Cuenta | Acción |
|---|---|---|
| 1 | Persona A | Inicia sesión y publica un objeto con fotos |
| 2 | Persona B | Busca, cotiza y solicita la reserva |
| 3 | Persona A | Acepta la solicitud |
| 4 | A y B | Conversan por el chat de la reserva |
| 5 | A y B | Entrega con evidencia y QR; pago reportado y confirmado |
| 6 | A y B | Devolución con QR |
| 7 | A y B | Se califican |
| 8 | B y admin | B abre una incidencia y el admin la cierra |

## 4. Persistencia
- MongoDB como única fuente de verdad; Redis solo para límite de tasa y caché (Art. XIV).
- Índices declarados en código y verificados con `cmd/admin indexes`.
- Integridad y concurrencia: confirmaciones atómicas con filtro de estado y versión.

## 5. Seguridad
- Autenticación con JWT de acceso (HS256, 15 min) y refresh tokens opacos con rotación y revocación.
- Autorización de admin comprobada contra la base (spec 012, AUD-017).
- Límite de tasa en Redis, cabeceras de seguridad, CORS acotado y evidencia solo por URL firmada.
- CI con gitleaks y escáner de comprimidos; remediación de noviembre 2026 (`docs/seguridad/remediacion-2026-11.md`), contada sin mostrar valores.

## 6. Pruebas
- Unitarias, de integración con MongoDB real (Testcontainers) y E2E del recorrido completo (`make e2e`).
- Cobertura global (RNF-01, ≥ 75 %) y umbral por capa en CI.
- Rendimiento: p50, p95 y p99 de `GET /items` y `GET /me/bookings` (de `docs/rendimiento.md`).

## 7. Despliegue
- Pipeline: PR → CI (Policy, Quality, Secrets, CodeQL) → `develop` → `deploy-staging.yml`.
- Verificación por HTTPS (`/healthz`, `/readyz`) y pruebas de humo (`smoke.yml`).

## 8. Conclusiones
- Qué se logró frente a los objetivos.
- Limitaciones conocidas y siguientes pasos (v1.1: specs 014 y 015).
- Preguntas.
