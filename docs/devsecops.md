# DevOps y DevSecOps

Solicitud del responsable: integrar CI, seguridad y validacion de ramas y PRs en los tres repositorios (2026-11-02). Esta integracion es independiente de las specs funcionales y no cambia la constitucion.

## Ramas y revisores

- Cambios en tipo/SCRUM-01-SPEC-01-T001 (feat, feature, fix, bugfix, hotfix, chore, ci, docs, refactor, test, perf, security, release, build o revert) hacia develop.
- Publicaciones de develop hacia main, directas o mediante una rama intermedia release/SCRUM-01-SPEC-01-T001 creada desde develop; urgencias en hotfix/SCRUM-01-SPEC-01-T001 hacia main. Despues de un release o hotfix, sincronizacion de main hacia develop.
- Titulos Conventional Commits; completar Resumen, Validación y Seguridad en el PR.
- Revisores solicitados: @JesusTrz y @JosephYanez27. JesusTrz tiene escritura; JosephYanez27 solo lectura al preparar esta configuracion y su aprobacion aun no cuenta como obligatoria. Un administrador debe concederle escritura antes de agregarlo a CODEOWNERS.
- Una aprobacion de un code owner distinto al autor, aprobacion del ultimo cambio, conversaciones resueltas y rama actualizada. Cambios nuevos invalidan aprobaciones anteriores.

## Controles

CI corre en PRs, push a main/develop, manualmente y semanalmente. Required checks agrega Policy, Quality, Secrets y CodeQL; falla si un control requerido falla o se cancela. CodeQL solo se omite cuando no hay aplicacion inicializada.

Policy verifica nombres, destino, titulo, secciones del PR y archivos sensibles. Gitleaks examina el historial alcanzable completo y los archivos actuales, con salida redactada, binario versionado y SHA-256 verificado. Las acciones se fijan por commit y Dependabot propone actualizaciones hacia develop. El token es de lectura salvo la carga de resultados de CodeQL. No se usan secretos de despliegue en PRs.

Al existir go.mod: gofmt, go vet, golangci-lint v2.15.0, pruebas con race detector, cobertura minima del 80% en domain/usecase, pruebas de integracion con etiqueta integration sobre Docker del runner, go build y govulncheck versionado. Las pruebas testcontainers deben crear su replica set MongoDB; no se usa la base de produccion.

Mientras solo exista documentacion, Quality informa que no aplican las pruebas de aplicacion; los controles de politica y secretos siguen siendo reales. Agregar codigo sin su manifiesto/verificador hace fallar CI. Dependabot para gomod empezara a trabajar cuando exista su manifiesto.

## Activacion administrativa pendiente

Los archivos de este PR no activan por si solos proteccion de ramas ni ajustes de seguridad. La conexion usada no expone administracion de repositorios.

1. Revisar y fusionar este PR en develop con sus comprobaciones en verde; promover los mismos archivos a main mediante PR develop -> main.
2. Conceder escritura a JosephYanez27 si debe ser revisor obligatorio y agregarlo a la linea de CODEOWNERS. Si el autor es JesusTrz, necesitara a JosephYanez27 o al propietario VaivenEnterprise como code owner habilitado para aprobar.
3. En Settings > Rules > Rulesets > New ruleset > Import a ruleset, importar .github/rulesets/protected-branches.json. Revisar que ambas ramas tienen el workflow antes de activarlo. Requiere Required checks proveniente de GitHub Actions, una aprobacion de code owner, ultimo cambio aprobado y conversaciones resueltas; bloquea eliminaciones, force push y push directo, sin excepciones.
4. En Settings > Code security, habilitar Dependabot alerts/security updates, secret scanning, push protection y private vulnerability reporting cuando esten disponibles. No activar CodeQL default setup porque este PR ya define advanced setup.
5. En Actions > General: token de lectura, no permitir que Actions apruebe PRs, y requerir aprobacion de workflows para contribuciones externas. Revisar ejecuciones antes de aprobar codigo externo.

## Despliegue y recuperacion

Backend y landing conservan el despliegue por MCP a Render; movil conserva Expo Go y la publicacion de la spec 013. Este cambio no crea servicios ni publica produccion. Antes de desplegar se exige CI en verde, aprobacion humana y las verificaciones de cada spec. Render no debe publicar automaticamente cada push sin estos controles. La cuenta administradora debe revisar esa configuracion cuando conecte Render al origen.

Para volver a una version anterior: PR de revert, mismos controles y despliegue por MCP del commit aprobado. No usar force push. No hay credenciales nuevas ni un workflow con acceso a produccion.

CodeQL exporta SARIF y el workflow bloquea hallazgos con security-severity >= 7 o nivel error. No basta con que la carga de resultados termine correctamente.

## Nombres de ramas con trazabilidad

Formato obligatorio para ramas de trabajo: `tipo/SCRUM-NN-SPEC-NN-TNNN`. SCRUM y SPEC requieren al menos dos digitos; la tarea al menos tres; se admiten numeros mayores. Ejemplo: `chore/SCRUM-01-SPEC-01-T001`. No se permiten sufijos ni segmentos adicionales. La validacion comprueba el formato, no la existencia del ticket en Jira.

Se conservan las promociones develop -> main, la sincronizacion main -> develop y las ramas automaticas dependabot/* solo para PRs del bot autentico hacia develop. La regla se ejecuta en Policy; para bloquear la fusion se debe exigir Required checks en la proteccion de ramas. No impide crear una rama en GitHub.

