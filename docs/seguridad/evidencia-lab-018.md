# SPEC-018 · Evidencia del laboratorio

Trabajo local sobre la instantánea de `feat/SPEC-18-aislado` en `8b96890`. Las primeras pruebas se ejecutaron cuando la SPEC-018 estaba en propuesta. B-2 quedó registrada el 18 de noviembre de 2026. La copia excluye el diagnóstico confidencial y el archivo de tareas de la SPEC-017. Los resultados del laboratorio no cambian el estado de aceptación ni de despliegue de las tareas.

## Implementado y validado

| Componente | Código y prueba | Resultado y evidencia | Riesgo residual |
|---|---|---|---|
| T001, portabilidad CRLF | La SPEC-016 corrigió el patrón en la prueba existente. Se verificaron entradas sintéticas vacías y con valor en LF y CRLF | Cuatro combinaciones en Windows y otras cuatro en Linux con red deshabilitada. La prueba Go original pasó en Windows | La prueba original necesita un checkout Git. El laboratorio sin metadatos Git acredita la portabilidad del patrón con fixtures. |
| Trivy y SBOM de la 016 | Job `Image scan and SBOM`, sin duplicar workflows de imagen | GitHub confirmó el job, el escaneo, la generación y la subida en verde, y la lista de artefactos contiene `sbom-cyclonedx`. El umbral de Trivy cubre vulnerabilidades altas y críticas con corrección disponible. [Ejecución 44236290100](https://github.com/VaivenEnterprise/VaivenApplication-Backend/actions/runs/44236290100) | Falta verificar el SBOM contra el digest desplegado y escanear antes de desplegar |
| Integración y contrato base de la 016 | Pruebas con MongoDB y Redis locales y respuestas descritas por OpenAPI | La SPEC-016 documenta 914 pruebas aprobadas en total con integración, dos omitidas, 69/69 operaciones ejercidas y cero incumplimientos. Fuente `docs/calidad/pruebas.md` | Los resultados pertenecen al árbol base de la 016 |

## Prototipo validado

| Componente | Código existente y correcciones | Prueba y resultado | Riesgo residual y aprobación |
|---|---|---|---|
| Caddy con dos réplicas | `deploy/lb-local/Caddyfile` existente. `tests/lab018-caddy.ps1` crea una red interna exclusiva y usa una imagen compilada de la instantánea | 8/8 de 16 peticiones antes de la caída, 0/12 con API1 detenida y 8/8 tras recuperarla. Cero pérdidas en las 44 peticiones de las tres ventanas. Se confirmó mediante identificadores sintéticos en los registros de ambas réplicas | PR y enmienda del Art. XIV.4 antes de integrar la arquitectura real. El laboratorio no mide fallos del nodo ni la transición inmediata entre ventanas |
| Gzip y forwarded headers | Middleware local existente. Se corrigieron los estados sin cuerpo e informativos, la precedencia del rechazo explícito de gzip sobre `*`, el tipo MIME y la exclusión de `/docs` | Pasaron las pruebas de negociación, exclusiones, concurrencia y confianza de IP. El verificador de contrato acreditó 69/69 operaciones con negociación de gzip habilitada | Gzip requiere 012-T003 y aprobación de la 018. El modelo de confianza del proxy requiere la decisión del equipo |
| Eventos | Prototipo de `ratelimit.blocked` con registro estructurado y seudónimo de red | La prueba del middleware comprueba el evento, su correlación y la ausencia de material sensible sintético | Los eventos de autorización y sesión requieren BE-A y la aprobación de la 018 |
| Revocación | Adaptador Redis y doble en memoria sin integración con autenticación. Añadí una prueba del corte monotónico con cuatro escritores concurrentes | Cuatro pruebas pasaron con Redis 7 real y `-race` en una red interna. Propagación entre clientes de 0,46 ms y p95 de 0,53 ms en 2 000 consultas con ocho lectores. La indisponibilidad devuelve error, y un corte anterior no revierte el posterior | Redis no puede convertirse en la fuente de verdad de las sesiones. T012 requiere la decisión de BE-A. El prototipo aislado no acredita revocación en rutas protegidas |

## Preparado para integrar

| Entrega | Evidencia local | Aprobación o prueba pendiente |
|---|---|---|
| IaC | `infra/main.bicep`, cuatro módulos, parámetros por entorno, `docs/infra.md` y workflow IaC. CLI Bicep 0.49.2 compiló la plantilla y los parámetros. Lint sin diagnósticos. `scripts/verify-iac.mjs` valida el ARM y rechaza tres variantes inválidas en memoria. Actionlint y Gitleaks pasaron para el workflow y Gitleaks reportó cero secretos en IaC | B-5 para inventario y `what-if`. Verificar IDs, relaciones y permisos. B-6 para costos y enmienda del Art. XIV.4 para habilitar balanceo |
| Firma, digest y reversión | `tests/security-readiness.test.mjs` verifica firmas Ed25519 efímeras y rechaza digest equivocado, etiqueta mutable, ausencia de firma y firmante, emisor o workflow incorrectos | Validar procedencia real con Cosign y OIDC tras aprobar T006 y T007. El entorno aprobado tendrá que demostrar que los fallos impiden alcanzar el paso de despliegue |
| Alertas y fallo de puerta | Tres pruebas de preparación pasaron con datos sintéticos. Incluyen positivo/negativo de las seis alertas y rechazo de severidad alta, crítica o datos inválidos | A-1 debe distinguir la primera reutilización confirmada para triage del umbral agregado. Revisar ventanas, rutas de escalamiento, fuentes reales, responsables y canales. Faltan pruebas de entrega |

## Bloqueado o sin implementar

- **ZAP baseline.** La SPEC-016 define un workflow pasivo manual. No se acreditó una corrida ni un informe. La revisión de origen exacto y la autorización específica de staging siguen pendientes.
- **DAST activo.** El laboratorio contiene comprobaciones defensivas de la puerta sobre informes sintéticos. No se implementó ni ejecutó automatización de escaneo activo.
- **Infraestructura remota.** La propuesta Bicep mantiene inactivo el LB privado en los parámetros de staging y producción. El presupuesto remoto sigue pendiente de saldo, vigencia, tarifas y aprobación. `docs/infra.md` documenta el futuro `what-if`, la revisión de operaciones y la reversión.
- **Sesiones y cabeceras.** BE-A debe aprobar los criterios complementarios de 012-T003, emisor por entorno y audiencia de la app con transición acotada a la vida del token. Para revocación, decidir alcance por token, familia y cuenta, durabilidad en MongoDB y degradación de Redis. Acotar explícitamente qué sucede con tokens anteriores y con cache ausente.

## Decisiones registradas antes de B-2

1. **Equipo:** aprobar la SPEC-018 y decidir si los criterios de Trivy/SBOM y DAST aislado amplían 016-T011 y 016-T010 o se implementan como diferencia en 018-T005 y T009.
2. **BE-A:** cerrar 012-T003 y acordar cabeceras, HSTS de staging y clasificación de respuestas. Decidir issuer/audience y estrategia de revocación antes de tocar los componentes de sesión.
3. **Equipo:** tramitar la enmienda del Art. XIV.4. Definir destino del TLS, orígenes de proxy, prueba de salud y reversión. Un LB privado requiere conectividad a la VNet y un único nodo sigue siendo un punto de fallo.
4. **Responsable Azure:** autorizar inventario de solo lectura y recopilar saldo, vencimiento, gasto base y margen restante. Un crédito nominal de USD 200 no sustituye esa autorización.

## Regresión local

- En Windows no se encontró `make`. En la copia ejecuté el equivalente de formato, lint y pruebas con cobertura. Tras corregir gzip, el formato quedó limpio, `golangci-lint run --timeout 5m` dio **0 hallazgos** y Go registró **1 149 pruebas y subpruebas aprobadas, 0 fallidas y 2 omitidas**.
- El detector de carreras en Linux pasó en `internal/adapter/http/middleware`, `internal/config` e `internal/platform/redis`. Esta corrida focalizada usó Go 1.26.11 en un contenedor local sin red y sin la etiqueta de integración.
- La corrida de Caddy usó una imagen de la instantánea previa a la corrección de gzip. Las pruebas de Go posteriores se ejecutaron con la corrección aplicada. La API no publicó puertos al host y las peticiones de prueba salieron desde Caddy en la red interna.
- La primera integración se detuvo por un fallo de compilación de la corrección local y por la selección de red de Testcontainers. Se corrigió el retorno de `mime.ParseMediaType` y la receta local selecciona explícitamente Podman. No se cambiaron archivos de autenticación.
- Las pruebas de contrato se ejecutaron desde Linux en Podman para evitar el reenvío de puertos de Windows. Las recetas locales de contrato y Redis están en `tests/lab018-contract.ps1` y `tests/lab018-redis.ps1`. Usan imágenes ya disponibles y limpian los contenedores identificados como propios.
- Pasaron las 14 suites de integración de contrato. El conteo combinado cubrió 68 operaciones hasta añadir `TestLab018HealthzContractWithGzip`. Esta prueba obtuvo HTTP 200 con el router real y verificó que la respuesta pequeña conserve `identity`. El informe `TestCoverageReport` registró 69 de 69 operaciones con cero faltantes. Los escenarios de método o ruta fuera de contrato también aparecen en la captura.
- Durante los intentos de Windows se observaron contenedores con etiquetas genéricas de Testcontainers. La limpieza se limitó a contenedores del laboratorio identificados por nombre propio o ID de sesión. El origen de los contenedores con etiquetas compartidas no quedó acreditado.

**Dictamen: MEDIO-ALTO.** Faltan controles obligatorios verificados de la SPEC-018 §6. La preparación local usó herramientas públicas y cachés locales. El trabajo no usó Azure ni inició despliegues o ejecuciones remotas.

## Continuación del 18 de noviembre de 2026

- **JWT y Redis.** El fixture invocó el verificador JWT de la API y dos clientes del almacén con Redis 7. Cinco pruebas pasaron con `-race`: validez y revocación por `jti`, corte de cuenta, JWT con expiración o llave incorrectas, falla del almacén, lectura concurrente y monotonía del corte. En la medición del almacén, el cambio se observó entre clientes en 0,15 ms y el p95 fue 0,39 ms con ocho lectores. Los endpoints reales conservan el comportamiento vigente. La [propuesta para BE-A](decision-revocacion-jwt.md) define pruebas de familia y respaldo en MongoDB antes de integrar.
- **IaC.** Bicep 0.49.2 compiló sin diagnósticos la plantilla y ambos parámetros. La elegibilidad del LB de staging exige dos IP de backend distintas. Pasaron cuatro contrapruebas del ARM, incluida una variante que intentaba autorizar un solo backend.
- **Contrato gzip.** Pasaron las 14 suites y el informe acreditó **69/69 operaciones**. La prueba de reanudación devolvía 200 en la primera corrida, pero su línea de cobertura faltó en el archivo compartido. Cambié el arnés para escribir por proceso y exigir 69/69 al consolidar. En la nueva corrida registró 486 respuestas, 94 comprimidas y una reducción del 57,0 % en bytes de cuerpo frente a esos mismos cuerpos sin comprimir. El lint con etiqueta de integración de Redis dio cero hallazgos.
- **Azure.** B-5 quedó autorizada para lectura de presupuesto, cuotas y red. Microsoft exigió renovar la sesión con MFA. El saldo y gasto real quedan pendientes. La tarifa pública verificada permite estimar USD 0,08 para un piloto de dos horas con tres horas facturables y hasta 1 GB, antes de asociados. La [propuesta de piloto](piloto-azure-load-balancer.md) fija el criterio de reversión y de aprobación. B-6, autorización de `what-if` y enmienda siguen pendientes.
- **Cliente móvil.** La [aceptación en Expo Go](aceptacion-gzip-movil.md) está preparada para MO-A y MO-B. Falta evidencia en la aplicación.

## Piloto de dos VMs y revalidación local

- **Caddy y sesiones.** La imagen del prototipo corregido repartió 8/8 peticiones antes de detener API1, 0/12 con API1 detenida y 8/8 tras recuperarla. Las 44 peticiones de las tres ventanas devolvieron respuesta. El registro se procesó en API1, el JWT permitió consultar el perfil desde API2 y el refresh se rotó en API2. El perfil respondió en API1 con el nuevo JWT. Después del logout en API1, API2 rechazó el refresh con HTTP 401. El flujo usó MongoDB y Redis compartidos en una red interna. Acredita sesiones entre procesos y conserva pendiente la integración del almacén de revocación con las rutas de autenticación.
- **Bicep y CI.** Compilaron `main.bicep`, `pilot.bicep` y los tres archivos de parámetros. Ambos lint de Bicep dieron cero diagnósticos. Cinco mutaciones de IaC del piloto fueron rechazadas: saltar la condición, convertir la VNet existente en un recurso administrado, añadir IP pública, eliminar un backend y eliminar la regla que deniega el egreso restante. `bicep snapshot` evaluó una configuración sintética habilitada y predijo cuatro recursos declarados, más el disco implícito de VM2. Cuatro variantes quedaron deshabilitadas con cero recursos: producción, IP repetida, frontend público y presupuesto sin confirmar. `actionlint` pasó. Gitleaks reportó cero secretos en `infra`, el workflow IaC y el verificador nuevo.
- **Presupuesto y autorizaciones.** B-2 ya está registrada. El reintento de B-5 al recurso real devolvió `AADSTS50076` por MFA de Azure CLI. La [propuesta de dos VMs](piloto-dos-vm-azure.md) usa un escenario público de cuatro horas y USD 3,22 con reserva antes de cargos asociados. Quedan por confirmar la factura y las cuotas con la suscripción, el canal privado, la enmienda, B-6, `what-if` y la operación concreta. El laboratorio no inició despliegues ni operaciones de escritura en Azure.

- **Formato, lint y suite global.** `gofmt -s -l .` quedó limpio. `golangci-lint run --timeout 5m` agotó el plazo durante la carga de paquetes. El diagnóstico de ocho minutos, con ocho linters activos y etiqueta `integration`, cargó paquetes en 7m20s y mostró cero hallazgos después del filtrado configurado, pero terminó con timeout y código 4. La suite global `go test ./... -race -cover` no produjo un resultado final en ninguno de los dos intentos. Sigue vigente la evidencia de la corrida completa con cobertura en Windows y del detector de carreras focalizado en Linux registrada arriba.

**Dictamen vigente: MEDIO-ALTO.** Firma OIDC, verificación por digest, alertas y revocación integrada siguen pendientes de evidencia en los flujos reales. También están pendientes las pruebas de HSTS, issuer/audience, DAST aislado y compatibilidad móvil.
