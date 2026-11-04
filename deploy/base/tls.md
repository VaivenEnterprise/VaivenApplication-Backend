# deploy/base/tls.md — Emisión y renovación de certificado Let's Encrypt para la VM Azure (staging: puerto 8443, producción: puerto 443).

## Requisitos

- El puerto 80 debe estar abierto en el NSG de la VM de Azure **de forma permanente**, porque la renovación automática lo usa cada vez (plan P-03).
- El nombre DNS de la IP pública (`AZURE_HOST`, por ejemplo `<etiqueta>.<region>.cloudapp.azure.com`) ya resuelve a la VM. Es el nombre del certificado y el que la API usará para servir HTTPS.
- `certbot` ya viene instalado por `deploy/bootstrap.sh` en la VM.
- No hay ningún servidor web permanente en la VM; `certbot` usa su propio servidor HTTP temporal en el puerto 80 solo para la validación DCV (Domain Control Validation).

## Emisión

Emite el certificado utilizando `certbot` en modo **standalone**, que crea un servidor HTTP efímero en el puerto 80 exclusivamente para la validación y luego se apaga:

```bash
sudo certbot certonly --standalone \
  -d $AZURE_HOST \
  --non-interactive \
  --agree-tos \
  -m <correo-del-proyecto>
```

- `--standalone`: `certbot` arranca y usa un servidor HTTP temporal en el puerto 80; después de la validación, el servidor se detiene y no queda ningún proceso escuchando.
- `-d $AZURE_HOST`: nombre DNS de la IP pública (la única excepción de costo aceptada, ≈ 3–4 USD/mes, RNF-02).
- `--non-interactive --agree-tos -m <correo-del-proyecto>`: respuestas automáticas, sin intervención del usuario y registro de correo electrónico.

Los certificados se generan en `/etc/letsencrypt/live/$AZURE_HOST/` y contienen `fullchain.pem` (certificado + cadena intermedia) y `privkey.pem` (clave privada).

## Ubicación de los archivos

Los archivos de certificado quedan en la VM y la API (spec 002) los monta en modo **solo‑lectura**. Rutas estándar:

| Archivo | Ruta en la VM | Uso en la API |
|---|---|---|
| `fullchain.pem` | `/etc/letsencrypt/live/$AZURE_HOST/fullchain.pem` | Cadena completa (certificado + intermedia); la API sirve HTTPS con esto |
| `privkey.pem`   | `/etc/letsencrypt/live/$AZURE_HOST/privkey.pem`   | La API lo lee para servir TLS; **nunca se copia fuera de la VM ni se sube al repo** |

La API (spec 002) monta ambos archivos en solo‑lectura para servir HTTPS directamente, sin necesidad de nginx. La clave privada nunca sale de la VM.

## Cómo recarga la API el certificado

Cómo recarga la API el certificado renovado se define en la spec 002.

## Renovación automática

La renovación es automática mediante el temporizador `certbot.timer`, que el paquete de Ubuntu deja activado. Para comprobarlo:

```bash
systemctl list-timers | grep certbot
```

El temporizador ejecuta `certbot renew` diariamente, que solo intentará renovar los certificados que faltan menos de 30 días para caducar (método optimista).

**Prueba de renovación sin riesgo** (dry-run), para verificar que todo está bien antes de que venza el certificado real:

```bash
sudo certbot renew --dry-run
```

Si la salida indica "No problems detected", el certificado se renovará sin problemas el próximo día de renovación.

**Deploy hook:** para reiniciar la API después de una renovación exitosa:

```bash
sudo certbot renew --deploy-hook "docker compose -f /opt/vaiven/<proyecto>/docker-compose.yml restart api"
```

El servicio `api` y su ruta exacta se definen en la spec 002.

## Comprobación de CA-05 (validación de la cadena TLS)

Una vez emitido el certificado, se verifica que la API sirva HTTPS correctamente antes de considerar el despliegue exitoso. Los pasos son:

### a) Servidor temporal en el puerto 8443 (staging)

```bash
# En la VM, un openssl s_server temporal que se detiene solo a los 60 s
sudo setsid nohup timeout 60 openssl s_server -quiet -accept 8443 \
  -cert       /etc/letsencrypt/live/$AZURE_HOST/cert.pem \
  -cert_chain /etc/letsencrypt/live/$AZURE_HOST/chain.pem \
  -key        /etc/letsencrypt/live/$AZURE_HOST/privkey.pem \
  -www </dev/null >/dev/null 2>&1 &
```

El servidor temporal responde una página de estado de OpenSSL. Necesita `sudo` porque `privkey.pem` solo lo puede leer root.

> **Ojo:** `s_server -cert fullchain.pem` solo envía el **primer** certificado del archivo, sin la intermedia; `openssl s_client` da entonces `Verify return code: 21`, aunque `curl` en Windows pase (descarga la intermedia por su cuenta). Por eso la prueba usa `cert.pem` + `-cert_chain chain.pem`. La API en Go sí envía la cadena completa al cargar `fullchain.pem`. `setsid nohup … </dev/null` evita que la sesión SSH quede abierta mientras corre.

La validación desde otra máquina:

```bash
openssl s_client -connect $AZURE_HOST:8443 -servername $AZURE_HOST
```

La salida debe mostrar `Verify return code: 0 (ok)` y no debe haber advertencias sobre fecha inválida ni nombre incorrecto.

### b) Validación con curl sin `-k`

```bash
# Sin -k (que saltaría invalidaciones de certificado):
curl -v https://$AZURE_HOST:8443/
```

La respuesta debe validar que `curl` conecta y el certificado está bien (sin errores de TLS).

### c) Flujo completo de CA-05

1. `openssl s_server` en el 8443 (temporal; al terminar, `sudo pkill -f "openssl s_server"`).
2. `openssl s_client` conecta y valida la cadena (return code 0).
3. `curl` sin `-k` conecta y valida el certificado (sin errores de TLS).

Si alguna de estas tres falla, el certificado no está bien configurado y no se debe desplegar la API.

## Problemas comunes

| Problema | Causa | Solución |
|---|---|---|
| `certbot` falla al validar | El puerto 80 no está abierto en el NSG | Abrir regla de entrada en el NSG para el puerto 80 |
| El DNS $AZURE_HOST no resuelve | El DNS aún no se ha propagado o la IP pública no está asignada | Esperar a que el DNS se propague; verificar con `nslookup $AZURE_HOST` |
| `certbot` dice "Too many certificates already issued" | Límite de emisiones de Let's Encrypt | Probar primero con `--staging` para desarrollo; en producción, rotar dominios o esperar la ventana de reintento |
| `curl` falla con error de certificado | El certificado aún no está activo o hay mezcla de DNS | Esperar a que `certbot` termine y el certificado se propague; usar `--staging` para pruebas |

## Resumen rápido

1. `certbot certonly --standalone -d $AZURE_HOST` (puerto 80 abierto, sin servidor permanente).
2. Certificados en `/etc/letsencrypt/live/$AZURE_HOST/`.
3. API monta `fullchain.pem` y `privkey.pem` en solo‑lectura; la clave privada nunca sale de la VM.
4. Renovación automática con `certbot.timer` y validación `certbot renew --dry-run`.
5. CA-05: `openssl s_server` + `openssl s_client` + `curl` sin `-k` en el puerto 8443.