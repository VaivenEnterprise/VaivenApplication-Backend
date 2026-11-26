# Registro de deuda técnica (spec 016, T004)

- **Fecha:** 2026-11-17 · **Fuente:** deuda conocida de la 016 (RF-04, a–g), `inventario.md` (T001), `conformidad.md` (T002) y `trazabilidad.md` (T003)
- **Gravedad:**
  - **P0:** bloquea la entrega;
  - **P1:** se corrige en esta spec;
  - **P2:** se documenta con dueño y justificación.
- **Estado:** abierta · corregida (con evidencia) · pendiente de decisión (fuera del alcance de quien implementa).
- **Revisión de BE-A:** **pendiente** (aceptación de T004).

| ID | Hallazgo | Spec / Art. | Gravedad | Dueño | Corrige | Estado |
|---|---|---|---|---|---|---|
| DT-a | `validator/v10` en la frontera falta en 10 módulos: `bookings`, `handover`, `payments` (BE-A) y `search`, `chat`, `ratings`, `incidents`, `sanctions`, `notifications`, `reports` (BE-B) | 006–011 / VI.2 | P1 | BE-A y BE-B | T005 y T006 | Ver la actualización al final |
| DT-b | Las rutas de la 009 no montan `RequireActive` | 009 / VI | **P2** | BE-A | — | **No se corrige.** Bloquear a una cuenta suspendida en la entrega y la devolución impediría **cerrar préstamos en curso** (devolver el objeto, confirmar el pago). La spec 009 no lo pide. Si el equipo decide lo contrario, se hace con una regla explícita (permitir devolución y confirmación) |
| DT-c | El buscador del asistente filtra por la categoría que propone el modelo; si se equivoca, no encuentra el objeto | 014 / — | **P2** | BE-A | — | **Diferida por decisión del responsable** («mantenlo así», 2026-11-17). Propuesta: si no hay resultados con categoría, buscar sin ella |
| DT-d | Estados de spec desactualizados: 009, 010 y 011 en «Borrador» estando desplegadas; 013-T008 con evidencia sin marcar | 009–011, 013 / I, X.5 | P1 | BE-B (010, 011, 013) y BE-A (009) | T007 | Ver la actualización al final |
| DT-d2 | Visto bueno de BE-A pendiente en 003 (ajustes del plan §9), 005 y 007 | 003, 005, 007 / XII.1 | P1 | **BE-A** | BE-A | Pendiente de decisión |
| DT-e | POL-06 (QR de 15 min) y POL-07 (tolerancia de 2 h) siguen provisionales | 009 / — | P1 | **Equipo** | Equipo | Pendiente de decisión |
| DT-f | Staging con muy pocos objetos activos para demostrar | 012, 014, 015 / — | P1 | BE-B | T012 | Necesita correr `seed` en staging (Cloud Shell) |
| DT-g | Foto de prueba de la 014 huérfana en el Cloudinary de staging | 014 / VI | P1 | BE-B | Responsable | Pendiente: se borra a mano en Cloudinary (requiere la sesión del responsable) |
| DT-h | Desarrollo con Claude Code en lugar de OpenCode con modelos gratuitos | Varias / X | P2 | Equipo | — | Desviación aceptada por el responsable y registrada en `memoria.md`; se documenta en el informe final |
| DT-i | Implementación antes de aprobar la spec (006, 007, 008, 009, 010, 011 y 002-T001/T002) | Varias / I.1 | P2 | Equipo | — | Desviación registrada; las specs quedaron aprobadas después. Se documenta |
| DT-j | 002 sin «Dueño» en el encabezado; 017 sin encabezado, estado, RF/CA ni IDs de tarea | 002, 017 / I, XII.1 | P1 (002) · P2 (017) | BE-B | T007 (002) | 017: requiere reescribirla como spec formal; se propone en una spec aparte, fuera del alcance de la 016 |
| DT-k | La 012 está casi entera abierta: E2E (T001, T002), cabeceras y límite de cuerpo (T003), revisión de seguridad (T008), revisión final (T011) y despliegue (T012) | 012 / VI, VII, XIII.5 | **P0** | **BE-A** (T001–T003, T008, T011) y BE-B (T009, T010, T012) | 012 | Pendiente: son tareas de la 012 con su dueño; la 016 no las duplica (la 018 propone criterios complementarios) |
| DT-l | La prueba de claves de la 014 da un falso positivo en Windows (CRLF) | 014 / VII | P1 | BE-B (avisar a BE-A) | T007 | Ver la actualización al final |
| DT-m | 131 de 287 RF, RNF y CA no tienen una prueba que los cite por ID | 003–015 / VII | P2 | Dueño de cada spec | — | Muchos se verifican por despliegue, documentación o con pruebas que no citan el ID. Se mejora al tocar cada módulo, citando el ID en la prueba |
| DT-n | Faltan pruebas de contrato (respuestas reales contra `openapi.yaml`) | Todas / IV, VII | P1 | BE-B | T008 | Ver la actualización al final |
| DT-o | Mutación solo en 3 directorios | Todas / VII | P1 | BE-B | T009 | Ver la actualización al final |
| DT-p | Sin DAST, escaneo de imagen ni SBOM | Todas / VI | P1 | BE-B | T010 y T011 | Ver la actualización al final |

## Actualización al final (2026-11-17, tras T005–T013)

| ID | Estado | Evidencia o siguiente paso |
|---|---|---|
| DT-a | **Corregida** | `validator/v10` en la frontera de `bookings`, `handover` y `payments` (BE-A: **desviación**, BE-B tocó módulos de BE-A a pedido del responsable; falta su visto bueno) y de `chat`, `ratings`, `incidents`, `sanctions`, `notifications` y `reports`. Helper común `internal/adapter/http/validate` (100 %) que conserva el contrato de error: mismo código, mismo mensaje del módulo y un detalle por campo con su nombre JSON. `search` no tiene cuerpo: sus parámetros los valida `domain.Parse`. Cambio de comportamiento menor: `notifications` y `reports` ahora informan **todos** los campos obligatorios que faltan, no solo el primero (sigue siendo 400 `validation_failed`) |
| DT-d | **Corregida en parte** | 009, 010 y 011 ya dicen «implementada y desplegada en staging»; el `tasks.md` de 009 y 011 decía «sin ejecutar». **013-T008 no se marca:** su aceptación (ajustada el 2026-11-16) pide que **otra persona del equipo** ejecute la reversión en staging, y eso no ha pasado. La observación del T002 («con evidencia sin marcar») era imprecisa |
| DT-j | **Corregida (002)** | «Dueño» en el encabezado de la 002. La 017 sigue pendiente (P2) |
| DT-l | **Corregida** | La prueba usa una cadena interpretada con tabulador y retorno de carro reales: en una expresión POSIX, «	» dentro de corchetes son dos caracteres. Pasa en Windows y sigue fallando con una clave de prueba |
| DT-n | **Corregida** | Paquete `internal/adapter/http/contract` con `libopenapi-validator` v0.16.2, solo en pruebas (no está en el binario). Envuelve las 14 pruebas de integración que levantan la API: 69 de 69 operaciones y 0 incumplimientos. Para cubrir 4 operaciones se agregaron llamadas a pruebas existentes: cambiar el punto de encuentro, rechazar, `PATCH /me` y `GET /categories` |
| DT-o | **Corregida en código; pendiente de corrida** | `scripts/mutation.sh` recorre por omisión los 32 `domain` y `usecase` de módulo, y el flujo tiene 180 minutos de límite. Falta lanzar *Actions → Mutation* y pasar aquí los paquetes bajo el umbral |
| DT-p | **Corregida en código; pendiente de corrida** | `zap.yml`: solo manual y solo staging, con confirmación. Job `image` en la CI: Trivy v0.70.0 con `trivy-action` v0.36.0 fijada por SHA, falla con críticas o altas que tengan corrección, sube el SBOM CycloneDX y es requisito de «Required checks». Falta la primera corrida de cada uno; ZAP necesita autorización |
| DT-f | **Bloqueada** | `seed` exige un archivo de credenciales que `vm-admin.sh` no monta. Opciones en `docs/calidad/demo.md` §2; se recomienda la A, una tarea de `devops-mcp` |
| DT-b, DT-c, DT-d2, DT-e, DT-g, DT-h, DT-i, DT-k, DT-m | Sin cambio | Ver la tabla de arriba |
