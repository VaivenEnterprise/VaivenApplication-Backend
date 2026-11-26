# Balanceo, compresión y revocación de JWT (spec 018, trabajo en aislamiento)

- **Fecha:** 2026-11-18 · **Rama local:** `feat/SPEC-18-aislado` (desde `develop` en `8b96890`); sin commit ni push
- **Estado de la spec 018:** **APROBADA** por el responsable el 2026-11-18 (B-2). Aun así, nada de esto se despliega hasta:
  - la autorización concreta de cada operación remota (B-5, B-6, what-if);
  - la enmienda del Art. XIV.4 (proxy o balanceador);
  - el visto bueno de BE-A para `internal/auth` (C-11, C-12).
- **Costo generado:** $0. Todo corrió en esta PC (Go, Podman). No se consultó ni se modificó Azure.

## 1. Resumen por objetivo

| Objetivo | Estado | Evidencia |
|---|---|---|
| Balanceador local con dos réplicas | **IMPLEMENTADO LOCALMENTE** y demostrado | §2 |
| Azure Standard Load Balancer | **BLOQUEADO POR APROBACIÓN Y PRESUPUESTO.** Diseño en Bicep, sin desplegar ni compilar con `az` | §3, `infra/pilot.bicep` y `infra/modules/load-balancer.bicep` (versión de Codex, validada por `.github/workflows/iac.yml`) |
| Compresión gzip | **IMPLEMENTADO LOCALMENTE** y validado en las 69 operaciones. Apagada por omisión (`HTTP_COMPRESSION_ENABLED`) | §4 |
| Revocación de JWT con Redis | **Almacén implementado y probado; integración BLOQUEADA** (necesita el visto bueno de BE-A en `internal/auth`) | §5 |
| Evento `ratelimit.blocked` (C-02, parte) | **IMPLEMENTADO LOCALMENTE** | §6 |
| Proxy de confianza para el límite por IP | **IMPLEMENTADO LOCALMENTE** (vacío por omisión: sin cambios hoy) | §2, §6 |

## 2. Balanceador local (`deploy/lb-local`, `tests/lb`)

**Topología:**
- Caddy 2 (único puerto, `127.0.0.1:18080`) delante de `api1` y `api2`, la misma imagen de la API;
- MongoDB 7 (replica set) y Redis 7 compartidos;
- las réplicas no publican puertos: solo se llega a ellas por Caddy.

**Configuración de Caddy y de la API:**
- `round_robin` sin afinidad;
- salud activa a `/healthz` cada 2 s y salud pasiva (`max_fails 1`, 10 s);
- reintento en la otra réplica si una no acepta la conexión;
- tiempos límite de lectura, escritura y conexión;
- `unhealthy_request_count 200` por réplica;
- la API solo confía en el `X-Forwarded-For` de la IP fija de Caddy (`TRUSTED_PROXIES=172.31.250.10/32`).

**Resultado** (`go test -tags=lbproto ./tests/lb/`, corrida del 2026-11-18, **PASS**):

| Prueba | Resultado |
|---|---|
| 1-2. Ambas réplicas responden y se reparte el tráfico | 40 peticiones: **20 a api1 y 20 a api2**, contadas por `X-Request-Id` en los registros de cada réplica |
| 6. JWT y refresh entre réplicas | El mismo token de acceso vale en las dos (5 y 5). Se rota en una; reutilizar el viejo da 401 y **revoca la familia** (el nuevo también da 401). Un token alterado da 401. La cuenta de prueba se dio de baja |
| 9. Concurrencia | 100 peticiones simultáneas: **100 × 200** |
| 3. Caída de una réplica | Con `api1` detenida: 30 peticiones en 22 ms, **0 fallidas**, todas en api2 |
| 4. Recuperación | Al reiniciar `api1`, vuelve a recibir tráfico (3 de 10 en la primera ventana) |
| 5 y 8. Redis compartido y `X-Forwarded-For` falso | 130 peticiones, cada una con un `X-Forwarded-For` inventado: **120 × 200 y 10 × 429**. El límite de 120/min es **uno solo para las dos réplicas** y la cabecera falsa no abre otro contador. Se registraron 10 eventos `ratelimit.blocked` |
| 7. Sin doble compresión | Caddy no comprime (sin `encode`); la API lo hace una sola vez |

**Consumo** (RSS por proceso en reposo, tras las pruebas):

| Componente | RSS |
|---|---|
| api1 | 42,6 MB |
| api2 | 26,4 MB |
| Caddy | 54,2 MB |
| Redis | 11,9 MB |
| MongoDB | 175 MB |

`podman stats` no reporta datos en esta máquina (WSL); se leyó `/proc/<pid>/status`.

**Capacidad:** dos réplicas y Caddy suman unos 120 MB por entorno. La VM actual tiene staging y producción juntos con límites de memoria por contenedor. **No se propone llevar esta arquitectura a la VM** sin medir su memoria libre real; queda como requisito de capacidad.

**Hallazgo corregido:** sin `ClientIP`, detrás de cualquier proxy todos los clientes compartirían el contador por IP del límite global y del de autenticación. Por eso un balanceador **no se puede encender** sin `TRUSTED_PROXIES`.

**Nota de Podman sobre WSL:** un puerto publicado desde una red propia no llega a Windows. La prueba se compila para Linux y corre dentro de la VM de Podman (`podman machine ssh`).

## 3. Azure Standard Load Balancer

**Alternativas:**

| Opción | ¿Balanceo real? | ¿Disponibilidad real? | Costo incremental | Veredicto |
|---|---|---|---|---|
| A. Dos backends independientes en la misma VM | Sí, entre contenedores | No: si cae la VM, cae todo | $0 en Azure, más memoria | Es el prototipo local; no necesita Azure LB |
| B. Dos VMs más Standard LB | Sí | Sí, ante la caída de una VM | Segunda VM, discos, LB e IP | **La única que cumple el objetivo.** Bloqueada |
| C. VM Scale Set | Sí | Sí | Igual o mayor que B | Excesiva para el proyecto |
| D. Standard LB delante de una sola VM | No | No | LB e IP | **Descartada:** paga un balanceador sin beneficio |

**Diseño** (`infra/pilot.bicep` y `infra/modules/load-balancer.bicep` (versión de Codex, validada por `.github/workflows/iac.yml`), opción B):
- IP pública Standard estática;
- regla TCP 443 → 8443 (TLS de extremo a extremo: lo termina cada API con su certificado);
- regla 80 → 80 para la renovación ACME;
- sonda HTTPS a `/healthz` cada 5 s, 2 fallos;
- distribución de 5-tupla, sin afinidad;
- backends sin IP pública y NSG que solo admite 8443, 80 y la sonda, con todo lo demás negado;
- `disableOutboundSnat`.

**Antes de desplegar hay que resolver:**
1. **Certificado con dos backends.** El reto HTTP-01 de `certbot --standalone` puede llegar a la VM equivocada. Se necesita DNS-01 o un certificado compartido entre las VMs.
2. **Secretos y datos.** MongoDB y Redis no pueden vivir dentro de una sola de las VMs, o esa VM sigue siendo punto único de falla. Requiere rediseñar los datos: fuera de alcance y con costo.
3. **Art. XIV.4 y XIII.6.** Hacen falta la enmienda y la aprobación del equipo.

**Presupuesto:** **no verificado.** Este trabajo no tuvo acceso de lectura a la suscripción (solo `devops-mcp` o el responsable operan Azure).

| Dato requerido | Valor |
|---|---|
| Oferta y tipo de suscripción | No consultado |
| Crédito de 200 USD vigente | No consultado |
| Saldo real disponible | No consultado |
| Fecha de expiración del crédito | No consultado |
| Gasto acumulado y proyectado | No consultado |
| Límite de gasto efectivo | No consultado |
| Costo incremental (LB Standard con 2 reglas, IP Standard, segunda VM, disco, datos procesados) | **Sin estimar con precios verificados.** Calcularlo en la calculadora de precios de Azure para la región real |

**Regla:** si el costo proyectado (actual más nuevo) supera el crédito disponible menos una reserva del 20 %, o si no se puede verificar, **no se crea nada**.

Comandos de solo lectura para quien tenga acceso (no los ejecuté):

```bash
az account show --query "{name:name, offer:subscriptionPolicies.quotaId, spendingLimit:subscriptionPolicies.spendingLimit}"
az consumption usage list --start-date 2026-11-01 --end-date 2026-11-18 --query "[].{svc:consumedService, cost:pretaxCost}" -o table
az resource list -g <rg> --query "[].{name:name, type:type, sku:sku.name}" -o table
az deployment group what-if -g <rg> -f infra/pilot.bicep -p infra/params/pilot.bicepparam
```

**Plan de apagado:** borrar el balanceador, su IP pública, el NSG nuevo y la segunda VM con su disco (`az resource delete --ids ...`), y verificar en *Cost Management* que no quedan cargos. Una alerta de presupuesto **no** detiene los cargos.

**Recursos creados en Azure:** ninguno.

## 4. Compresión gzip (`internal/adapter/http/middleware/compress.go`)

**Comportamiento:**
- gzip solo si `Accept-Encoding` lo acepta con `q > 0`;
- desde 1 KiB;
- solo JSON, texto o YAML;
- quita `Content-Length` y agrega `Vary: Accept-Encoding`;
- no comprime `HEAD`, 204, 304, lo que ya trae `Content-Encoding` ni los binarios;
- el `Flush` antes del umbral sale en claro;
- writers en `sync.Pool`, nivel 5.

**Exclusiones por seguridad** (BREACH: un secreto y texto que controla otro en la misma respuesta comprimida):
- `/api/v1/auth/*` (tokens);
- `/api/v1/me` (datos de la cuenta);
- `/api/v1/uploads/signature` (firma);
- `/delivery/qr` y `/return/qr` (token del QR);
- **toda respuesta con `Cache-Control: no-store`**.

Las listas (`/me/bookings`, búsqueda, chat, recomendaciones) sí se comprimen: no llevan secretos de autenticación.

**Clasificación de las 69 operaciones:**

| Grupo | Operaciones | Trato |
|---|---|---|
| Autenticación | 4 (`/auth/register`, `login`, `refresh`, `logout`) | Excluidas |
| Cuenta | 3 (`GET`, `PATCH` y `DELETE /me`) | Excluidas |
| Firma de subida | 1 | Excluida |
| QR | 2 (entrega y devolución) | Excluidas, más `no-store` |
| Resto | 59 | Se comprimen si pasan de 1 KiB |

**Validación** (sin regresiones):
- **Pruebas unitarias:** 11 (`compress_test.go`). Cubren sin `Accept-Encoding`, gzip, JSON idéntico tras descomprimir, paginación, errores 400, vacío y 204, pequeño, exclusiones, `no-store`, binario, ya codificado, `HEAD`, `q=0`, escrituras en trozos, `Flush` y 64 clientes concurrentes con el cliente de Go (descompresión transparente, como hacen los clientes móviles).
- **Integración completa con `CONTRACT_COMPRESS=1`:** cada operación pasa por gzip, el contrato se valida sobre el cuerpo descomprimido y cada prueba hace sus aserciones sobre ese JSON. Resultado: **935 pruebas en verde, 0 fallas, 69/69 operaciones y 0 incumplimientos del contrato**. Ninguna ruta excluida salió comprimida.

**Métricas reales** (490 respuestas de la integración):

| Medida | Valor |
|---|---|
| Respuestas comprimidas | 94 (las demás son pequeñas o están excluidas) |
| Bytes de las comprimidas | 207 697 → 47 734 (**−77 %**) |
| Bytes totales | 280 201 → 120 238 (**−57 %**) |

**Carga local** (16 clientes, 3 200 peticiones de un listado de 20 objetos, 13,7 KB):

| Modo | Tamaño | p50 | p95 |
|---|---|---|---|
| Sin gzip | 13 706 B | 2,26 ms | 3,29 ms |
| Con gzip | 589 B | 0,51 ms | 1,40 ms |

- El JSON de esa prueba es muy repetitivo; la cifra representativa es la de −77 % de arriba.
- La latencia es por loopback: en una red móvil el ahorro de bytes pesa más.

**CPU y memoria** (`BenchmarkCompress`):

| Modo | Tiempo por respuesta | Memoria |
|---|---|---|
| identity | 2,9 µs | 15,3 KB |
| gzip | 56,4 µs | 17,9 KB |

El costo es **unos 54 µs de CPU por respuesta de 13,7 KB**.

No se usó k6: no está instalado. La medición es un programa Go equivalente (`TestCompressionReport`).

**Antes de encenderla en staging:**
- coordinar con móvil (spec 018, T014);
- terminar 012-T003 (cabeceras de seguridad), la dependencia que pide la 018.

## 5. Revocación de JWT con Redis

**Implementado (sin conectar):**
- `internal/usecase/revocation` (puerto);
- `internal/platform/redis/revocation.go`, con versiones Redis y memoria.

**Claves** (con el prefijo `vaiven:<entorno>:`):

| Clave | Contenido y vencimiento |
|---|---|
| `revoke:jti:<jti>` | Logout de ese token; vence con el token |
| `revoke:user:<id>` | Corte en segundos; se rechaza todo token emitido en o antes de ese segundo. Solo sube (script Lua); vence tras la vida del token de acceso |

Nunca se guarda el token ni el refresh. Toda clave tiene TTL. La consulta es un solo `MGET`.

**Pruebas con Redis real** (`revocation_integration_test.go`, PASS):

| Requisito | Resultado |
|---|---|
| 1. Token válido | Aceptado en dos réplicas (dos clientes) |
| 2. Logout | Revocación vista por la otra réplica en **0,65 ms** (objetivo ≤ 60 s): **inmediata** |
| Otra sesión del mismo usuario | Sigue válida |
| 3. Baja de cuenta | Todo lo anterior queda rechazado en las dos réplicas |
| 7 y 9. Otros usuarios | No se ven afectados |
| Carrera de cortes | Un corte viejo no deshace uno nuevo |
| 8. Reinicio de la API | Con un cliente nuevo, **sigue revocado** |
| TTL | Toda clave tiene TTL ≤ 16 min; no se guarda lo ya vencido |
| Latencia | 2 000 consultas con 8 en paralelo: **p95 0,63 ms**. El p50 aparece en 0 por la resolución del reloj de Windows |
| 10. Redis caído | `IsRevoked` **devuelve error**; revocar también falla |

**Requisitos que dependen del middleware** (pendiente de BE-A):
- tokens vencidos o falsificados: los rechaza `security.JWT.Parse` y lo prueban las pruebas existentes de auth;
- fallo cerrado ante Redis caído (requisito 10): lo tiene que aplicar el middleware.

**Política propuesta para BE-A:**
- rutas protegidas: si el almacén no responde, **503** (fallo cerrado);
- rutas públicas: no consultan el almacén.

**Integración pendiente** (toca `internal/auth`; **BLOQUEADA** hasta el visto bueno de BE-A):

| Punto | Cambio |
|---|---|
| `Authenticate` | Tras `Parse`, `IsRevoked(sub, jti, iat)`; si hay error, 503 |
| Logout | `RevokeToken(jti, exp)` del token de acceso actual, además de revocar la familia de refresh como ya hace |
| Baja de cuenta | `RevokeUserBefore(id, ahora, accessTTL)` |
| Reutilización de refresh | Hoy revoca la familia. Para cortar también el acceso de esa familia sin cerrar otras sesiones hace falta un claim `sid` (id de familia) y una clave `revoke:fam:<sid>`. Si no, `RevokeUserBefore` cierra todas las sesiones del usuario |

Las pruebas de dos réplicas de la §2 (sesión, refresh y reutilización) ya pasan con el estado compartido en MongoDB. Con esto, el acceso revocado también quedaría compartido en Redis.

## 6. Eventos y proxy de confianza

**`security_event` en `RateLimit`:**
- campos: `event=ratelimit.blocked`, `result`, `severity`, `category` (`global`, `auth`, `chat`…), `requestId`, `subject` y marca temporal;
- `subject` es el id de usuario o la IP truncada a /24 (IPv4) o /48 (IPv6);
- una prueba comprueba que no aparecen la IP completa, `Authorization`, la contraseña ni el token.

**`authz.forbidden`** y los eventos `auth.*` viven en `internal/auth`: **bloqueados** (BE-A).

**`ClientIP`:**
- con `TRUSTED_PROXIES` vacío no hace nada (como hoy);
- con un proxy de confianza toma la dirección no confiable **más a la derecha** de `X-Forwarded-For`;
- ignora la cabecera si la conexión no viene del proxy y desconfía de valores mal formados.

## 7. Plan de reversión

| Cambio | Cómo revertir |
|---|---|
| Compresión | `HTTP_COMPRESSION_ENABLED=false` (ya es el valor por omisión); no hace falta redesplegar código |
| Proxy de confianza | `TRUSTED_PROXIES` vacío (por omisión) |
| Almacén de revocación | Nadie lo usa: no hay nada que revertir |
| Prototipo local | `podman compose -f deploy/lb-local/compose.yml -p vaiven-lb down` |
| Azure | Nada creado |

## 8. Dictamen

**MEDIO-ALTO.** No se puede declarar ALTO (spec 018 §6), porque faltan:
- **C-01:** cabeceras y HSTS (012-T003, BE-A);
- **C-11:** emisor y audiencia del JWT (BE-A);
- **C-05, C-06 y C-07:** firma, verificación obligatoria y despliegue por digest (no implementados; necesitan correr en Actions con OIDC);
- **C-09:** ZAP activo aislado (no ejecutado);
- **C-10:** alertas sin destinatarios;
- **C-02:** completo (los eventos de `internal/auth`);
- **C-12:** la revocación conectada.

**Lo que este trabajo sí deja probado:**
- compresión segura sin regresiones;
- balanceo local real con conmutación por falla;
- límite de tasa correcto detrás de un proxy;
- un almacén de revocación inmediata listo para que BE-A lo conecte.
