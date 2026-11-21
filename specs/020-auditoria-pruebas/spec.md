# Spec 020 — Bitácora de auditoría inmutable y pruebas de seguridad avanzadas (backend)

- **Dueño sugerido:** BE-B · **Con:** BE-A (rutas de cuenta y de pagos que escriben en la bitácora) · **Prioridad:** P1
- **Depende de:** 003, 009, 010, 011, 012, 016, 017 y 018 (sus controles y su contrato) · **Fase del plan:** A, «sin costo, en código» (`docs/seguridad/plan-nivel-bancario.md` §3.6 y §3.8)
- **Constitución:** v2.0.0 · **Estado:** **Borrador** (2026-11-21). Ninguna tarea se implementa sin aprobación (Art. I).
- **Fuente:** `docs/seguridad/plan-nivel-bancario.md` §3.6 (bitácora inmutable y guardia) y §3.8 (pruebas); estado real de `docs/seguridad/matriz-018.md` (C-10).

## 1. Objetivo y por qué

Hoy el backend registra eventos de seguridad estructurados (`security_event`, spec 018 C-02) y los alerta, pero esos registros son **logs**: rotan, viven en la VM y cualquiera con acceso a ella podría alterarlos sin dejar rastro. No existe una prueba de **quién hizo qué** en las acciones sensibles (sanciones, pagos, cambios de cuenta, accesos de admin a datos personales). Además, las pruebas de seguridad actuales cubren el comportamiento esperado, pero no las entradas hostiles que nadie imaginó, no recorren las 69 operaciones del contrato y el escaneo ZAP aislado (018 C-09) corre sin sesión, de modo que no alcanza las rutas protegidas. Por último, el control C-10 (entrega de alertas) sigue sin acreditar con evidencia de recepción real.

Esta spec define **qué** hay que agregar para cerrar esas brechas **sin costo** (Art. XIII.1) y **cómo se demuestra** que funcionan. No agrega funcionalidad visible a la app.

**Estado actual relevante**
- Existen `internal/adapter/http/middleware` (eventos de seguridad con seudónimos, `requestid`, `clientip`) y el contrato en `internal/adapter/http/contract` y `openapi.yaml` (69 operaciones).
- Swagger UI se monta solo si `DOCS_ENABLED` lo permite (development y staging); son 3 rutas.
- `zap-isolated.yml` (semanal y por PR) hace un escaneo **activo y aislado** con puerta en `High` (`scripts/zap-gate.mjs`), pero sin autenticación.
- C-10: alertas A1–A6 implementadas y probadas en Node; destinatario titular `JosephYanez27`; **faltan** el suplente, la recepción real de las seis y el simulacro. Además, el escaneo programado de GitHub corrió cada 5 a 9 horas en vez de cada 30 minutos (ver RF-20).
- Brecha conocida: `POST /bookings/{id}/reject` y `POST /bookings/{id}/cancel` no tienen prueba de 401 propia a nivel de ruta.

## 2. Historias

| # | Prio. | Historia |
|---|---|---|
| H-01 | P0 | Como responsable de seguridad, quiero un registro de auditoría que nadie pueda alterar sin que se note, para reconstruir quién hizo una acción sensible. |
| H-02 | P1 | Como persona que revisa, quiero un comando que verifique la bitácora y me diga si hubo manipulación y dónde. |
| H-03 | P1 | Como equipo, queremos que entradas malformadas nunca tumben ni confundan a los analizadores críticos. |
| H-04 | P0 | Como equipo, queremos la certeza de que **cada** operación del contrato exige sesión y respeta dueños y roles. |
| H-05 | P1 | Como equipo, queremos que ZAP pruebe también lo que solo ve quien inició sesión. |
| H-06 | P0 | Como responsable, quiero acreditar que una alerta de seguridad llega a una persona real que la atiende, también cuando el titular no está. |

## 3. Requisitos

### A. Bitácora de auditoría inmutable (plan §3.6)

| ID | Requisito |
|---|---|
| RF-01 | Existe una colección `audit_log` de **solo anexar**: la aplicación solo inserta; ningún código de la API actualiza ni borra registros. Los permisos del usuario de MongoDB de la API sobre esta colección se limitan a insertar y leer (principio de mínimo privilegio, Art. VI). |
| RF-02 | **Cadena de hash.** Cada registro incluye el hash del registro anterior y su propio hash, calculado sobre sus campos y el hash previo con un algoritmo estándar de la biblioteca estándar de Go. El primer registro apunta a un valor de génesis conocido. La inserción mantiene la cadena íntegra bajo concurrencia y con varias réplicas (Art. XIV: estado en MongoDB, no en memoria del proceso). |
| RF-03 | Se auditan **al menos**: acciones de administración; sanciones (aplicar y levantar); cierre de incidencias; confirmación y reporte de pago; cambios de cuenta (correo, contraseña, MFA, eliminación); y **accesos de administradores a datos personales** de terceras personas. |
| RF-04 | Cada registro guarda **solo**: identificador de quien actúa, acción (catálogo cerrado), identificador del objetivo, marca de tiempo UTC, `requestId` y resultado (éxito, rechazo, error). **Nunca** valores personales (correo, teléfono, dirección, texto de mensajes) ni secretos (contraseñas, tokens, códigos), conforme al Art. VI.5. |
| RF-05 | Una acción auditada y su registro son **consistentes**: si la acción tiene éxito, queda registro; si no se puede escribir el registro de una acción sensible, la acción no se da por completada (política de fallo cerrado) [NEEDS CLARIFICATION: ver pregunta 2]. |
| RF-06 | `cmd/admin` ofrece un comando de **verificación** que recorre la cadena y detecta cualquier alteración: registro modificado, eliminado, insertado fuera de orden o cadena cortada. Informa el primer punto de ruptura (posición e identificador) y termina con código distinto de cero si hay manipulación. Se puede ejecutar en la CI contra una base de pruebas y a mano contra staging, en solo lectura. |
| RF-07 | **Periodo de retención** definido y documentado para `audit_log` [NEEDS CLARIFICATION: ver pregunta 1]. Los registros no se purgan por debajo de ese periodo. |
| RF-08 | **Copia diaria a almacenamiento inmutable (WORM)**: queda **fuera de alcance a costo cero** y se marca como mejora futura de pago (fase C del plan). Mientras tanto, el comando de verificación admite exportar el **hash de la cabeza de la cadena** (sin datos) para anclarlo fuera de la base (p. ej. en el issue de la revisión o en el repositorio), de modo que un borrado de la cola también sea detectable. |
| RF-09 | La bitácora no es una API pública: no se agregan endpoints (ver §6). La lectura para revisión es solo por `cmd/admin`. |

### B. Fuzzing nativo de Go (plan §3.8)

| ID | Requisito |
|---|---|
| RF-10 | Pruebas de fuzzing con `testing.F` para: (a) decodificación de cursores de paginación; (b) validación de DTO de entrada; (c) análisis de JWT; (d) lectura del QR de entrega; (e) análisis de `X-Forwarded-For` y de la IP del cliente. Cada una con un corpus semilla versionado en `testdata/fuzz`. |
| RF-11 | Propiedades mínimas: ningún `panic`; ningún bucle ni consumo de memoria sin límite; una entrada inválida produce un error, nunca una aceptación; una entrada válida de la semilla se acepta de forma estable. |
| RF-12 | Un trabajo de CI **nocturno** ejecuta cada objetivo de fuzzing durante **≥ 10 minutos en total**, sin servicios de pago. Cualquier fallo (`crash`) **bloquea**: el caso mínimo se guarda como prueba de regresión en el corpus y la CI queda en rojo hasta corregirlo. |
| RF-13 | Los fuzzers corren como pruebas normales (solo el corpus) en `make test`, de modo que los casos de regresión se ejecutan en cada PR. |

### C. Pruebas de seguridad por ruta (plan §3.8)

| ID | Requisito |
|---|---|
| RF-14 | Las pruebas por ruta se **generan desde el contrato** (`openapi.yaml`): para **las 69 operaciones**, una prueba comprueba **401** sin sesión (o con sesión inválida o expirada) en toda operación que declare autenticación. Una operación nueva sin cobertura hace fallar la CI. |
| RF-15 | Para cada operación con recurso de dueño, una prueba comprueba **403 o 404 según el contrato** cuando otra persona (IDOR) o un rol insuficiente intenta acceder o modificar. Cada operación se clasifica en una tabla versionada (pública, autenticada, dueño, rol) que es la fuente de las pruebas. |
| RF-16 | Se cierra la brecha actual: `POST /bookings/{id}/reject` y `POST /bookings/{id}/cancel` tienen prueba de 401 propia a nivel de ruta, además de las de dueño. |
| RF-17 | Pruebas de integración para las **3 rutas de Swagger UI**: responden en development y staging y **no existen (404)** en production. |

### D. ZAP autenticado (plan §3.8)

| ID | Requisito |
|---|---|
| RF-18 | El escaneo completo de ZAP contra el entorno aislado se ejecuta **con sesión**: el trabajo crea usuarios sintéticos (persona usuaria y rol admin) en la base vacía del runner y entrega el token a ZAP sin imprimirlo. La puerta falla con cualquier alerta **High** sin excepción vigente (misma regla de `.zap/exceptions.json`, con vencimiento). Sigue sin tocar staging ni producción. |

### E. Cierre del control C-10 (plan §3.6, guardia)

| ID | Requisito |
|---|---|
| RF-19 | **Evidencia de entrega:** ejecutar `Security alerts` con entregas sintéticas **A3, A4, A5 y A6** (A1 y A2 ya tienen su evidencia o se repiten por completitud) y registrar, por cada una, que la persona destinataria **confirmó la recepción** (captura o referencia al issue, sin datos sensibles). |
| RF-20 | **Suplente de guardia con nombre** (`ALERT_ASSIGNEES` con titular y suplente), recepción confirmada por el suplente y una regla escrita de escalamiento si el titular no responde en 15 min. |
| RF-21 | **Simulacro de mesa trimestral:** acta con fecha, escenario, tiempos de detección y de respuesta, y acciones derivadas. El primero antes de declarar C-10 acreditado; la cadencia queda en el calendario de seguridad. |
| RF-22 | **Cadencia de detección.** El cron de GitHub Actions de `security-alerts.yml` se ejecutó cada 5 a 9 horas en lugar de cada 30 minutos. La spec exige que la cadencia real de detección esté **medida y documentada** y propone opciones (sin decidir): (a) activar el programador externo de bajo costo cero ya disponible (p. ej. latido o monitor de Better Stack que dispare `workflow_dispatch`); (b) mover la detección a la propia VM con un temporizador (`systemd` o `cron`) que invoque `scripts/alert-scan.sh`; (c) aceptar la cadencia degradada y documentarla como riesgo residual con su impacto en el tiempo de aviso. Hasta decidir, el informe de C-10 declara la cadencia real [NEEDS CLARIFICATION: ver pregunta 4]. |

### Requisitos no funcionales

| ID | Requisito |
|---|---|
| RNF-01 | **Sin cambios de contrato público.** No se espera ningún cambio en `openapi.yaml` ni en `api/paths/` (ni endpoints nuevos ni campos nuevos); si una corrección lo exigiera, se publica antes y se avisa a móvil (Art. IV.6). |
| RNF-02 | **Costo cero** (Art. XIII.1): solo MongoDB, la biblioteca estándar, `go test -fuzz`, GitHub Actions y las herramientas ya en uso. Sin dependencias de Go nuevas salvo justificación en `plan.md` y registro en `memoria.md` (Art. VIII.1). |
| RNF-03 | Arquitectura limpia (Art. III): el puerto de auditoría vive en `usecase`; solo el adaptador de MongoDB importa el controlador; `domain` y `usecase` no importan `mongo` ni `bson`. |
| RNF-04 | La escritura en la bitácora no degrada el p95 de las rutas auditadas más de lo medido y aceptado en el plan [NEEDS CLARIFICATION: umbral; propuesta ≤ 10 ms]. |
| RNF-05 | Cobertura ≥ 80 % en los paquetes nuevos de `domain` y `usecase` (Art. VII.1); `gofmt`, `go vet` y `golangci-lint` limpios (Art. VII.3). |
| RNF-06 | Ningún registro, informe, acta ni salida de CI contiene secretos, tokens ni datos personales (Art. VI.5); los repositorios son públicos, así que las evidencias de C-10 solo llevan identificadores, severidad, entorno y hora. |

## 4. Criterios de aceptación

| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una acción auditada (p. ej. aplicar una sanción), entonces existe un registro con actor, acción, objetivo, hora UTC, `requestId` y resultado, y **ningún** campo contiene correo, teléfono ni secretos. |
| CA-02 | Dada una cadena íntegra, entonces el comando de verificación termina con código 0 e informa el número de registros y el hash de la cabeza. |
| CA-03 | Dado un registro modificado en la base de pruebas, entonces la verificación falla con código distinto de cero e indica la posición y el identificador del primer registro roto. Lo mismo para uno eliminado del medio y uno insertado a mano. |
| CA-04 | Dado el hash de cabeza anclado fuera de la base, cuando se borran los últimos registros, entonces la verificación contra ese hash lo detecta. |
| CA-05 | Dadas dos réplicas que auditan a la vez (prueba de concurrencia), entonces la cadena queda íntegra y sin posiciones duplicadas. |
| CA-06 | Dado el código de la API, entonces ninguna ruta de código actualiza ni borra `audit_log`, y el usuario de MongoDB de la API no tiene permisos para hacerlo (prueba de integración con MongoDB real, Art. VII.2). |
| CA-07 | Dados los cinco objetivos de fuzzing, entonces cada uno tiene corpus semilla y corre sin fallos ≥ 10 min en la ejecución nocturna; el informe de la ejecución queda enlazado. |
| CA-08 | Dado un `crash` encontrado por el fuzzing, entonces la CI nocturna falla y el caso queda como regresión que se ejecuta en `make test` de cada PR. |
| CA-09 | Dadas las 69 operaciones de `openapi.yaml`, entonces cada una aparece en la tabla de clasificación y tiene su prueba de 401, 403 o IDOR según corresponda; agregar una operación sin clasificar hace fallar la CI. |
| CA-10 | Dadas `reject` y `cancel` de reservas, entonces sin sesión responden 401 y con otra persona responden lo que el contrato indica. |
| CA-11 | Dadas las 3 rutas de Swagger UI, entonces en development y staging responden 200 y en production responden 404. |
| CA-12 | Dado el escaneo ZAP autenticado, entonces recorre rutas protegidas (el informe lista URL que solo responden con sesión), y la puerta falla con una High sin excepción vigente; una prueba negativa lo demuestra. |
| CA-13 | Dadas las entregas sintéticas A3 a A6, entonces cada destinatario (titular y suplente) confirmó la recepción y la evidencia está registrada sin datos sensibles. |
| CA-14 | Dado el acta del simulacro de mesa, entonces tiene fecha, escenario, tiempos y acciones; y existe la siguiente fecha trimestral. |
| CA-15 | Dado el informe de C-10, entonces declara la cadencia real de detección medida (últimas 20 ejecuciones) y la decisión tomada sobre las opciones de RF-22. |
| CA-16 | Dada la comparación de `openapi.yaml` y `api/paths/` antes y después, entonces no hay diferencias (RNF-01). |

## 5. Fuera de alcance

- Copia diaria a almacenamiento WORM y retención legal gestionada en la nube (de pago; fase C del plan).
- Un endpoint o una pantalla para consultar la bitácora; la integración con Log Analytics o con un SIEM.
- Pentest externo, checklist ASVS firmada y caos del balanceador (otras partes de §3.8, fases E y F).
- MFA, dispositivos y sesiones fuertes (spec 019), y secretos y TLS (spec 021).
- Cambiar el motor del escaneo ZAP o escanear staging o producción de forma activa.
- Reescribir los eventos `security_event` existentes: la bitácora los **complementa** (log operativo frente a registro de auditoría con integridad).

## 6. Dependencias

- Rutas y casos de uso de cuenta (003), pagos (009), incidencias (010), sanciones y moderación (011), y de administración existentes: se instrumentan con una llamada al puerto de auditoría, sin cambiar su contrato.
- `cmd/admin` (existente) para el comando de verificación.
- `.github/workflows/zap-isolated.yml` y `scripts/zap-gate.mjs` (018 C-09); `security-alerts.yml`, `scripts/alert-scan.sh` y `scripts/notify-alert.sh` (018 C-10).
- MongoDB con usuario de aplicación de permisos limitados (`docs/servicios.md`); Redis no interviene (nunca es fuente de verdad, Art. XIV.2).
- Decisiones del equipo: suplente de guardia y cadencia de detección.

## 7. Riesgos

| Riesgo | Mitigación |
|---|---|
| Un atacante con acceso total a MongoDB recalcula toda la cadena | El hash de cabeza anclado fuera de la base (RF-08) y la copia WORM futura; a costo cero solo se detecta la alteración parcial o la que no recalcula todo |
| El fallo cerrado (RF-05) bloquea una acción legítima si la bitácora falla | Decisión explícita en la pregunta 2; alertar por A-evento y probar la degradación |
| Escritura serializada en la cadena limita la concurrencia | Volumen bajo (solo acciones sensibles); medir contra RNF-04 |
| El fuzzing consume minutos de Actions o genera falsos bloqueos | Tiempo acotado, ejecución nocturna y solo en el repositorio; los casos mínimos se versionan |
| El escaneo ZAP autenticado crea datos o dispara bloqueos por límite de tasa | Entorno efímero con datos sintéticos; límite de tasa relajado solo en el runner aislado |
| La cadencia de GitHub Actions sigue siendo irregular y retrasa los avisos | Opciones de RF-22 y declarar la cadencia real |
| Evidencias de C-10 en repositorio público filtran datos | Solo id, severidad, entorno y hora (RNF-06) |

## 8. Preguntas abiertas

1. [NEEDS CLARIFICATION: **retención** de `audit_log` (RF-07); propuesta 24 meses, sin purga automática antes.]
2. [NEEDS CLARIFICATION: política de fallo de la bitácora (RF-05): fallo cerrado en las acciones de dinero, sanciones y cuenta frente a fallo abierto con alerta; propuesta: cerrado.]
3. [NEEDS CLARIFICATION: **suplente de guardia** (RF-20): persona con nombre y su cuenta de GitHub.]
4. [NEEDS CLARIFICATION: **opción para la cadencia de detección** (RF-22): (a) disparador externo, (b) temporizador en la VM o (c) aceptar y documentar.]
5. [NEEDS CLARIFICATION: umbral de latencia aceptable al auditar (RNF-04).]
6. [NEEDS CLARIFICATION: catálogo definitivo de acciones auditadas por módulo, a confirmar con BE-A para las de cuenta y pagos.]
