# Plan 017

Usar GitHub Actions en runners alojados para repositorios publicos, sin servicios de pago. Acciones oficiales fijadas por commit, Gitleaks v8.30.1 con checksum verificado y CodeQL v4. golangci-lint v2.15.0 y govulncheck v1.9.0 son herramientas de CI, no dependencias de la aplicacion.

Archivos previstos:

- .github/workflows/ci.yml
- .github/scripts/policy.mjs
- .github/scripts/policy.test.mjs
- .github/scripts/project.mjs
- .github/scripts/coverage.mjs
- .github/CODEOWNERS
- .github/PULL_REQUEST_TEMPLATE.md
- .github/dependabot.yml
- .github/rulesets/protected-branches.json
- SECURITY.md
- docs/devsecops.md
- memoria.md
- specs/017-devsecops/spec.md
- specs/017-devsecops/plan.md
- specs/017-devsecops/tasks.md

La proteccion real y los permisos de revisores requieren administracion; docs/devsecops.md y el ruleset JSON dejan el cambio exacto listo para esa cuenta. El PR inicial se revisa y no se fusiona automaticamente.

- .github/scripts/check-sarif.mjs bloquea hallazgos CodeQL high/critical despues del analisis.
