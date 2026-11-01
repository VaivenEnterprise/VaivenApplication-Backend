# ROADMAP — VAIVÉN (backend + móvil)

> **Nota (2026-11-01):** este plan se escribió cuando VAIVÉN era un monorepo. Ahora son tres repositorios: `backend/` → `vaiven-backend`, `mobile/` → `vaiven-frontend`, `landing/` → `vaiven-landing-page`. Donde el texto diga una de esas carpetas, léase el repositorio correspondiente.

> **Nota (2026-11-01, segunda reorganización):** (1) se eliminó todo lo relativo a repositorio, ramas, PR y pipeline CI/CD (y con ello la antigua spec 016 de pruebas/DevSecOps): esa parte se integrará de cero; (2) el trabajo va **por OpenCode y servidores MCP**: la spec **001 es «Servicios por MCP»** (MongoDB Atlas, Cloudinary, Render) y la arquitectura base pasa a ser la **002**; (3) **cada spec termina desplegada**: backend y landing en Render por MCP (tarea final «Despliegue») y la app móvil verificada **en local con Expo Go** contra staging (tarea final «Verificación en Expo Go»); la spec siguiente no empieza hasta que esa tarea pase. La app móvil no se despliega salvo la publicación en Google Play (spec 013).

> Plan maestro: backend y móvil specs `001` a `013`, y el proyecto `landing/` (L001–L002). Si este archivo contradice una constitución, gana la constitución.
> Última actualización: 2026-11-01 · Constituciones backend v1.3.0, móvil v1.2.0, landing v1.1.0 · Decisiones de producto confirmadas (§8)

## 1. Fechas

| Hito | Fecha |
|---|---|
| Arranque del desarrollo | **Jueves 29 de octubre** |
| **Código congelado de la 1.0 (sin IA)** | **Domingo 15 de noviembre** |
| **Solicitud de acceso a producción en Google Play** (día 14 de la prueba cerrada = 14 nov) | **Domingo 15 de noviembre** |
| Revisión de Google (la controla Google; suele tardar ~7 días o más). Mientras tanto: correcciones, sin funcionalidad nueva en 1.0, y desarrollo de la IA (spec 014) | 16 – 22+ de noviembre |
| **Primera versión pública sin IA (1.0) en Google Play (producción)** | Meta: **viernes 20 de noviembre** solo si Google responde en ≤ 5 días; **estimada 22 – 25 de nov** |
| IA (spec 014) y ML (spec 015): desarrollo 16–24 nov; v1.1 a la prueba cerrada el 25 nov y a producción 26–28 nov, cuando la 1.0 ya esté aprobada | 16 – 28 de noviembre |
| Documentación final, demostración y colchón | 29 nov – 30 nov |
| **ENTREGA OFICIAL: todos los specs completos (001–014 y landing)** | **1 – 4 de diciembre** |

> El calendario incluye fines de semana (1–2 y 8–9 de nov). Es ajustado; la **línea de corte** (§6) dice qué se recorta primero si hay retraso.

### Ruta a Google Play (la fecha límite depende de esto)

Según las guías consultadas el 28 de oct (ver §11), una cuenta **personal** de Google Play creada después de nov-2023 debe correr una **prueba cerrada con ≥ 12 testers inscritos 14 días seguidos** antes de poder pedir producción; después Google revisa la solicitud (puede tardar ~7 días o más). **Decisión del 28 de oct (actualizada):** la cuenta se compra el **29–30 de oct** para que la prueba cerrada empiece el **1 de nov** (día 1) y termine el **14 de nov** (día 14); la solicitud de producción es el **15 de nov**. **Cada día que se retrase la compra o el primer AAB mueve la publicación un día.** El plan completo (sin IA primero, IA después) se explica en §3.

| Fecha | Qué ocurre | Quién |
|---|---|---|
| **29–30 oct** | **Comprar la cuenta de desarrollador de Google Play** (pago único de $25 USD) e iniciar el mismo día la verificación de identidad y teléfono | Administrador del repositorio |
| 29–31 oct | Reunir **15–20 testers** (cuentas de Google/Gmail) que acepten quedarse inscritos hasta ~el 25 de nov | Administrador |
| 30 oct–1 nov | Landing L001 publicada: política de privacidad, términos y eliminación de cuenta (las URL que pide la ficha) | MO-B |
| 31 oct–1 nov | Backend de producción creado (backend 013 fase 1) y primer **AAB** (specs 001–003: registro e inicio de sesión) subido a la **prueba cerrada**; ficha con textos preliminares | MO-B / BE-B / Administrador |
| **1 nov** | Testers aceptan la invitación → **día 1 de 14** (si la cuenta o el AAB se retrasan, el día 1 se mueve igual) | Administrador |
| 2–14 nov | Cada hito (H1–H4) se sube como actualización a la prueba cerrada; los testers no se salen. Día 14 = **14 nov** | MO-B |
| **15 nov** | Código congelado, AAB final (1.0, sin IA) y **solicitud de acceso a producción** (≥ 12 testers · 14 días) | Todos |
| 16–22+ nov | Revisión de Google; publicación de la 1.0 cuando apruebe | Administrador |

> **Riesgo alto:** los tiempos de verificación de la cuenta y de revisión los controla Google. Si algo se retrasa, la publicación se mueve con ellos; la meta del 20 de nov solo se cumple si todo sale a tiempo y Google responde rápido. Si la cuenta ya existe y fue creada antes del 13-nov-2023, este bloque no aplica.

## 2. Equipo y reparto

Cada persona trabaja con su propio OpenCode. Las parejas comparten spec de punta a punta (una hace el backend, la otra el móvil) y se coordinan por el contrato OpenAPI.

| Rol | Persona | Pareja | Dueña de las specs |
|---|---|---|---|
| **BE-A** (backend) | _(completar)_ | MO-A | 003, 005, 007, 009, 014 |
| **MO-A** (móvil) | _(completar)_ | BE-A | 003, 005, 007, 009 |
| **BE-B** (backend) | _(completar)_ | MO-B | 001, 004, 006, 008, 010, 011, 015 |
| **MO-B** (móvil) | _(completar)_ | BE-B | 001, 004, 006, 008, 010, 011, **013 móvil**, **landing** |

El **administrador del repositorio** (quien paga y administra la cuenta de Google Play) es responsable de la cuenta, los testers y las respuestas de Google; MO-B prepara los artefactos (AAB, ficha, landing) y BE-B el backend de producción (backend 013).

Spec **002** (arquitectura base) se reparte por tareas, ver §5; la **001** (servicios por MCP) la hacen BE-B y MO-B. Spec **012** es de todos. La landing es pequeña y estática: si MO-B está saturado, puede hacerla el administrador.

## 3. Specs y calendario

Prioridad: **P0** = sin esto no hay app · **P1** = esperado · **P2** = recortable.

| Spec | Nombre | Qué entrega | Dueñas | Prio. | Depende de | Ventana |
|---|---|---|---|---|---|---|
| 001 | **Servicios por MCP** | Atlas, Cloudinary y Render aprovisionados y verificados por MCP desde OpenCode (móvil: Expo Go + MCP de Expo) | BE-B / MO-B | P0 | — | 29 oct |
| 002 | Arquitectura base | Esqueleto de backend y app; primer despliegue a staging (backend) y primera verificación en Expo Go (móvil) | repartida | P0 | 001 | 29–31 oct |
| 003 | Cuenta y autenticación | Registro, login, sesión, perfil, baja; concha de navegación | BE-A / MO-A | P0 | 002 | 31 oct–2 nov |
| 004 | Imágenes | Firmas de subida, selector y subida de fotos | BE-B / MO-B | P0 | 001, 003 (auth) | 31 oct–2 nov |
| 005 | Objetos | Publicar, editar, pausar, precio sugerido | BE-A / MO-A | P0 | 003, 004 | 3–5 nov |
| 006 | Búsqueda | Texto, categoría, zona, modalidad, fechas | BE-B / MO-B | P0 | 005 | 3–5 nov |
| 007 | Solicitudes y reservas | Cotización, solicitud, aceptar/rechazar, reserva, cancelar | BE-A / MO-A | P0 | 005 | 5–8 nov |
| 008 | Chat privado | Mensajes de texto por reserva | BE-B / MO-B | P1 | 007 | 5–8 nov |
| 009 | Entrega, devolución y pagos | Evidencia, QR, confirmaciones, pago registrado, retrasos | BE-A / MO-A | P0 | 004, 007 | 8–11 nov |
| 010 | Calificaciones e incidencias | Reputación, reportes con seguimiento manual, sanciones | BE-B / MO-B | P1 | 009 (parcial) | 8–11 nov |
| 011 | Notificaciones y moderación | Avisos en app y push (P2), filtro de contenido y reportes (**P1: exigidos por la política de contenido de usuarios de Google Play**) | BE-B / MO-B | P1 | 007–010 | 10–12 nov |
| 012 | Integración y entrega | Pruebas de punta a punta, endurecimiento, documentación, AAB final | todos | P0 | todas | 12–15 nov |
| 013 | Publicación en Google Play (móvil) y producción (backend) | Ficha, cumplimiento, prueba cerrada, producción; backend de producción | MO-B / BE-B + administrador | P0 | 002 (inicio), 012 (cierre) | 30 oct–20 nov |
| 014 | **IA (v1.1, después de la 1.0)** | Sugerir título y categoría a partir de una foto, siempre con revisión de quien publica (proveedor gratuito; interruptor `AI_ENABLED`) | BE-A / MO-A (+ MO-B cumplimiento) | P1 | 004, 005, 011, 013 | 16–24 nov |
| 015 | **ML: recomendaciones (v1.1)** | «Recomendado para ti»: recomendador híbrido propio (colaborativo objeto-objeto + contenido), evaluado con Hit@10/Recall@10/MRR contra popularidad; interruptor `RECO_ENABLED` | BE-B / MO-B | P1 | 005, 006, 007, 013 | 16–24 nov |
| L001 | Landing: base y páginas legales | Sitio estático, privacidad, términos, eliminación de cuenta, publicación gratuita | MO-B | P0 | — | 30 oct–1 nov |
| L002 | Landing: página principal | Información de la app, cómo funciona, FAQ, botón a Google Play | MO-B | P1 | L001 | 2–9 nov |

**Regla de cierre (todas las specs desde la 002):** una spec está terminada solo cuando su última tarea pasa. Backend y landing: «Despliegue» (`@devops-mcp` despliega en Render y verifica los endpoints o páginas de la spec). Móvil: «Verificación en Expo Go» (`@probador-expo` recorre los criterios de aceptación contra el staging ya desplegado). Backend 013 y móvil 013 (producción y Google Play) siguen su propio proceso. La spec siguiente no empieza hasta que la anterior esté cerrada.

Las dependencias "parciales" se resuelven con **puertos vacíos** (§4) y contratos publicados el primer día de cada spec.

**Estrategia en dos versiones (decisión del 28 de oct).** El cuello de botella no es el desarrollo sino los 14 días de la prueba cerrada de Google Play. Por eso:
1. **1.0 (sin IA):** núcleo completo; sube desde el 1 nov al canal de prueba cerrada y se pide a producción el 15 nov.
2. **v1.1 (con IA y ML, specs 014 y 015):** se desarrolla del 16 al 24 nov (mientras Google revisa la 1.0), **detrás de un interruptor** (`AI_ENABLED`) y sin tocar el AAB que se revisa; BE-A y MO-A la construyen la 014; BE-B y MO-B construyen la 015 (recomendaciones con ML); MO-B actualiza privacidad y «Seguridad de datos» de Play y BE-A revisa la lectura de reservas de la 015. Se sube como actualización el 25 nov y a producción cuando la 1.0 esté aprobada. Alcance, proveedor gratuito de IA y fecha exacta están pendientes (el PDF solo dice «sugerir título y categoría a partir de una fotografía, con revisión de quien publica», y que se concretará para la entrega académica).

**Fecha oficial de entrega (28 de oct): 1 al 4 de diciembre, con todos los specs completos.** Terminar *todo* en la primera semana de noviembre no era posible (~340 tareas para 4 personas con modelos gratuitos y 14 días de prueba cerrada); con la entrega oficial en diciembre, la 1.0 y la v1.1 caben con colchón de cerca de una semana. Si hay retraso se aplica la línea de corte (§6).

### Hitos

| Hito | Fecha de cierre | Debe estar cerrado (desplegado en staging / verificado en Expo Go) |
|---|---|---|
| H0 Base | 30 oct | 001 (servicios por MCP) + 002 (arquitectura base) |
| H-Legal Landing legal | 1 nov | L001: privacidad, términos y eliminación de cuenta publicados (requisito para la ficha de Play) |
| H-Play Prueba cerrada | 1 nov | Primer AAB en la prueba cerrada con ≥ 12 testers (día 1 de 14); la cuenta se compra el 29–30 oct |
| H1 Cuenta e imágenes | 2 nov | 003, 004: una persona puede registrarse y subir una foto |
| H2 Catálogo | 5 nov | 005, 006: publicar y encontrar un objeto |
| H3 Transacción | 8 nov | 007, 008: solicitar, aceptar y conversar |
| H4 Operación | 11 nov | 009, 010: entrega/devolución con QR, calificar, reportar |
| H5 Cierre | 15 nov | 011, 012, 013: avisos, reportes, endurecimiento, AAB final en la prueba cerrada |
| H-Prod Solicitud | 15 nov | Solicitud de acceso a producción enviada (día 14 cumplido el 14 nov) |
| H6 Publicación 1.0 | 20 nov (meta) · 22–25 nov (estimada) | App pública sin IA en Google Play (producción); depende de Google |
| H7 IA y ML v1.1 | 25–28 nov | Specs 014 y 015: sugerencias con IA y recomendaciones con ML en la prueba cerrada (25 nov) y en producción (26–28 nov) |
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
| BE-B | T002, T003, T004, T008, T014, T016, T017, T018, T019, T020, T021, T023, T024–T027 (servicios y staging del backend) |
| MO-A | T001, T002, T005, T010, T011, T012, T013, T016, T017, T018, T022 |
| MO-B | T003, T004, T006, T007, T008, T009, T014, T015, T019, T020, T021, T023, T024–T029 (servicios y Expo Go de la app) |

T001 de cada proyecto la hace primero la persona A; el resto espera a que esté terminada.

## 6. Línea de corte (qué se recorta si hay retraso)

| Si al final de… | y no está integrado… | Se recorta |
|---|---|---|
| 8 nov (H3) | 007 | 011 push y avisos; 010 solo calificaciones (**no** se recortan el filtro de contenido ni los reportes) |
| 24 nov | 014 o 015 sin integrar | Queda apagada (`AI_ENABLED=false` / `RECO_ENABLED=false`) y se entrega como demostración en staging; la 1.0 ya está publicada |
| 11 nov (H4) | 009 | Pagos registrados y retrasos (queda evidencia + QR); incidencias |
| 12 nov | 011 push | Notificaciones push (queda lista de avisos dentro de la app) |
| 13 nov | 012 | Documentación extendida y accesibilidad fina; **nunca** el recorrido de punta a punta |

Nunca se recorta: cuenta, publicar, buscar, solicitar/reservar, entrega y devolución con QR, calificar, **reportar contenido y filtro de contenido** (requisito de Google Play para apps con contenido de usuarios), eliminar cuenta (dentro y fuera de la app), política de privacidad y la ruta a Google Play (§1).

## 7. Cómo trabajan los 4 OpenCode a la vez

1. **Un dueño por spec.** Solo esa persona edita su `tasks.md` y su código.
2. **Contrato primero.** Día 1 de cada spec: el backend publica `api/paths/<modulo>.yaml`; el móvil empieza a la par.
3. **Repositorio y revisión:** el flujo de ramas, revisión y pipeline **no está definido todavía**; se integrará de cero. Mientras tanto no se hacen commits ni se crean ramas desde los agentes.
4. **Sincronía diaria** de 15 min: qué se integró, qué bloquea, qué contrato cambia.
5. **Un cambio de contrato** se avisa en el grupo y se actualiza en ambos lados el mismo día.
6. **`memoria.md` solo-anexar** con tu etiqueta; conflictos: conservar ambas líneas.
7. **Al terminar cada spec:** `@revisor-constitucion`, la tarea de cierre (Despliegue / Verificación en Expo Go) y marcar el hito.
8. **Desde el 5 de nov**, cada noche una persona ejecuta el recorrido de punta a punta (spec 012) y reporta fallos.
9. **MCP:** solo `@devops-mcp` (backend, landing) y `@probador-expo` (móvil) usan los MCP; sus claves son variables de entorno del sistema, nunca archivos. Los demás agentes piden el trabajo a ellos.
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

## 10. Antes del 29 de octubre (lo prepara quien administra el repositorio)

- [ ] **Repositorio, ramas, revisión y pipeline:** pendiente; se definirá e integrará de cero (fuera de este SDD por ahora).
- [ ] Claves para los MCP como variables de entorno del sistema: `MDB_MCP_API_CLIENT_ID` y `MDB_MCP_API_CLIENT_SECRET` (cuenta de servicio de Atlas), `CLOUDINARY_URL`, `RENDER_API_KEY`; reiniciar OpenCode después de definirlas. Expo se autentica con `opencode mcp auth expo`.
- [ ] Cuenta de **MongoDB Atlas** con una cuenta de servicio (API); el clúster gratuito M0, el usuario y la red los crea la spec 001 por MCP.
- [ ] Cuenta de **Cloudinary** (nombre de nube, API key, API secret); carpeta base `vaiven/`.
- [ ] Cuenta de **Expo** para las 2 personas de móvil (MCP de Expo en la spec móvil 001; EAS solo para la spec 013).
- [x] Hosting gratuito: **Render** (decidido), por MCP: `vaiven-staging` (spec backend 002), `vaiven-prod` (013) y el static site `vaiven-landing` (L001). El origen que Render consume (repositorio o imagen) queda **por definir** al integrar el repositorio.
- [ ] Cuenta de desarrollador de **Google Play** pagada y verificada **el 29–30 de oct**, y lista de 15–20 testers antes del 1 nov (ruta en §1). **Es lo único que cuesta dinero.**
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
| Render necesita un origen (repositorio o imagen) y aún no existe | Alta | Las tareas «Despliegue» no se pueden cerrar hasta integrar el repositorio; mientras tanto las pruebas locales (`make`/`npm`) sí corren. Definirlo antes de la spec 002 |
| Cambios de contrato a medias | Media | Aviso el mismo día y actualización en ambos lados |
| Políticas rechazadas | Baja | Una sola constante por proyecto; cambio de 1 archivo |
| **Prueba cerrada de Google Play (12 testers, 14 días) y revisión de producción** | **Alta** | Cuenta comprada el 29–30 oct: día 1 el 1 nov, día 14 el 14 nov, solicitud el 15 nov y publicación estimada 22–25 nov. Testers confirmados antes del 1 nov; actualizaciones durante la prueba. Cualquier retraso en cuenta, testers o primer AAB mueve la publicación día por día. Fuente: guías de 2026 ([Choicely](https://www.choicely.com/blog/google-play-12-tester-rule)); confirmar en Play Console Help |
| Verificación de identidad de la cuenta de Google Play tarda | Media | Iniciarla el mismo día de la compra (29–30 oct); cada día de demora mueve la publicación un día |
| Render gratuito se "duerme" (arranque en frío de ~1 min) | Alta | Monitor gratuito que haga ping cada 10 min; app con estados de carga; documentado en backend 013 |
| Atlas M0 sin copias de seguridad automáticas | Media | Exportación manual (`mongodump`) antes de cada despliegue grande; documentado |
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
| 001 Servicios por MCP | 7 | 4 |
| 002 Arquitectura base | 27 | 29 |
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
| **Total** | **184** | **182** |

Landing: L001 (11 tareas) y L002 (11 tareas), ver `specs/` del repositorio `vaiven-landing-page`. **Total general: 388 tareas.**
