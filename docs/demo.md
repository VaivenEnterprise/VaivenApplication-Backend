# Guion de demostración — VAIVÉN backend

> **Versión de T010 (2026-11-11).** Falta solo lo que debe decidir el equipo (abajo) y probarlo con una persona ajena al backend (CA-04).
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

### Datos que deja el `seed` (T005)

| Cuenta | Rol en la demo | Lo que ya tiene |
|---|---|---|
| `ana.demo@vaiven.example` | Dueña | Taladro, escalera y juego de desarmadores (Guadalajara); el taladro tiene una solicitud **pendiente** de Beto |
| `beto.demo@vaiven.example` | Dueño y solicitante | Bicicleta, tienda de campaña y podadora (Zapopan); su bicicleta tiene una reserva **aceptada** de Carla |
| `carla.demo@vaiven.example` | Solicitante | Reserva aceptada de la bicicleta de Beto (entra en 14 días); además publica la hidrolavadora, la mesa y la sierra |
| `diego.demo@vaiven.example` | Solicitante | Solicitud **cancelada** de los desarmadores de Ana; publica los patines y la carretilla |
| `elena.demo@vaiven.example`, `fer.demo@vaiven.example` | Dueñas | Licuadora y scooter (Elena); caja de herramientas y proyector (Fer) |
| `admin.demo@vaiven.example` | Administración | Cola de incidencias y reportes |

Las contraseñas **no** están aquí: están en el archivo de `SEED_CREDENTIALS_FILE` de quien presenta. Para no gastar tiempo, conviene tener las sesiones abiertas antes de empezar.

### Recorrido

| Paso | Cuenta | Acción | Qué mostrar |
|---|---|---|---|
| 1 | Ana | Inicia sesión y publica un objeto nuevo con foto | Firma de subida, validación de la foto y moderación del texto |
| 2 | Diego | Busca el objeto (por texto o categoría), cotiza y lo solicita | Búsqueda que nunca muestra los objetos propios; cotización en centavos de MXN |
| 3 | Ana | Acepta la solicitud | Cambio de estado y aviso a Diego |
| 4 | Ana y Diego | Conversan por el chat de la reserva | Conversación abierta automáticamente al solicitar |
| 5 | Ana y Diego | Entrega: evidencia, QR de un solo uso y confirmación; pago reportado y confirmado | QR de 15 min (POL-06); el pago se registra, no se cobra |
| 6 | Diego y Ana | Devolución con evidencia y QR | Detección de retraso (POL-07) |
| 7 | Ana y Diego | Se califican | Promedio en el perfil público |
| 8 | Diego y admin | Diego abre una incidencia y la administración la cierra | Rutas `/admin/*` protegidas por rol |

**Si falta tiempo:** se puede saltar de 3 a 7 usando la reserva aceptada que ya trae el seed (Carla y Beto) para mostrar el chat, y dejar la entrega y la devolución para la sección de pruebas (el recorrido E2E automatizado cubre los 8 pasos).

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
- Rendimiento (de `docs/rendimiento.md`): con 1515 objetos, p95 de 13,2 ms en la búsqueda y 6,7 ms en `GET /me/bookings` (umbral de 500 ms); las 10 consultas más frecuentes usan índice.
- Índices: `go run ./cmd/admin indexes` compara los 34 índices declarados con los reales, solo con permiso de lectura.

## 7. Despliegue
- Pipeline: PR → CI (Policy, Quality, Secrets, CodeQL) → `develop` → `deploy-staging.yml`.
- Verificación por HTTPS (`/healthz`, `/readyz`) y pruebas de humo (`smoke.yml`).

## 8. Conclusiones
- Qué se logró frente a los objetivos.
- Limitaciones conocidas y siguientes pasos (v1.1: specs 014 y 015).
- Preguntas.
