# Piloto de dos VMs: datos compartidos e imagen firmada en VM2

- **Fecha:** 2026-11-19 · **Estado:** análisis y diseño. **No se implementó nada** en Azure, en VM1 ni en sus NSG.
- **Hechos de partida** (lectura B-5 y repositorio):
  - VM1 (`vm-vaiven`) aloja staging **y producción**;
  - MongoDB y Redis corren en Docker **sin publicar puertos** (Art. XIV);
  - VNet `10.0.0.0/16` con una subred `10.0.0.0/24`;
  - el NSG de VM2 (Bicep del piloto) niega toda salida salvo MongoDB, Redis y DNS.
- **Regla:** cualquier opción que cambie VM1 necesita una **aprobación aparte**, porque VM1 también sirve producción.

## 1. Cómo usa VM2 MongoDB y Redis de staging

| Opción | Qué cambia | Cifrado y autenticación | Exposición | Dependencia de VM1 | Veredicto |
|---|---|---|---|---|---|
| **A. Pasarela con TLS mutuo en VM1** (un contenedor *ghostunnel* o *stunnel* en la red Docker de staging, escuchando **solo** en la IP privada de VM1, en puertos 27018 y 6380) | Un contenedor nuevo en el Compose de staging de VM1. MongoDB y Redis **no cambian** ni publican puertos | mTLS: solo un certificado de cliente emitido para VM2 entra. Detrás siguen la contraseña de MongoDB y la de Redis | Solo la IP privada. Un NSG del lado de VM1 admitiría solo `10.0.0.10` (hoy la regla por defecto `AllowVnetInBound` ya admite la VNet) | Contenedor nuevo en VM1 (staging); reversión: quitarlo | **Recomendada** |
| B. Publicar MongoDB y Redis en la IP privada de VM1 | `ports: 10.0.0.x:27017` y `6379` en el Compose de staging | Solo la autenticación de las bases; sin TLS si no se configura en MongoDB y Redis | Las bases quedan escuchando en la VNet | Cambia los servicios de datos de VM1 | Descartada: expone las bases directamente y contradice el Art. XIV |
| C. MongoDB y Redis propios en VM2 | Nada en VM1 | — | Ninguna | Ninguna | **Descartada:** no hay estado compartido, así que el logout en VM1 no revocaría en VM2. Prueba balanceo, no réplicas reales |
| D. MongoDB Atlas o Azure Cache for Redis | Servicios administrados | Sí | Endpoint privado (cuesta) o público | Migración de datos | Descartada: costo y fuera del presupuesto de USD 15 |

**Detalles de la opción A:**
- **Cadena de conexión de VM2:** `mongodb://…@<ip-vm1>:27018/?directConnection=true&tls=false`. `directConnection` evita el nombre interno `mongo:27017` que anuncia el replica set; el túnel ya cifra el tramo entre VMs.
- **Redis:** `redis://:<clave>@<ip-vm1>:6380/0` por el túnel.
- **Certificados:** una CA efímera solo para el piloto, cuya clave privada no sale del equipo de quien opera; el certificado de VM2 vence en 24 h.
- **Pruebas antes de declarar listo:**
  - desde VM2, `/readyz` responde `ready` con `redis: ok`;
  - desde otra IP de la VNet, la conexión a 27018 se rechaza por falta de certificado;
  - desde internet no hay ruta (sin IP pública ni regla).
- **Reversión:** quitar el contenedor de la pasarela y revocar la CA. MongoDB y Redis no se tocaron.

## 2. Cómo obtiene VM2 la imagen firmada

| Opción | Salida a internet de VM2 | Verificación | Costo en 4 h | Veredicto |
|---|---|---|---|---|
| **1. Relevo por Azure Storage privado** (la CI verifica la firma y guarda `docker save` del digest en un blob privado; VM2 lo descarga con un SAS de 1 h y comprueba el sha256 y el digest) | Solo a la etiqueta de servicio `Storage.WestUS2` en 443 (regla del NSG **de VM2**, recurso nuevo del piloto) | La firma se verifica en la CI, como hoy en `deploy-staging`. VM2 comprueba el sha256 del archivo y el `RepoDigest` | Centavos (almacenamiento LRS y operaciones) | **Recomendada** |
| 2. Descargar de GHCR desde VM2 | Abrir 443 hacia los rangos de GitHub (lista grande y cambiante) y una salida NAT | VM2 puede ejecutar `cosign verify` (necesita salida a Sigstore) | NAT Gateway ≈ USD 0,045/h más el tráfico | Viable, pero amplía la salida |
| 3. Relevo en VM1 (`docker save` por la red privada) | Ninguna | Igual que la 1 | $0 | Cambia VM1 y deja una ruta de VM1 a VM2: requiere aprobación |
| 4. Azure Container Registry con endpoint privado | Solo al registro | Firma en el registro | Premium para endpoint privado: fuera de presupuesto | Descartada |

**Docker en VM2:** sin salida a internet, `apt` no funciona. Con la opción 1, el mismo blob lleva los `.deb` de Docker (de los repositorios oficiales, con sus sha256), o se usa una imagen de Ubuntu con Docker preinstalado (*Marketplace*), que se evalúa en el what-if.

**Cómo queda la salida de VM2:**
- MongoDB y Redis solo por la pasarela de VM1;
- DNS de la plataforma;
- `Storage.WestUS2` en 443 durante la instalación;
- nada más.

## 3. Lo que necesita aprobación antes de crear

| # | Cambio | Dónde | Quién aprueba |
|---|---|---|---|
| 1 | Contenedor de pasarela mTLS (opción A) en el Compose de **staging** | VM1 | Responsable y equipo (toca la VM de producción) |
| 2 | Regla de salida del NSG de VM2 a `Storage.WestUS2:443` | Recurso nuevo del piloto (Bicep) | Incluido en el what-if |
| 3 | Cuenta de Storage del piloto (blob privado, borrado al final) | Recurso nuevo | Presupuesto B-6 |

Sin el punto 1, el piloto solo puede probar el balanceador con VM2 sin datos: la sonda `/readyz` de VM2 fallaría y **no** habría dos réplicas reales.

## 4. Puntos únicos de falla que siguen

Aun con dos VMs, **no hay alta disponibilidad completa**:
- MongoDB y Redis siguen en VM1;
- un solo replica set de un nodo;
- la IP pública y el DNS apuntan a VM1;
- el balanceador del piloto es interno.

El piloto demuestra el balanceo y la tolerancia a la caída de la **API**, no de los datos.

## 5. Costos del piloto (máximo USD 15)

Últimos datos de facturación vistos por el responsable (portal, 2026-11-18):
- crédito de ≈ USD 198;
- vencimiento en 14 días (≈ 2026-12-02);
- gasto del mes de ≈ USD 1,8.

Son **datos históricos**: el día de la creación se vuelven a leer en el portal.

| Concepto (West US 2, tarifa pública) | 4 h (reservando 5 h facturables) |
|---|---|
| VM2 `Standard_B2ats_v2` (0,0094 USD/h) | ≈ 0,05 |
| Disco E4 de 32 GiB (2,40 USD/mes, prorrateado) | ≈ 0,02 |
| Standard Load Balancer interno (0,025 USD/h más 0,005 USD/GB) | ≈ 0,13 |
| Storage del relevo (LRS, < 1 GB, operaciones) | < 0,05 |
| Contingencia | 3,00 |
| **Total estimado** | **≈ 3,25** |

**Lo que sigue facturando si no se borra:**
- con VM2 solo detenida: el disco (2,40 USD/mes) y el balanceador (≈ 18 USD/mes);
- por eso el retiro **borra** el balanceador, VM2, su disco, su NIC, su NSG y el Storage, y lo confirma en *Cost Management*.

**Regla:** si el día de la creación no se puede leer el saldo o el costo proyectado supera el crédito disponible menos el 20 %, no se crea nada.
