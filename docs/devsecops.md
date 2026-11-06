# DevOps y DevSecOps

Solicitud del responsable: integrar CI, seguridad y validacion de ramas y PRs en los tres repositorios (2026-11-02). Esta integracion es independiente de las specs funcionales y no cambia la constitucion.

## Ramas y revisores

- Cambios en tipo/SPEC-01-T001 (feat, feature, fix, bugfix, hotfix, chore, ci, docs, refactor, test, perf, security, release, build o revert) hacia develop.
- Publicaciones de develop hacia main, directas o mediante una rama intermedia release/SPEC-01-T001 creada desde develop; urgencias en hotfix/SPEC-01-T001 hacia main. Despues de un release o hotfix, sincronizacion de main hacia develop.
- Titulos libres; completar Resumen, Validación y Seguridad en el PR.
- Revisores solicitados: @JesusTrz y @JosephYanez27. JesusTrz tiene escritura; JosephYanez27 solo lectura al preparar esta configuracion y su aprobacion aun no cuenta como obligatoria. Un administrador debe concederle escritura antes de agregarlo a CODEOWNERS.
- Una aprobacion de un code owner distinto al autor, aprobacion del ultimo cambio, conversaciones resueltas y rama actualizada. Cambios nuevos invalidan aprobaciones anteriores.

## Controles

CI corre en PRs, push a main/develop/spec/**, manualmente y semanalmente. Required checks agrega Policy, Quality, Secrets y CodeQL; falla si un control requerido falla o se cancela. CodeQL solo se omite cuando no hay aplicacion inicializada.

Policy verifica nombres, destino, secciones del PR y archivos sensibles. Gitleaks examina el historial alcanzable completo y los archivos actuales, con salida redactada, binario versionado y SHA-256 verificado. Las acciones se fijan por commit y Dependabot propone actualizaciones hacia develop. El token es de lectura salvo la carga de resultados de CodeQL. No se usan secretos de despliegue en PRs.

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

Formato para ramas de tareas: `tipo/SPEC-NN-TNNN`. SPEC requiere al menos dos digitos; la tarea al menos tres; se admiten numeros mayores. Ejemplo: `chore/SPEC-01-T001`. No se permiten sufijos ni segmentos adicionales. La validacion comprueba el formato, no la existencia del ticket en Jira.

Se conservan las promociones develop -> main, la sincronizacion main -> develop y las ramas automaticas dependabot/* solo para PRs del bot autentico hacia develop. La regla se ejecuta en Policy; para bloquear la fusion se debe exigir Required checks en la proteccion de ramas. No impide crear una rama en GitHub.


## Trabajo por spec completa

Crear `spec/SPEC-02` desde `develop`. No lleva sufijo de tarea. SCRUM y SPEC requieren al menos dos digitos, sin texto adicional.

Implementar las tareas en esa rama, una a la vez, o crear ramas de tarea desde ella y abrir sus PRs hacia la rama de spec. Por ejemplo, `feat/SPEC-02-T001` -> `spec/SPEC-02`. El segmento opcional SCRUM-NN- puede variar por tarea; el numero SPEC debe coincidir (02 y 002 son equivalentes). Los titulos de PR son libres; Resumen, Validación y Seguridad siguen siendo obligatorios.

Al completar la spec: PR de la rama de spec hacia `develop`. Para actualizarla: PR `develop` -> rama de spec. No se permite spec -> main ni integrar tareas de otra SPEC. Se conserva el flujo tarea -> develop y la promocion develop -> main.

CI se ejecuta en push y PR hacia `spec/**`, ademas de main/develop. Primero integrar esta configuracion en develop y despues crear las ramas de spec. Para una spec ya creada, incorporar estos archivos antes de usar el nuevo flujo.

Las protecciones administrativas no se activan con estos archivos. Si se quiere exigir revision y Required checks tambien en specs, un administrador debe incluir las ramas `spec/**` en un ruleset activo. Sin esa configuracion, los checks se ejecutan pero no impiden por si solos una fusion.

## Ticket SCRUM opcional

Formatos: `spec/SPEC-02` para una spec completa y `feat/SPEC-02-T001` para una tarea (con los tipos existentes). Tambien se aceptan `spec/SCRUM-01-SPEC-02` y `feat/SCRUM-15-SPEC-02-T001`. Si se incluye SCRUM, conserva mayusculas y al menos dos digitos. SPEC sigue siendo obligatorio con al menos dos digitos; T sigue siendo obligatorio en ramas de tarea con al menos tres. Los destinos permitidos, las revisiones y los controles de seguridad no cambian.

## Ramas por fase

Tambien se permite `tipo/SPEC-NN-Fase-N`, por ejemplo `feat/SPEC-02-Fase-3`, con los mismos tipos de cambio que las tareas. `SCRUM-NN-` sigue siendo opcional: `feat/SCRUM-01-SPEC-02-Fase-3`. Se escribe `Fase` exactamente asi y su numero lleva uno o mas digitos (incluido 0 si el plan tiene fase cero). SPEC mantiene al menos dos digitos.

Crear la rama de fase desde la rama de spec y abrir su PR hacia esa misma SPEC; tambien se permite fase -> develop. No se permite fase -> main, ni fase -> otra SPEC. Las ramas de fase no son destinos de PR de tareas: implementar las tareas de la fase en ella y conservar el formato por tarea para los flujos existentes. Los titulos de PR siguen siendo libres y se mantienen las secciones de evidencia y los controles de seguridad.

Las ramas de spec existentes deben incorporar esta actualizacion desde develop para validar PRs de fase con la nueva politica.
