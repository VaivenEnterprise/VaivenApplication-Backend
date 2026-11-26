# Spec 018 — Hardening de seguridad: de MEDIO-ALTO a ALTO (backend)

- **Dueño propuesto:** BE-B (coordina) · **Con:** BE-A (specs 003 y 012-T003), MO-A y MO-B (app móvil) · **Prioridad:** P1
- **Depende de:** 002, 003, 012, 013, 016 y 017 · **Ventana:** por decidir, después de la 016
- **Constitución:** v2.0.0 · **Estado:** **APROBADA**. El responsable confirmó la aprobación el 2026-11-18 (B-2). Las tareas conservan las dependencias y autorizaciones del §9.
- **Fuente:** Diagnóstico interno del 17 de noviembre de 2026, disponible a solicitud del equipo. Los hallazgos concretos no se publican en este repositorio; cuando un control los necesita para aprobarse, se indica que se revisa por canal privado.

## 1. Objetivo y por qué

El diagnóstico interno ubica el backend en un nivel de seguridad **MEDIO-ALTO**: los controles preventivos del código y de la CI son sólidos. Para llegar a **ALTO** hay que reforzar cinco frentes:
- protección de las respuestas HTTP;
- integridad de la cadena de suministro (lo que se construye es lo que se despliega);
- detección y respuesta;
- pruebas de seguridad en ejecución;
- control del ciclo de vida de las sesiones.

Esta spec define **qué** controles hay que agregar y **cómo se demuestra** que funcionan, para poder declarar el nivel **ALTO** solo con evidencia de ejecución. Las herramientas por sí solas no cuentan.

## 2. Relación con otras specs y trazabilidad (sin tareas duplicadas)

**Regla:** si un control ya tiene una tarea de implementación en otra spec, **esa tarea se mantiene con su dueño**. La 018 solo agrega los **criterios técnicos complementarios y la evidencia** exigida, y no crea una segunda tarea de implementación. No se elimina ninguna tarea existente sin aprobación del equipo.

| Control 018 | Tarea que implementa (dueño) | Qué agrega la 018 | Tarea 018 (solo verificación o diferencia) |
|---|---|---|---|
| C-01 Cabeceras y HSTS | **012-T003** «Cabeceras de seguridad y límite de cuerpo» (**BE-A**; abierta; es el RF-04 de la 012) | Criterios complementarios (lista completa de cabeceras, rutas que deben llevarlas) y **validación de HSTS** solo cuando HTTPS esté comprobado (RF-01) | **T002**: verificación y evidencia; **no implementa** |
| C-03 Escaneo de contenedores | **016-T011** «Escaneo de la imagen y SBOM» (BE-B) | Escaneo sobre el **digest antes de desplegar**, imágenes base por digest, excepciones con vencimiento, prueba negativa | **T005**: solo lo que la 016-T011 no cubra, si el equipo no amplía la 016-T011 |
| C-04 SBOM | **016-T011** (BE-B) | Asociación del SBOM al digest y atestado verificable | Parte de **T006**: verificación; el despliegue por digest (C-07) sí es nuevo |
| C-09 DAST | **016-T010** «Seguridad dinámica (ZAP baseline)» (BE-B) | Modo **activo aislado** en el runner y umbrales; el modo pasivo contra staging queda en el grupo B | **T009**: solo el modo aislado, si la 016-T010 no lo cubre |
| C-11 y C-12 Sesión | **003** RF-05 (**BE-A**) | Emisor y audiencia; objetivo de revocación ≤ 60 s | **T011** y **T012**: bloqueadas hasta la decisión de BE-A |
| C-06 y C-07 en producción | **013** `deploy-production.sh` | Verificación de firma antes de promover a producción | **T019**: bloqueada (autorización) |
| C-03 a C-07 en CI/CD | **017** (pipeline) | Pasos nuevos en los *workflows*; no se quita ningún control | — |

## 3. Historias

| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona usuaria, quiero que la API indique al cliente usar siempre HTTPS y no interpretar contenido de forma insegura, para que mis datos no viajen ni se procesen de forma insegura. |
| H-02 | P1 | Como equipo, queremos desplegar solo imágenes construidas por nuestro pipeline, sin vulnerabilidades graves conocidas y con inventario de componentes, para confiar en lo que corre. |
| H-03 | P1 | Como equipo, queremos enterarnos a tiempo de un ataque o de un despliegue fallido, para responder antes de que haya daño. |
| H-04 | P2 | Como persona usuaria, quiero que al cerrar sesión o dar de baja mi cuenta nadie pueda seguir usando mi sesión. |
| H-05 | P2 | Como persona usuaria del móvil, quiero que las listas carguen más rápido con datos móviles, sin que cambie la app. |
| H-06 | P3 | Como equipo, queremos decidir con datos si conviene un reverse proxy, infraestructura como código o separar staging de producción. |

## 4. Requisitos

**Grupo A — preparación local.** Se puede implementar **después de aprobar la spec**, sin tocar Azure ni producción.

| ID | Control | Requisito |
|---|---|---|
| RF-01 | C-01 Cabeceras y HSTS (**implementa 012-T003, BE-A**; aquí solo criterios y evidencia) | Toda respuesta de la API lleva cabeceras que impiden interpretar el contenido como otro tipo, cargarlo en marcos y filtrar la página de origen. Las respuestas con datos privados no se guardan en cachés intermedias. Ninguna cabecera existente se duplica. **HSTS**, solo después de validar los nombres de host reales, el certificado, dónde termina TLS, la compatibilidad con staging y el comportamiento HTTP/HTTPS: primero en **staging**, con vigencia corta, **sin** subdominios y **sin** *preload*. En producción, HSTS **no** se configura dentro de esta spec sin autorización (B-8) |
| RF-02 | C-02 Eventos auditables | Se registran como eventos estructurados: reutilización de una sesión ya usada, revocación de una familia de sesiones, inicio de sesión fallido, acceso prohibido y bloqueo por límite de tasa. Cada evento lleva tipo, momento, id de petición y resultado, **sin** tokens, contraseñas, correos, cuerpos de petición ni IP completa |
| RF-03 | C-03 Escaneo de contenedores (**implementa 016-T011**; aquí criterios adicionales) | Cada imagen construida se analiza antes de desplegarla. Una vulnerabilidad **alta o crítica con corrección disponible** detiene el despliegue. Las excepciones tienen motivo, dueño y fecha de vencimiento. Las imágenes base se fijan por su identificador inmutable y tienen actualizaciones automáticas propuestas |
| RF-04 | C-04 SBOM (**implementa 016-T011**; aquí asociación al digest y evidencia) | Cada imagen tiene un inventario de componentes en formato estándar (CycloneDX o SPDX) con dependencias directas y transitivas, versiones e identificadores de paquete, asociado al identificador inmutable de la imagen. No contiene secretos |
| RF-05 | C-05 Firma de imágenes | Cada imagen se firma por su identificador inmutable con credenciales efímeras del pipeline, sin llaves privadas de larga duración |
| RF-06 | C-06 Verificación obligatoria | Antes de desplegar se verifica la firma y que el firmante sea exactamente el pipeline de este repositorio en la rama esperada. Sin verificación válida no hay despliegue (en staging primero) |
| RF-07 | C-07 Despliegue por digest | El despliegue usa el identificador inmutable de la imagen, nunca una etiqueta que se pueda reasignar; la vuelta a la versión anterior también |
| RF-08 | C-08 Compresión segura | Las respuestas JSON grandes se comprimen cuando el cliente lo pide, sin cambiar su contenido. **Nunca** se comprimen las respuestas que llevan secretos (sesión, firmas de subida, QR, perfil propio) |
| RF-09 | C-09 DAST aislado (**implementa 016-T010** si se amplía; si no, T009 de esta spec) | Un análisis dinámico **activo** corre solo contra una instancia efímera dentro del pipeline, con datos sintéticos y una cuenta de prueba creada en ese momento. Falla con hallazgos altos y publica su reporte |
| RF-10 | C-10 Observabilidad y alertas (local) | Seis alertas iniciales (reutilización de refresh tokens, fallos repetidos de inicio de sesión, errores 5xx, reinicios anómalos de contenedores, vulnerabilidades críticas y fallos de verificación de firma). Cada una con fuente, umbral, severidad, destinatario, canal, prueba de activación, tiempo de respuesta y procedimiento (plan, C-10). Los destinatarios que no estén asignados quedan como **decisión pendiente**, sin inventarlos. Procedimiento de respuesta a incidentes escrito |
| RF-15 | Corrección CRLF | La prueba que busca claves en el repositorio no da falsos positivos por los finales de línea de Windows |

**Grupo B — requiere coordinación o autorización.** Nada de esto queda aprobado de forma implícita con la spec.

| ID | Control | Requisito | Quién lo desbloquea |
|---|---|---|---|
| RF-11 | C-11 Emisor y audiencia del token | El token de acceso declara y se valida contra un emisor por entorno y una audiencia de la app, con una transición que no cierre las sesiones activas | **BE-A** (spec 003) |
| RF-12 | C-12 Revocación de sesiones | **Objetivo propuesto: revocación efectiva en ≤ 60 segundos** después de un logout (sesión de esa familia), una baja de cuenta (todas sus sesiones) o una reutilización confirmada de refresh token (la familia afectada). Las alternativas y su impacto en las sesiones existentes están en el plan; **en esta spec no se implementa Redis ni `sessionVersion`** | **BE-A** (decisión pendiente) |
| RF-13 | C-13 Segregación staging/producción | Se analiza el aislamiento entre entornos y se proponen una mitigación sin costo (restringir el acceso a las herramientas de prueba, cuotas de recursos y secretos separados) y otra con costo (infraestructura separada). La topología concreta se revisa por canal privado | **Equipo** (costo, Art. XIII.1) y **autorización de Azure** |
| RF-14 | C-14 Bicep y reverse proxy | Se evalúa describir la infraestructura existente como código **sin recrear recursos** (comprobado con una vista previa sin cambios), y se evalúa un reverse proxy local | Proxy: **enmienda del Art. XIV.4**. Bicep: **autorización de lectura en Azure** |
| RF-16 | C-09 DAST en staging | Un análisis **pasivo** contra staging, con intensidad limitada y nunca contra producción | **Responsable**: autorización específica |
| RF-17 | C-10 Monitoreo en la nube | Alertas con un servicio de monitoreo de Azure | **Autorización de Azure**, y del **equipo** si tiene costo |
| RF-18 | C-08 Compatibilidad móvil | Prueba de la app con compresión activa contra staging | **MO-A / MO-B** |

| ID | Requisito no funcional |
|---|---|
| RNF-01 | **El contrato HTTP no cambia:** mismas rutas, mismos cuerpos JSON y mismos códigos de error (Art. IV) |
| RNF-02 | El inicio de sesión, el refresh, el logout, los roles y la detección de reutilización siguen funcionando igual (regresión en verde) |
| RNF-03 | Costo cero, salvo decisión explícita del equipo (Art. XIII.1) |
| RNF-04 | Sin dependencias nuevas de Go, salvo justificación en el plan y registro en `memoria.md` (Art. VIII) |
| RNF-05 | El p95 de las rutas de lista no empeora más de 10 % con la compresión (medido con la prueba de carga) |
| RNF-06 | Ningún control se desactiva o se debilita para conseguir un pipeline en verde |

## 5. Criterios de aceptación

| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada cualquier ruta de la API en staging (al menos una pública, una autenticada, un 404 y un error), cuando se consulta por HTTPS, entonces lleva todas las cabeceras de RF-01; las rutas con datos privados llevan `no-store`; la evidencia es la salida de `curl -sSI` guardada en el informe. HSTS se exige **solo** después de que la validación de RF-01 esté documentada, y nunca por HTTP |
| CA-02 | Dado un refresh token ya usado, cuando se reutiliza, entonces se registra un evento de reutilización con el id de petición y **sin** el token ni datos personales |
| CA-03 | Dada una imagen con una vulnerabilidad crítica conocida y con corrección (prueba controlada), cuando corre el pipeline, entonces no se despliega |
| CA-04 | Dada una imagen publicada, entonces se pueden obtener su inventario y su firma a partir de su identificador inmutable |
| CA-05 | Dada una imagen sin firma, o firmada por otra identidad, cuando se intenta desplegar en staging, entonces el despliegue se detiene (prueba negativa) |
| CA-06 | Dado un despliegue, entonces el servidor corre exactamente el identificador inmutable verificado, y la vuelta atrás usa el identificador anterior |
| CA-07 | Dada una lista grande pedida con compresión, entonces el JSON descomprimido es idéntico al que se recibe sin compresión; las rutas con secretos nunca se comprimen |
| CA-08 | Dado el análisis activo aislado, entonces no hay hallazgos altos y el reporte queda publicado |
| CA-09 | Dada cada una de las seis alertas del plan (C-10), cuando se simula su condición en staging (o en el runner, para las de pipeline), entonces la alerta llega por su canal con la severidad indicada; las que no tienen destinatario asignado no se dan por aceptadas |
| CA-10 | Dada la prueba de claves en Windows, entonces no hay falsos positivos por los finales de línea |
| CA-11 (B) | Dado un token de otro entorno, o sin la audiencia de la app, cuando llega a la API después de la transición, entonces se rechaza con 401 |
| CA-12 (B) | **En esta spec:** dada la propuesta de revocación, cuando BE-A la revisa, entonces queda documentada su decisión (objetivo y alternativa) en `docs/seguridad/decision-revocacion.md`. **Criterio para la tarea futura de implementación**, que no es de esta spec: un token de acceso emitido antes de un logout, una baja o una reutilización confirmada se rechaza en ≤ 60 s |
| CA-13 (B) | Dada la descripción de la infraestructura como código, cuando se compara con Azure, entonces no hay cambios propuestos |

## 6. Cuándo se puede declarar el nivel ALTO

Solo cuando **todos** estos controles estén **implementados y validados con evidencia** (ejecuciones, reportes, pruebas negativas):
- C-01 (cabeceras y HSTS en staging), implementado por la 012-T003 y verificado por la T002;
- C-02 (eventos);
- C-03, C-04, C-05, C-06 y C-07 (cadena de suministro con verificación obligatoria), incluidos los criterios que la 018 agrega a la 016-T011;
- C-09 en su modo aislado (sin altos), por la 016-T010 ampliada o por la T009;
- C-10 (alertas probadas);
- C-11 (emisor y audiencia), que **depende de BE-A**: si BE-A no lo aprueba, no se puede declarar ALTO y queda como riesgo residual documentado.

C-12, C-13 y C-14 **suman pero no son requisito**; sus riesgos residuales se documentan. HSTS en producción tampoco es requisito de esta spec (B-8).

## 7. Fuera de alcance

- Alta disponibilidad con varias instancias (costo).
- WAF administrado (Application Gateway WAF o Front Door Premium).
- Cambios en el repositorio móvil.
- Cambios del contrato HTTP.
- Rotación automática de secretos.
- Análisis activo contra staging o producción.

## 8. Preguntas: resueltas con propuesta y pendientes

**Resueltas para la spec aprobada**, sujetas a los permisos de cada tarea:

| # | Pregunta | Propuesta |
|---|---|---|
| P-1 | ¿Quién implementa C-01? | **012-T003, con BE-A** como dueño; la 018 solo verifica (T002). No se asume el visto bueno de BE-A sobre los criterios nuevos |
| P-2 | ¿La 016 conserva T010 y T011? | **Sí**, se conservan; la 018 aporta criterios y evidencia, con trazabilidad en el §2 |
| P-3 | Tiempo máximo de revocación (C-12) | **≤ 60 s** según el evento (RF-12) |
| P-4 | Umbrales de alerta | Seis alertas con umbral, severidad, canal y prueba (plan, C-10) |
| P-5 | HSTS | **Sin** `includeSubDomains` y **sin** `preload`; primero se valida; solo staging en esta spec |

**Pendientes de aprobación** (nadie las da por aprobadas):

- [NEEDS CLARIFICATION: BE-A acepta los criterios complementarios de C-01 en la 012-T003.]
- [NEEDS CLARIFICATION: el equipo decide si la 016-T011 y la 016-T010 se amplían con los criterios de C-03, C-04 y C-09, o si la 018 ejecuta solo la diferencia (T005 y T009).]
- [NEEDS CLARIFICATION: BE-A aprueba el objetivo de ≤ 60 s y elige la alternativa de C-12.]
- [NEEDS CLARIFICATION: destinatarios de las alertas (C-10) cuando no están asignados en el ROADMAP.]

## 9. Autorizaciones (B-1 a B-8)

| # | Qué | Quién decide | Tareas que desbloquea |
|---|---|---|---|
| B-1 | Enmienda del Art. XIV.4 para un reverse proxy | Equipo (Art. XI) | T017 |
| B-2 | Aprobación de la spec confirmada el 2026-11-18 | Equipo (Art. I), confirmación comunicada por el responsable | T001–T010, según sus dependencias |
| B-3 | Cambios en la sesión: emisor y audiencia, revocación | **BE-A** (spec 003) | T011, T012 |
| B-4 | DAST pasivo contra staging | Responsable | T013 |
| B-5 | Lectura de Azure (configuración de red, permisos y exportación) | Responsable (MFA) | T015, T016 |
| B-6 | Cualquier recurso con costo | Equipo (Art. XIII.1) | Partes con costo de T015 y T018 |
| B-7 | Prueba en la app móvil | **MO-A y MO-B** | T014 |
| B-8 | Commit, push, despliegue y cambios en producción (incluido HSTS en producción) | Responsable | T019, T021 y HSTS en producción |

El responsable autorizó además la preparación local de un piloto de segunda VM y Azure Standard Load Balancer. El límite incremental propuesto de USD 15 es condición de presupuesto. El piloto remoto requiere enmienda aplicable, B-5, B-6 y aprobación de la operación concreta. Ver `docs/seguridad/piloto-dos-vm-azure.md`.

## 10. Matriz de responsabilidades

| Control | Implementa | Verifica | Aprueba o desbloquea |
|---|---|---|---|
| C-01 Cabeceras y HSTS | BE-A (012-T003) | BE-B (T002) | BE-A |
| C-02 Eventos | BE-B (T003); BE-A o BE-B con su visto bueno (T004) | `revisor-constitucion` | BE-A para `internal/auth` |
| C-03 y C-04 Trivy y SBOM | BE-B (016-T011, o T005 y T006) | BE-B | Equipo (alcance de la 016) |
| C-05, C-06 y C-07 Firma, verificación y digest | BE-B (T006, T007) | BE-B | Responsable (B-8 para producción, T019) |
| C-08 Compresión | BE-B (T008) | MO-A y MO-B (T014) | MO-A y MO-B (B-7) |
| C-09 DAST | BE-B (016-T010 o T009; T013) | BE-B | Responsable (B-4) para staging |
| C-10 Alertas | BE-B con BE-A (T010) | BE-A y BE-B | Destinatarios: **pendiente** |
| C-11 y C-12 Sesión | BE-A (T011, T012) | BE-A | **BE-A** (B-3) |
| C-13 Segregación | Equipo (T015) | BE-B | Equipo y responsable (B-5, B-6) |
| C-14 Bicep y proxy | Equipo (T016, T017) | BE-B | Equipo (B-1); responsable (B-5) |
| C-15 CRLF | BE-B (T001) | BE-B | Aviso a BE-A (dueño de la 014) |

## 11. Riesgos residuales y reversibilidad

- **Riesgos que quedan aunque todo se apruebe:**
  - disponibilidad y aislamiento entre entornos, mientras no se apruebe C-13 con costo;
  - gestión de la llave de firma de tokens;
  - protección de aplicación administrada (fuera de alcance por costo);
  - alcance de la detección mientras no exista una plataforma central de registros (RF-17).

  El detalle de cada riesgo residual se revisa por canal privado.
- **Reversibilidad:** cada control tiene una condición de *rollback* en su ficha. Los controles de la API son una línea en `router.go`; los de CI/CD son pasos de *workflow*. HSTS empieza con vigencia corta. Ningún control bloqueante se desactiva sin una decisión escrita (RNF-06).
