# Tareas 001 — Servicios externos con MCP (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- Las cuentas (Atlas, Cloudinary, Render) y sus credenciales de API las crea la persona administradora a mano (ningún MCP crea cuentas). Todo lo demás lo hace `devops-mcp`.
- Esta spec no tiene tarea «Despliegue»: todavía no hay nada que desplegar (Art. XIII.4 aplica desde la 002).

- [ ] **T001** Variables locales y conexión de los tres MCP (**la hace una persona** con `devops-mcp`)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: — (manual) y `devops-mcp` · Depende de: —
  - Aceptación: variables del plan §3 definidas en el sistema (sin valores en el repo); `opencode mcp list` muestra `mongodb`, `cloudinary` y `render` conectados (CA-01); `dev-go` rechaza una herramienta MCP y `devops-mcp` la ejecuta (CA-02)
  - Refs: RF-01, RF-02 · Art. XIII

- [ ] **T002** Atlas: clúster, usuario y lista de acceso
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T001
  - Aceptación: si no existe ya un clúster gratuito en el proyecto, se crea `vaiven` (M0); usuario `vaiven_staging` con `readWrite` solo sobre `vaiven_staging`; lista de acceso `0.0.0.0/0`; contraseña generada con 24+ caracteres, **nunca escrita en el repo ni en `memoria.md`**; `MONGO_URI` guardada solo como variable local
  - Refs: RF-03 · P-06

- [ ] **T003** [P] Verificar la conexión a MongoDB
  - Archivos: `opencode.json` (agregar `MDB_MCP_CONNECTION_STRING: "{env:MONGO_URI}"` al bloque `mongodb`), `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T002
  - Aceptación: tras reiniciar OpenCode con `MONGO_URI` definida, por MCP se conecta y lista las bases de datos; no escribe ni borra datos de negocio (CA-03)
  - Refs: RF-04

- [ ] **T004** [P] Cloudinary: verificar la nube y crear la carpeta base
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T001
  - Aceptación: por MCP se confirma que la nube responde y se crea la carpeta `vaiven/staging` (CA-04); el *API secret* no se imprime en ninguna salida
  - Refs: RF-05

- [ ] **T005** [P] Render: verificar el espacio de trabajo
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T001
  - Aceptación: `select_workspace` de un espacio solo para VAIVÉN; `list_services` sin servicios previos o con `vaiven-staging` ya existente; se anota que la creación ocurre en 002-T027
  - Refs: RF-06

- [ ] **T006** `docs/servicios.md` y `.env.example`
  - Archivos: `docs/servicios.md`, `.env.example`
  - Agente: `spec-writer` · Depende de: T003, T004, T005
  - Aceptación: lista de servicios, variables (sin valores reales), cómo se obtuvo cada credencial, cómo rotarlas y riesgos conocidos (`0.0.0.0/0`, arranque en frío de Render, secretos y contexto del modelo)
  - Refs: RF-07, RNF-01

- [ ] **T007** Revisión de conformidad
  - Archivos: ninguno (solo informe)
  - Agente: `revisor-constitucion` · Depende de: T006
  - Aceptación: veredicto **CONFORME**; búsqueda de cadenas sospechosas sin hallazgos (CA-05); hito H0 (servicios) marcado
  - Refs: DoD

| Total | 7 tareas |
|---|---|
