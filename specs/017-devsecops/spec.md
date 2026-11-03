# Spec 017 — Integracion DevOps y DevSecOps

Solicitada por el responsable el 2026-11-02: configurar los tres repositorios con DevOps, DevSecOps y validacion de ramas y PRs. Revisores indicados: JesusTrz y JosephYanez27. Esta solicitud autoriza preparar la integracion; la fusion se revisa en PR.

## Aceptacion

- Cada PR ejecuta validacion de metadatos, controles de calidad aplicables, escaneo de secretos y analisis estatico.
- El estado agregado no puede aprobar si una validacion obligatoria falla o se cancela.
- Las ramas main/develop se protegen mediante reglas administradas separadamente de los archivos de CI.
- La ausencia actual de aplicacion se informa y no se presenta como pruebas funcionales ejecutadas.
- No se alteran servicios externos, credenciales, modelos de OpenCode ni specs de negocio.
- Despliegue por MCP y verificacion en Expo Go siguen las constituciones actuales.
