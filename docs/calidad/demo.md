# Staging de demostración (spec 016, T012)

- **Fecha:** 2026-11-17 · **Dueño:** BE-B · **Criterio:** CA-08 (búsqueda con resultados, asistente con coincidencias cercanas y recomendaciones con `reason`)
- **Estado:** **guion listo; sin recorrer en staging.** Falta cargar `seed` en staging (ver «Bloqueo») y recorrerlo con una cuenta de prueba que se borra al terminar.
- **Relación con `docs/demo.md`:** aquel es el guion completo de la presentación (spec 012). Este documento solo cubre la parte que pide la 016: dejar staging con datos suficientes y comprobar el recorrido de búsqueda, asistente y recomendaciones.
- **Reglas:**
  - nunca en producción (`seed` se niega con `APP_ENV=production`, spec 012 RF-02b);
  - sin contraseñas, tokens ni URLs firmadas en este documento ni en `memoria.md`;
  - las cuentas de prueba se dan de baja al terminar.

## 1. Situación actual de staging

| Dato | Valor | Fuente |
|---|---|---|
| Objetos activos | 3 | `memoria.md`, 015/T011 (2026-11-17) |
| Interacciones sintéticas (`seed-interactions`) | 1 517, sobre esos 3 objetos | `memoria.md`, 015/T011 |
| Cuentas del `seed` (`*.demo@vaiven.example`) | No cargadas | El `seed` nunca se corrió en staging |

Con 3 objetos, la búsqueda y las recomendaciones funcionan, pero la demostración es pobre (**DT-f**). El `seed` agrega 6 cuentas y unos 16 objetos con fotos.

## 2. Bloqueo: `seed` en staging

`seed` exige `SEED_CREDENTIALS_FILE`, un archivo `correo=contraseña` fuera del repositorio. En staging, las utilidades corren por `scripts/vm-admin.sh` en un contenedor de un solo uso, pero ese script **no monta ningún archivo de credenciales**. Por eso hoy no hay forma documentada de correr `seed` en staging.

Opciones, ninguna aplicada, porque tocan la VM y la decide el responsable:

| Opción | Qué implica | Riesgo |
|---|---|---|
| A. `vm-admin.sh` monta en el contenedor un archivo de credenciales que vive en la VM, junto al `.env` de staging (permisos 600), y define `SEED_CREDENTIALS_FILE` | Cambio pequeño de un script (lo hace `devops-mcp`) y crear el archivo en la VM sin imprimir su contenido | Bajo: solo staging; las contraseñas no entran al repositorio ni a los registros. Como los argumentos de `cloudshell-run.sh` viajan a Azure, el archivo no se pasa como argumento |
| B. Correr `seed` desde una máquina con acceso a MongoDB de staging | MongoDB no publica puertos; requeriría un túnel | Alto: abre una vía que hoy no existe (Art. XIV.4) |
| C. Crear los objetos de demostración a mano por la API con una cuenta de prueba | Sin cambios de infraestructura | Medio: lento y sin las fotos del `seed` |

**Recomendación:** opción A, en una tarea de `devops-mcp` aparte, antes de recorrer el guion.

## 3. Preparación (cuando se resuelva el bloqueo)

Desde Azure Cloud Shell, con el commit desplegado en staging (`<commit>`):

```bash
bash cloudshell-run.sh <commit> scripts/vm-admin.sh staging seed
bash cloudshell-run.sh <commit> scripts/vm-admin.sh staging seed-interactions
bash cloudshell-run.sh <commit> scripts/vm-admin.sh staging recompute-recommendations
```

- `seed` es idempotente: si las cuentas ya existen, no las duplica.
- `seed-interactions` y `recompute-recommendations` son los mismos comandos que se usaron en la 015 (T011).
- Comprobar después: `ADMIN_EXIT=0` en las tres salidas y `scripts/check-logs.sh staging` con todas las cuentas en 0.

## 4. Recorrido (CA-08)

Con una **cuenta de prueba nueva** (correo `@vaiven.invalid`), creada al empezar y dada de baja al terminar. URL base: la de staging (`STAGING_URL`).

| Paso | Petición | Resultado esperado |
|---|---|---|
| 1 | `POST /api/v1/auth/register` | 201 con tokens |
| 2 | `GET /api/v1/items?q=taladro` | 200 con al menos un resultado del `seed`; nunca objetos propios |
| 3 | `GET /api/v1/items?category=<categoría del seed>&lat=…&lng=…` | 200, ordenados por cercanía |
| 4 | `POST /api/v1/ai/assistant/messages` con «quiero arreglar una repisa este fin de semana» | 200 con necesidades, pasos y **coincidencias reales** (`matches` de cada necesidad, con objetos cercanos) |
| 5 | `POST /api/v1/recommendations/events` con `view` de dos objetos | 204 |
| 6 | `GET /api/v1/recommendations` | 200 con objetos y `reason` en cada uno |
| 7 | `DELETE /api/v1/recommendations/history` | 204 |
| 8 | `DELETE /api/v1/me` | 204; la cuenta queda dada de baja |

Cada respuesta debe cumplir el contrato: las mismas rutas se validan contra `openapi.yaml` en las pruebas de integración (T008).

## 5. Resultado

| Fecha | Commit | Pasos en verde | Notas |
|---|---|---|---|
| — | — | — | Sin recorrer: espera el `seed` en staging (sección 2) |
