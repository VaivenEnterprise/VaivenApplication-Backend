# Respaldos y restauración (spec 013, T006; RF-07, CA-04)

Respaldos **cifrados** de MongoDB con `mongodump --archive --gzip`, hechos en la VM, con copia fuera de la VM y restauración probada en staging. Todo corre sin SSH, con `scripts/cloudshell-run.sh` desde Azure Cloud Shell (Bash). Ningún script imprime contraseñas, la frase de cifrado ni credenciales.

| Qué | Dónde |
|---|---|
| Frase de cifrado | `/opt/vaiven/backup/backup.key` en la VM (600, root) **y** en el gestor de contraseñas del equipo. Nunca en git |
| Respaldos en la VM | `/opt/vaiven/backup/<entorno>/vaiven_<entorno>_<UTC>.archive.gz.enc` (+ `.sha256` y `.counts.json`); se conservan los 8 más recientes |
| Copia fuera de la VM | Cloudinary, raw **authenticated** (privado): `vaiven/backups/<entorno>/…`. Está cifrada: Cloudinary nunca ve los datos |
| Cifrado | `openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt` (plan P-05) |

## 1. Frase de cifrado (una sola vez)

En Cloud Shell. La frase se genera en tu Cloud Shell, viaja a la VM como **parámetro protegido** (Azure lo guarda cifrado y no lo devuelve) y no se sobrescribe si ya existe:

```bash
umask 077; [ -s ~/vaiven-backup.key ] || openssl rand -base64 48 > ~/vaiven-backup.key
az vm run-command create -g rg-vaiven --vm-name vm-vaiven --location westus2 \
  --name backup-key --async-execution false --timeout-in-seconds 120 \
  --protected-parameters BK="$(cat ~/vaiven-backup.key)" \
  --script 'install -d -m 0700 /opt/vaiven/backup; K=/opt/vaiven/backup/backup.key;
if [ -s "$K" ]; then echo "EXISTE (no se toca)"; else (umask 077; printf "%s\n" "$BK" > "$K"); echo "CREADA"; fi; stat -c "%a %U" "$K"'
az vm run-command show -g rg-vaiven --vm-name vm-vaiven --name backup-key --instance-view --query "instanceView.output" -o tsv
az vm run-command delete -g rg-vaiven --vm-name vm-vaiven --name backup-key --yes
```

Guarda el contenido de `~/vaiven-backup.key` en el gestor de contraseñas del equipo. **Sin la frase, los respaldos no sirven.**

## 2. Hacer un respaldo

Antes de cada despliegue a producción y una vez por semana (lunes). `<commit>` es el commit de `main` que contiene los scripts:

```bash
bash cloudshell-run.sh <commit> scripts/backup.sh production --offsite
```

Termina con `BACKUP_RESULT=ok`, el tamaño, el inicio del sha256, los conteos por colección y las dos rutas `cloudinary:` de la copia fuera de la VM. (Para obtener `cloudshell-run.sh`: `curl -fsSLO https://raw.githubusercontent.com/VaivenEnterprise/VaivenApplication-Backend/<commit>/scripts/cloudshell-run.sh`.)

## 3. Probar la restauración (CA-04)

Restaura en el MongoDB de **staging**, en la base `vaiven_restore_check` (nunca en `vaiven_staging` ni en producción), compara colecciones y conteos con el manifiesto y la borra al terminar:

```bash
# desde la copia de la VM
bash cloudshell-run.sh <commit> scripts/restore.sh /opt/vaiven/backup/production/<archivo>.enc
# desde la copia de fuera de la VM
bash cloudshell-run.sh <commit> scripts/restore.sh cloudinary:vaiven/backups/production/<archivo>.enc
```

Debe terminar con `0 diferencias` y `RESTORE_RESULT=ok`. Si un conteo difiere, sale con `RESTORE_RESULT=diferencias`; si el archivo fue alterado, falla el sha256; con una frase equivocada falla el descifrado.

## 4. Recuperación real de producción (desastre)

Solo con aprobación de la persona responsable y anotándolo en `memoria.md`:

1. Restaurar con `--keep` en una base de comprobación (paso 3) y revisar los conteos.
2. Detener la API de producción: con `scripts/deploy-production.sh` no hay paso de «detener»; se hace con `az vm run-command invoke … --scripts 'cd /opt/vaiven/production && docker compose stop api'`.
3. Copiar la base restaurada sobre `vaiven_prod` con `mongorestore --nsFrom 'vaiven_prod.*' --nsTo 'vaiven_prod.*' --drop` contra el contenedor `vaiven-production-data-mongo-1`, con la contraseña de root de `/opt/vaiven/production/data.env` en un archivo `--config` (como en `scripts/backup.sh`).
4. Arrancar la API (`docker compose start api`) y correr `scripts/smoke.sh`.

## 5. Registro

| Fecha | Entorno | Archivo | Fuera de la VM | Restauración en staging | Quién |
|---|---|---|---|---|---|
| 2026-11-16 | production | `vaiven_production_20261117T021736Z` (antes del primer despliegue, base vacía) | sí | — | BE-B |
| 2026-11-16 | production | respaldo con la cuenta de administrador | sí | ✅ desde Cloudinary: 12 colecciones, 0 diferencias | BE-B |

## 6. Antes de migrar (spec 021: quitar los `null`)

`admin migrate-remove-nulls` modifica documentos reales y no se puede deshacer sin un respaldo. **Nunca se aplica sin respaldo previo.** Solo hace `$unset` de campos `null` y `$pull` de `null` sueltos en arreglos; nunca borra documentos. Orden obligatorio:

1. **Respaldo** (paso 2 de este documento) y anotar su nombre, p. ej. `vaiven_staging_20261126T120000Z`. Comprobar que existe la copia fuera de la VM.
2. **Dry-run** (es lo que hace el comando por omisión; no escribe nada): `admin migrate-remove-nulls`. Revisar los conteos por colección y que no aparezcan líneas `NO SOPORTADA` (rutas con más de un nivel de arreglo: el comando las reporta y no las toca; salida 1; se añade una regla antes de seguir).
3. **Apply** con la referencia del respaldo: `admin migrate-remove-nulls --apply --backup-ref vaiven_staging_20261126T120000Z`. Sin `--backup-ref` el comando se niega. Con `APP_ENV=production` exige además `--yes-production` y solo se usa cuando el responsable lo indique. La salida registra el `backup-ref` y los documentos modificados por colección (sin valores ni ids).
4. **Verificar**: `admin verify-no-nulls` (solo lectura) debe salir con 0. Repetir el paso 3 no debe cambiar nada (`0 documentos modificados`; es idempotente y reanudable: si se interrumpe, se vuelve a ejecutar).
5. Solo entonces se endurecen los validadores (T030 de la spec 021) con el interruptor explícito `admin enforce-validators` (dry-run por omisión; `--apply` vuelve a verificar cero `null` y se niega si hay; con `APP_ENV=production` exige `--yes-production`; requiere un usuario con `dbAdmin`). La API **nunca** endurece sola al arrancar ni afloja un endurecimiento ya hecho. Para volver atrás: `admin enforce-validators --apply --tolerant`.

**Si algo sale mal** (conteos inesperados, error a medias, un lector que dependía de un `null`): detener la API, restaurar el respaldo del paso 1 como en el paso 4 de «Recuperación real» (con `--keep` en una base de comprobación primero si hay duda), arrancar la API y anotar el motivo en `memoria.md`. Probar siempre primero en una copia o en staging.
