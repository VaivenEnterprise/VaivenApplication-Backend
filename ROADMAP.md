# ROADMAP — VAIVÉN (backend + móvil)

> **Nota (2026-11-01):** este plan se escribió cuando VAIVÉN era un monorepo. Ahora son tres repositorios: `backend/` → `vaiven-backend`, `mobile/` → `vaiven-frontend`, `landing/` → `vaiven-landing-page`. Donde el texto diga una de esas carpetas, léase el repositorio correspondiente.

> **Nota (2026-11-01, segunda reorganización):** (1) se eliminó todo lo relativo a repositorio, ramas, PR y pipeline CI/CD (y con ello la antigua spec 016 de pruebas/DevSecOps): esa parte se integrará de cero; (2) el trabajo va **por OpenCode y servidores MCP**: la spec **001 es la de servicios/infraestructura por MCP** (el 2026-11-02 el backend pasó a Azure, ver la nota siguiente) y la arquitectura base pasa a ser la **002**; (3) **cada spec termina desplegada**: backend en Azure y landing en Render (tarea final «Despliegue») y la app móvil verificada **en local con Expo Go** contra staging (tarea final «Verificación en Expo Go»); la spec siguiente no empieza hasta que esa tarea pase. La app móvil no se despliega salvo la publicación en Google Play (spec 013).

> **Nota (2026-11-02, Azure):** lo **obligatorio** es usar **Azure** (o AWS; se eligió Azure) y **Redis**. El backend (staging y producción) corre en **una VM gratuita de Azure** con Docker Compose: la API en Go (una réplica) con **MongoDB** (replica set de un nodo) y **Redis** (límite de tasa y caché). **HTTPS es forzoso** y lo sirve la propia API con un certificado de Let's Encrypt emitido por `certbot`; **no hay nginx ni balanceador** (decisión del 2 de nov: la VM tiene ~1 GiB de RAM y la app publicada exige HTTPS, no balanceo). Atlas y Render dejan de usarse para el backend; **siguen igual** Cloudinary y la landing en Render. **Principio de trabajo (indicado por el profesor):** la persona solo se registra y hace configuración mínima; **todo lo demás lo ejecuta OpenCode con los MCP** (Azure, Cloudinary, Render, Expo), y `az`/SSH solo para lo que un MCP no cubra. El despliegue no depende del repositorio: la imagen se construye en la máquina de quien despliega y viaja a la VM por SSH. **Costos:** una sola excepción aceptada, la IP pública estándar (≈ 3–4 USD/mes). Constitución del backend v1.4.0 (Art. XIII reescrito y Art. XIV nuevo).

> Plan maestro: backend y móvil specs `001` a `013`, y el proyecto `landing/` (L001–L002). Si este archivo contradice una constitución, gana la constitución.
> Última actualización: 2026-11-02 · Constituciones backend v1.4.0, móvil v1.2.1, landing v1.1.0 · Decisiones de producto confirmadas (§8)

## 1. Fechas

> **Calendario vigente (fijado el 2 de nov, día de arranque del desarrollo).** Sustituye al plan original del 28 de oct. El desarrollo empieza **hoy, lunes 2 de noviembre**. Todo depende del día 1 de la prueba cerrada: cada día que se retrase el primer AAB mueve todo lo demás un día.

| Hito | Fecha |
|---|---|
| **Arranque del desarrollo** (spec 001) | **Lunes 2 de noviembre** |
| H0 Base: 001 (Azure y servicios) y 002 (arquitectura base, primer despliegue) | 5 nov |
| Landing legal publicada (L001: privacidad, términos y eliminación de cuenta; las URL que pide la ficha de Play) | 5 nov |
| Backend de producción mínimo (backend 013, fase 1) | 6 – 8 nov |
| Testers reunidos (≥ 15 personas) y ficha con textos preliminares | antes del 9 nov |
| **Primer AAB** (registro e inicio de sesión: 001–003) a la prueba cerrada → **día 1 de 14** | **Lunes 9 de noviembre** |
| Specs 004–011, una por una con su cierre (Despliegue / Expo Go); cada hito sube una actualización a la prueba cerrada | 5 – 19 nov |
| 012 (integración, endurecimiento y AAB final 1.0, sin IA) y backend 013 fase 2 | 19 – 22 nov |
| **Día 14 de la prueba cerrada** | **Domingo 22 de noviembre** |
| **Código congelado de la 1.0 (22 nov) y solicitud de acceso a producción** | **Lunes 23 de noviembre** |
| Revisión de Google (la controla Google; suele tardar ~7 días o más). Mientras tanto: correcciones, sin funcionalidad nueva en la 1.0, y desarrollo de la IA y el ML (specs 014 y 015) | 23 nov – 1 dic |
| **Publicación de la 1.0 en Google Play (producción)** | **estimada 30 nov – 7 dic** (no antes del 28 nov aunque Google responda en 5 días) |
| IA y ML (v1.1) a la prueba cerrada, detrás de interruptores | ~2 dic |
| Documentación final, demostración y colchón | 1 – 3 dic |
| **ENTREGA OFICIAL: todos los specs completos (001–015 y landing)** | **1 – 4 de diciembre** |

> **Riesgo alto:** la 1.0 podría no estar aún en producción el 4 de dic (la revisión de Google no la controla el equipo). En ese caso se entrega con la prueba cerrada, el APK de demostración y el staging funcionando (ver línea de corte, §6). El calendario incluye fines de semana (7–8, 14–15 y 21–22 de nov); es ajustado.

### Ruta a Google Play (la fecha límite depende de esto)

Según las guías consultadas (ver §11), una cuenta **personal** de Google Play creada después de nov-2023 debe correr una **prueba cerrada con ≥ 12 testers inscritos 14 días seguidos** antes de poder pedir producción; después Google revisa la solicitud (puede tardar ~7 días o más). La cuenta ya está comprada (pago único de $25 USD, confirmado el 2 de nov). **Cada día que se retrase la verificación de la cuenta, la lista de testers o el primer AAB mueve la publicación un día.**

| Fecha | Qué ocurre | Quién |
|---|---|---|
| 2 – 3 nov | Confirmar la verificación de identidad de la cuenta de Google Play y reunir **15–20 testers** (cuentas de Google/Gmail) que acepten quedarse inscritos hasta ~el 30 de nov | Administrador |
| 2 – 5 nov | Landing L001 publicada (privacidad, términos y eliminación de cuenta) | MO-B |
| 6 – 8 nov | Backend de producción creado (backend 013 fase 1); ficha con textos preliminares y declaraciones de Play Console | BE-B / MO-B / Administrador |
| **9 nov** | Primer **AAB** (001–003) en la prueba cerrada; testers aceptan la invitación → **día 1 de 14** (si algo se retrasa, el día 1 se mueve igual) | Administrador / MO-B |
| 10 – 22 nov | Cada hito (H1–H5) se sube como actualización a la prueba cerrada; los testers no se salen. Día 14 = **22 nov** | MO-B |
| **23 nov** | Código congelado, AAB final (1.0, sin IA) y **solicitud de acceso a producción** (≥ 12 testers · 14 días) | Todos |
| 24 nov – 7 dic | Revisión de Google; publicación de la 1.0 cuando apruebe | Administrador |

> Si la cuenta ya existía antes del 13-nov-2023, este bloque no aplica; no es el caso.

## 2. Equipo y reparto

Cada persona trabaja con su propio OpenCode. Las parejas comparten spec de punta a punta (una hace el backend, la otra el móvil) y se coordinan por el contrato OpenAPI.

| Rol | Persona | Pareja | Dueña de las specs |
|---|---|---|---|
| **BE-A** (backend) | _(completar)_ | MO-A | 003, 005, 007, 009, 014 |
| **MO-A** (móvil) | _(completar)_ | BE-A | 003, 005, 007, 009 |
| **BE-B** (backend) | _(completar)_ | MO-B | 001, 004, 006, 008, 010, 011, 015 |
| **MO-B** (móvil) | _(completar)_ | BE-B | 001, 004, 006, 008, 010, 011, **013 móvil**, **landing** |

El **administrador del repositorio** (quien paga y administra la cuenta de Google Play) es responsable de la cuenta, los testers y las respuestas de Google; MO-B prepara los artefactos (AAB, ficha, landing) y BE-B el backend de producción (backend 013).

Spec **002** (arquitectura base) se reparte por tareas, ver §5; la **001** (infraestructura en Azure y servicios) la hacen BE-B y MO-B. Spec **012** es de todos. La landing es pequeña y estática: si MO-B está saturado, puede hacerla el administrador.

## 3. Specs y calendario

Prioridad: **P0** = sin esto no hay app · **P1** = esperado · **P2** = recortable.

| Spec | Nombre | Qué entrega | Dueñas | Prio. | Depende de | Ventana |
|---|---|---|---|---|---|---|
| 001 | **Infraestructura en Azure y servicios** | VM de Azure (talla gratuita elegida por región disponible) con MongoDB, Redis y certificado HTTPS; Cloudinary; todo por MCP de Azure + `az` desde OpenCode (móvil: Expo Go + MCP de Expo) | BE-B / MO-B | P0 | — | 2–3 nov |
| 002 | Arquitectura base | Esqueleto de backend y app; primer despliegue a staging (backend) y primera verificación en Expo Go (móvil) | repartida | P0 | 001 | 3–5 nov |
| 003 | Cuenta y autenticación | Registro, login, sesión, perfil, baja; concha de navegación | BE-A / MO-A | P0 | 002 | 5–7 nov |
| 004 | Imágenes | Firmas de subida, selector y subida de fotos | BE-B / MO-B | P0 | 001, 003 (auth) | 5–7 nov |
| 005 | Objetos | Publicar, editar, pausar, precio sugerido | BE-A / MO-A | P0 | 003, 004 | 8–10 nov |
| 006 | Búsqueda | Texto, categoría, zona, modalidad, fechas | BE-B / MO-B | P0 | 005 | 8–10 nov |
| 007 | Solicitudes y reservas | Cotización, solicitud, aceptar/rechazar, reserva, cancelar | BE-A / MO-A | P0 | 005 | 10–13 nov |
| 008 | Chat privado | Mensajes de texto por reserva | BE-B / MO-B | P1 | 007 | 10–13 nov |
| 009 | Entrega, devolución y pagos | Evidencia, QR, confirmaciones, pago registrado, retrasos | BE-A / MO-A | P0 | 004, 007 | 13–17 nov |
| 010 | Calificaciones e incidencias | Reputación, reportes con seguimiento manual, sanciones | BE-B / MO-B | P1 | 009 (parcial) | 13–17 nov |
| 011 | Notificaciones y moderación | Avisos en app y push (P2), filtro de contenido y reportes (**P1: exigidos por la política de contenido de usuarios de Google Play**) | BE-B / MO-B | P1 | 007–010 | 17–19 nov |
| 012 | Integración y entrega | Pruebas de punta a punta, endurecimiento, documentación, AAB final | todos | P0 | todas | 19–22 nov |
| 013 | Publicación en Google Play (móvil) y producción (backend) | Ficha, cumplimiento, prueba cerrada, producción; backend de producción | MO-B / BE-B + administrador | P0 | 002 (inicio), 012 (cierre) | 2 nov–7 dic |
| 014 | **IA (v1.1, después de la 1.0)** | Sugerir título y categoría a partir de una foto, siempre con revisión de quien publica (proveedor gratuito; interruptor `AI_ENABLED`) | BE-A / MO-A (+ MO-B cumplimiento) | P1 | 004, 005, 011, 013 | 23 nov–1 dic |
| 015 | **ML: recomendaciones (v1.1)** | «Recomendado para ti»: recomendador híbrido propio (colaborativo objeto-objeto + contenido), evaluado con Hit@10/Recall@10/MRR contra popularidad; interruptor `RECO_ENABLED` | BE-B / MO-B | P1 | 005, 006, 007, 013 | 23 nov–1 dic |
| L001 | Landing: base y páginas legales | Sitio estático, privacidad, términos, eliminación de cuenta, publicación gratuita | MO-B | P0 | — | 2–5 nov |
| L002 | Landing: página principal | Información de la app, cómo funciona, FAQ, botón a Google Play | MO-B | P1 | L001 | 9–16 nov |

**Regla de cierre (todas las specs desde la 002):** una spec está terminada solo cuando su última tarea pasa. Backend: «Despliegue» (`@devops-mcp` ejecuta `scripts/deploy.sh staging` en la VM de Azure y verifica los endpoints por HTTPS). Landing: «Despliegue» en Render (static site). Móvil: «Verificación en Expo Go» (`@probador-expo` recorre los criterios de aceptación contra el staging ya desplegado). Backend 013 y móvil 013 (producción y Google Play) siguen su propio proceso. La spec siguiente no empieza hasta que la anterior esté cerrada.

Las dependencias "parciales" se resuelven con **puertos vacíos** (§4) y contratos publicados el primer día de cada spec.

**Estrategia en dos versiones (decisión del 28 de oct).** El cuello de botella no es el desarrollo sino los 14 días de la prueba cerrada de Google Play. Por eso:
1. **1.0 (sin IA):** núcleo completo; sube desde el 9 nov al canal de prueba cerrada y se pide a producción el 23 nov.
2. **v1.1 (con IA y ML, specs 014 y 015):** se desarrolla del 23 nov al 1 dic (mientras Google revisa la 1.0), **detrás de un interruptor** (`AI_ENABLED`) y sin tocar el AAB que se revisa; BE-A y MO-A la construyen la 014; BE-B y MO-B construyen la 015 (recomendaciones con ML); MO-B actualiza privacidad y «Seguridad de datos» de Play y BE-A revisa la lectura de reservas de la 015. Se sube como actualización a la prueba cerrada hacia el 2 dic y a producción solo cuando la 1.0 esté aprobada (probablemente después de la entrega). Alcance, proveedor gratuito de IA y fecha exacta están pendientes (el PDF solo dice «sugerir título y categoría a partir de una fotografía, con revisión de quien publica», y que se concretará para la entrega académica).

**Fecha oficial de entrega: 1 al 4 de diciembre, con todos los specs completos.** Terminar *todo* en la primera semana de noviembre no era posible (~340 tareas para 4 personas con modelos gratuitos y 14 días de prueba cerrada); con la entrega oficial en diciembre, la 1.0 y la v1.1 caben con colchón de cerca de una semana. Si hay retraso se aplica la línea de corte (§6).

### Hitos

| Hito | Fecha de cierre | Debe estar cerrado (desplegado en staging / verificado en Expo Go) |
|---|---|---|
| H0 Base | 5 nov | 001 (infraestructura en Azure y servicios) + 002 (arquitectura base, primer despliegue) |
| H-Legal Landing legal | 5 nov | L001: privacidad, términos y eliminación de cuenta publicados (requisito para la ficha de Play) |
| H-Play Prueba cerrada | 9 nov | Primer AAB en la prueba cerrada con ≥ 12 testers (día 1 de 14); la cuenta ya está comprada |
| H1 Cuenta e imágenes | 7 nov | 003, 004: una persona puede registrarse y subir una foto |
| H2 Catálogo | 10 nov | 005, 006: publicar y encontrar un objeto |
| H3 Transacción | 13 nov | 007, 008: solicitar, aceptar y conversar |
| H4 Operación | 17 nov | 009, 010: entrega/devolución con QR, calificar, reportar |
| H5 Cierre | 22 nov | 011, 012, 013: avisos, reportes, endurecimiento, AAB final en la prueba cerrada |
| H-Prod Solicitud | 23 nov | Solicitud de acceso a producción enviada (día 14 cumplido el 22 nov) |
| H6 Publicación 1.0 | 30 nov – 7 dic (estimada) | App pública sin IA en Google Play (producción); depende de Google |
| H7 IA y ML v1.1 | ~2 dic | Specs 014 y 015: sugerencias con IA y recomendaciones con ML en la prueba cerrada (~2 dic); a producción solo si la 1.0 ya está aprobada |
| **H8 Entrega oficial** | **1–4 dic** | Todos los specs completos, documentación y demostración |

## 4. Piezas compartidas (se crean una vez y no se reimplementan)

| Pieza | Qué es | La crea | La reemplaza con la versión real |
|---|---|---|---|
| `notify.Notifier` | `Notify(ctx, userID, Event)` | 003 (vacía) | 011 |
| `moderation.Checker` | `Check(ctx, texto) → permitido/motivo` | 003 (permite todo) | 011 |
| `chat.Opener` | Abre la conversación de una reserva | 007 (vacía) | 008 |
| `ActiveBookingChecker` | ¿Usuario u objeto con reservas activas? (bloquea baja y borrado) | 003 (siempre falso) | 007 |
| `users.Directory` | `PublicUsers(ctx, ids)` perfiles públicos por lote | 003 | — |
| `images.Service` | Valida referencias y firma URLs | 004 | — |
| `auth` (middleware, `RequireRole`) | Identidad en el contexto | 003 | — |
| `cmd/api/modules.go` | Registro de módulos (1 línea por spec) | 003 | — |
| `api/paths/<modulo>.yaml` | Contrato por módulo | primera tarea de cada spec | — |
| Concha de navegación móvil (`_layout`, pestañas) | 5 pestañas con marcadores | 003 | — |
| Componentes `MoneyInput`, `RemoteImage`, `PhotoPicker`, `StatusBadge` | UI compartida | 004 (`RemoteImage`, `PhotoPicker`), 005 (`MoneyInput`), 007 (`StatusBadge`) | — |

## 5. Reparto de la spec 002 (arquitectura base)

| Persona | Tareas |
|---|---|
| BE-A | T001, T005, T006, T007, T009, T010, T011, T012, T013, T015, T022 |
| BE-B | T002, T003, T004, T008, T014, T016, T017, T018, T019, T020, T021, T023, T024–T031 (servicios, Redis, HTTPS y despliegue en Azure) |
| MO-A | T001, T002, T005, T010, T011, T012, T013, T016, T017, T018, T022 |
| MO-B | T003, T004, T006, T007, T008, T009, T014, T015, T019, T020, T021, T023, T024–T029 (servicios y Expo Go de la app) |

T001 de cada proyecto la hace primero la persona A; el resto espera a que esté terminada.

## 6. Línea de corte (qué se recorta si hay retraso)

| Si al final de… | y no está integrado… | Se recorta |
|---|---|---|
| 13 nov (H3) | 007 | 011 push y avisos; 010 solo calificaciones (**no** se recortan el filtro de contenido ni los reportes) |
| 1 dic | 014 o 015 sin integrar | Queda apagada (`AI_ENABLED=false` / `RECO_ENABLED=false`) y se entrega como demostración en staging |
| 17 nov (H4) | 009 | Pagos registrados y retrasos (queda evidencia + QR); incidencias |
| 19 nov | 011 push | Notificaciones push (queda lista de avisos dentro de la app) |
| 20 nov | 012 | Documentación extendida y accesibilidad fina; **nunca** el recorrido de punta a punta |

Nunca se recorta: cuenta, publicar, buscar, solicitar/reservar, entrega y devolución con QR, calificar, **reportar contenido y filtro de contenido** (requisito de Google Play para apps con contenido de usuarios), eliminar cuenta (dentro y fuera de la app), política de privacidad y la ruta a Google Play (§1).

## 7. Cómo trabajan los 4 OpenCode a la vez

1. **Un dueño por spec.** Solo esa persona edita su `tasks.md` y su código.
2. **Contrato primero.** Día 1 de cada spec: el backend publica `api/paths/<modulo>.yaml`; el móvil empieza a la par.
3. **Repositorio y revisión:** una rama y una PR por spec, `main` protegida, por el MCP de GitHub (`@gestor-repo`); una persona revisa y fusiona. Sin pipeline automatizado. Los commits, ramas y PR solo los hace `@gestor-repo`, con aprobación.
4. **Sincronía diaria** de 15 min: qué se integró, qué bloquea, qué contrato cambia.
5. **Un cambio de contrato** se avisa en el grupo y se actualiza en ambos lados el mismo día.
6. **`memoria.md` solo-anexar** con tu etiqueta; conflictos: conservar ambas líneas.
7. **Al terminar cada spec:** `@revisor-constitucion`, la tarea de cierre (Despliegue / Verificación en Expo Go) y marcar el hito.
8. **Desde el 10 de nov**, cada noche una persona ejecuta el recorrido de punta a punta (spec 012) y reporta fallos.
9. **MCP:** solo `@devops-mcp` (backend: MCP de Azure con escritura y aprobación por llamada, `az`/SSH solo para lo que el MCP no cubra, y MCP de Cloudinary; landing: Render) y `@probador-expo` (móvil) usan los MCP; las claves son variables de entorno o la sesión de `az`, nunca archivos. Los demás agentes piden el trabajo a ellos.
10. **Modelos gratuitos:** tareas de 1–3 archivos; si un agente falla dos veces, se para, se reescribe la tarea más pequeña y se reintenta.

## 8. Políticas de producto (confirmadas el 28 de oct por el responsable del producto, con los cambios marcados)

El PDF dice que importes y plazos se fijan en las políticas del servicio. Estos valores fueron **confirmados** el 28 de oct (POL-02 y POL-14 modificadas por el responsable); cada spec los cita por su ID y viven en una sola constante (`internal/policy` en backend, `src/lib/policy.ts` en móvil).

| ID | Política | Valor provisional |
|---|---|---|
| POL-01 | Cobro | MXN, tarifa por día. Fechas inclusivas: `días = (fin − inicio) + 1`; un día iniciado cuenta completo. Total = días × tarifa. Préstamo = $0 |
| POL-02 | Duración | Mínimo 1 día, **máximo 7 días (una semana)** — confirmado |
| POL-03 | Vencimiento de solicitud | 48 h o al llegar la fecha de inicio (lo primero) |
| POL-04 | Precio sugerido | 3 % del valor declarado por día, mínimo $10, redondeado hacia arriba a múltiplos de $5. Valor declarado entre $50 y $100,000 |
| POL-05 | Modalidades de pago | `full_on_delivery` (100 % en la entrega) o `split_50_50` (entrega = `ceil(total/2)`, devolución = `floor(total/2)`) |
| POL-06 | QR | Vigencia 15 min, un solo uso, ligado a reserva + etapa; regenerar invalida el anterior |
| POL-07 | Devolución tardía | Tolerancia 2 h tras el fin del último día (`America/Mexico_City`). Ajuste = días de retraso (redondeo arriba) × tarifa diaria. Solo se **registra**, no se cobra. Préstamo sin ajuste |
| POL-08 | Reporte tras devolver | Ventana de 72 h |
| POL-09 | Cancelación tardía y ausencia | Sin monto automático. Se informa antes de solicitar; la consecuencia es incidencia y posible advertencia o suspensión manual |
| POL-10 | Calificaciones | 1–5, comentario ≤ 300 caracteres, hasta 14 días después de completar |
| POL-11 | Chat | Texto ≤ 1000 caracteres, 30 mensajes/min por persona; sin borrado en esta versión |
| POL-12 | Alta | Declaración de ser mayor de 18 años y aceptación de términos; **sin** verificación de correo |
| POL-13 | Fotos | 1–5 por objeto; 1–5 por etapa de evidencia; ≤ 5 MB; JPEG, PNG o WebP |
| POL-14 | Categorías | Herramientas de trabajo en su mayoría, más movilidad: `herramientas_manuales` (Herramientas manuales), `herramientas_electricas` (Herramientas eléctricas), `construccion_jardineria` (Construcción y jardinería), `movilidad` (Patines, patinetas, bicicletas y similares), `deportes` (Deportes y aire libre), `hogar` (Hogar), `otros` (Otros) |
| POL-15 | Sanciones | Advertencia o suspensión con fecha fin, decididas manualmente; no cambian el promedio |
| POL-16 | Contenido prohibido | Sexual, armas, drogas, ilegal; filtro por lista de palabras + reportes + revisión humana |
| POL-17 | Baja de cuenta | Si hay reservas activas, se rechaza; si no, se **anonimiza** la cuenta (correo, nombre y teléfono se sustituyen, se revocan sesiones y tokens de push) y se muestra «Cuenta eliminada». Reservas, evidencia e incidencias se conservan **sin datos que identifiquen a la persona**, por seguridad de la contraparte (Google Play exige poder solicitar el borrado de la cuenta y sus datos) |

## 9. Fuera de alcance (consciente)

- Más IA o ML que la sugerencia por foto (spec 014) y las recomendaciones (spec 015): redes neuronales, embeddings externos, entrenamiento en línea. La IA y el ML **no** van en la 1.0; entran como v1.1.
- Páginas con servicios o cuentas en la web: la **landing** es solo informativa (no hay registro, ni inicio de sesión, ni panel web). Sin App Store en esta versión.
- Verificación de correo, recuperación de contraseña, inicio de sesión social.
- Pasarela de pagos, cobros automáticos, seguros, depósitos y garantías.
- Panel de administración visual: la administración se hace con endpoints y `scripts/admin.http`.
- Apelaciones, escalamiento y etapas extra de incidencias.
- Tiempo real con WebSockets (el chat usa consulta periódica + push).

## 10. Antes de arrancar (2 de nov) (lo prepara quien administra el repositorio)

- [ ] **Repositorios en GitHub (Art. XV backend y móvil, Art. XIII landing):** la persona crea los tres repositorios, protege `main` y crea el token `GITHUB_PERSONAL_ACCESS_TOKEN` (permisos mínimos, un token por repositorio o uno limitado a los tres); `@gestor-repo` hace ramas, commits y PR por el MCP de GitHub; una persona revisa y fusiona. Sin pipeline automatizado (CI) por ahora.
- [ ] **Lo único manual (Art. XIII.2 backend):** registrarse en Azure y en Cloudinary (y en Render y Expo para landing y móvil), `az login` en la máquina de quien despliega, `CLOUDINARY_URL` y `RENDER_API_KEY` (solo para la landing) como variables de entorno del sistema; reiniciar OpenCode después de definirlas. Expo se autentica con `opencode mcp auth expo`.
- [ ] Cuenta **gratuita de Azure creada con el correo de la cuenta principal del proyecto** (la de Google Play; requisito del curso): pide tarjeta y teléfono pero **no cobra mientras no se actualice a pago por uso**; el crédito de 200 USD dura **30 días desde el alta**, así que se crea justo antes de la spec 001. Alerta de presupuesto desde el primer día. Si la oferta se rechaza porque la persona ya la usó, otra persona del equipo hace la verificación. La región y la talla gratuita las elige `devops-mcp` revisando disponibilidad (`az vm list-skus`); la VM, MongoDB, Redis y el certificado HTTPS los crea la spec 001 por OpenCode.
- [ ] Cuenta de **Cloudinary** (nombre de nube, API key, API secret); carpeta base `vaiven/`.
- [ ] Cuenta de **Expo** para las 2 personas de móvil (MCP de Expo en la spec móvil 001; EAS solo para la spec 013).
- [x] Hosting: **Azure** (VM gratuita con Docker Compose) para el backend, `staging` en el puerto 8443 (spec 001/002) y `production` en el 443 (spec 013); la **landing** sigue en **Render** (static site `vaiven-landing`, spec L001). El origen de Render queda por definir al integrar el repositorio, pero **solo afecta a la landing**.
- [x] Cuenta de desarrollador de **Google Play** pagada y con licencia (confirmado el 2 de nov); y pendiente: lista de 15–20 testers antes del día 1 de la prueba cerrada (ruta en §1). **Es lo único que cuesta dinero.**
- [x] Correo de soporte público: `vaivenedeltrabajo@gmail.com` (privacidad, eliminación de cuenta, ARCO y ficha de Play).
- [x] Responsable de los datos / nombre del desarrollador: Joseph Abrham Yañez Garcia (persona física, cuenta personal).
- [ ] Secretos compartidos por un canal seguro (nunca en git); cada persona crea su `.env` local.
- [ ] Cada persona: `opencode` instalado, sesión iniciada con Zen y `opencode models` verificado.
- [ ] Una persona de cada pareja confirma que Docker funciona (MongoDB local para las pruebas locales).

## 11. Riesgos principales

| Riesgo | Probabilidad | Mitigación |
|---|---|---|
| Plazo corto para 14 specs y la landing | Alta | Línea de corte (§6), contrato primero, tareas pequeñas |
| Un modelo gratuito deja de estar disponible | Media | Alternos en `memoria.md`; cambiar solo `opencode.json` |
| Conflictos entre OpenCodes | Media | Dueños, módulos, i18n por funcionalidad, `memoria` solo-anexar |
| Push remoto no funciona en Expo Go (Android) | Alta | En Expo Go se marca «no verificable»; se prueba con *development build* de EAS en la spec 013 |
| Memoria justa en la VM gratuita (1 GiB para MongoDB, Redis y la API; luego dos entornos) | Alta | Una sola réplica, sin nginx; límites de memoria por contenedor, `GOMEMLIMIT`, swap de 2 GB (las tallas nuevas no traen disco temporal), `maxmemory` en Redis y caché reducida de Mongo; staging se reduce o se detiene mientras producción esté en uso; si no alcanza, consultar antes de cambiar de talla |
| Créditos de CPU de la serie B (línea base 20 %; 10 % en B1s): si se agotan, la VM se estrangula | Media | Cargas ligeras, vigilar la métrica de créditos; reportar si hay estrangulamiento |
| `NotAvailableForSubscription`: la región o la talla no están disponibles para la suscripción | Alta | `devops-mcp` revisa `az vm list-skus` antes de crear y prueba regiones y tallas (B2ats_v2 → B1s → B2pts_v2, ARM64 exige imagen `arm64`); sin zonas de disponibilidad; si es «Azure for Students», solo regiones de la política |
| La IP pública estándar no es gratuita (≈ 3–4 USD/mes, desde 2025 es obligatoria) | Media (cierta) | Costo aceptado; sale del crédito de 30 días y después de la tarjeta; alerta de presupuesto desde el primer día |
| Un certificado de Let's Encrypt no se emite o no se renueva | Media | `certbot renew --dry-run` en la spec 001; puerto 80 abierto solo para certbot; la API recarga el certificado sin reiniciarse |
| El crédito de Azure vence a los 30 días (con la prueba cerrada y la revisión de Google pasando de principios de diciembre) | Alta | Crear la cuenta justo antes de empezar; sin pasar a pago por uso los recursos se desactivan al terminar el crédito; si hay que seguir, una persona decide actualizar (costo: IP pública ≈ 0,12 USD/día) y se borra todo al terminar; alerta de presupuesto |
| Una sola VM: sin alta disponibilidad | Media | Reinicio automático de contenedores y de la VM, monitor externo, reversión con `--rollback`; se documenta como límite conocido |
| Render necesita un origen (repositorio) para la landing | Media | La tarea «Despliegue» de la landing no cierra hasta integrar el repositorio; el backend no depende de él |
| Cambios de contrato a medias | Media | Aviso el mismo día y actualización en ambos lados |
| Políticas rechazadas | Baja | Una sola constante por proyecto; cambio de 1 archivo |
| **Prueba cerrada de Google Play (12 testers, 14 días) y revisión de producción** | **Alta** | Cuenta ya comprada: día 1 el 9 nov, día 14 el 22 nov, solicitud el 23 nov y publicación estimada 30 nov – 7 dic (después de la entrega si Google tarda). Testers confirmados antes del 9 nov; actualizaciones durante la prueba. Cualquier retraso en cuenta, testers o primer AAB mueve la publicación día por día. Fuente: guías de 2026 ([Choicely](https://www.choicely.com/blog/google-play-12-tester-rule)); confirmar en Play Console Help |
| Verificación de identidad de la cuenta de Google Play tarda | Media | Confirmarla el 2–3 nov; cada día de demora mueve la publicación un día |
| MongoDB en la VM sin copias de seguridad automáticas | Media | `mongodump` manual cifrado y copiado fuera de la VM antes de cada despliegue grande y cada semana; documentado en backend 013 |
| Cola/límite de compilaciones gratuitas de EAS | Media | Solo se usan en la spec 013 |
| Google rechaza la ficha o la app (datos, permisos, contenido de usuarios) | Media | Spec móvil 013 prepara hoja de seguridad de datos, calificación de contenido y justificación de permisos; reportes y filtro no se recortan |

## 12. Puntos de conexión del móvil (para no editar archivos ajenos)

| Punto | Lo crea | Lo usan | Cómo se conecta |
|---|---|---|---|
| Pestañas, rutas marcador y `_layout` | 003 (MO-A) | todas | cada spec **reemplaza el contenido** de su archivo marcador; nadie edita los layouts |
| `renderActions` / `renderOwnerExtras` del detalle de objeto | 005 (MO-A) | 007 (Solicitar), 011 (Reportar) | una línea en el archivo del dueño |
| `renderReputation` / `renderExtraRows` del perfil | 003 (MO-A) | 010 (reputación, Mis incidencias), 011 (Notificaciones) | una línea registrada desde el `index.ts` de la feature |
| Secciones `chat`, `deliveryAndPayments`, `ratingsAndIncidents` del detalle de reserva | 007 (MO-A) | 008, 009, 010 | cada feature registra su componente en su `index.ts` |
| `ModerationNotice` | 011 (MO-B) | 005, 008 | una línea en el formulario correspondiente |
| i18n y claves de consulta | cada feature | — | `src/i18n/<f>.ts` + reexport de una línea; claves en `features/<f>/keys.ts` |

## 13. Tareas por spec

Incluye la tarea final de cierre de cada spec («Despliegue» en backend, «Verificación en Expo Go» en móvil).

| Spec | Backend | Móvil |
|---|---|---|
| 001 Infraestructura Azure / Servicios MCP | 9 | 4 |
| 002 Arquitectura base | 31 | 29 |
| 003 | 14 | 13 |
| 004 | 11 | 10 |
| 005 | 10 | 13 |
| 006 | 10 | 10 |
| 007 | 13 | 13 |
| 008 | 10 | 11 |
| 009 | 15 | 13 |
| 010 | 12 | 14 |
| 011 | 14 | 13 |
| 012 | 12 | 12 |
| 013 | 9 | 11 |
| 014 | 9 | 8 |
| 015 | 11 | 8 |
| **Total** | **191** | **183** |

Landing: L001 (12 tareas) y L002 (11 tareas), ver `specs/` del repositorio `vaiven-landing-page`. **Total general: 397 tareas.**
