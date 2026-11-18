# Tareas 017

- [x] Preparar politica de PRs y nombres de ramas con pruebas.
- [x] Preparar CI condicional segun el estado real del proyecto.
- [x] Preparar escaneo de secretos, CodeQL y actualizaciones de dependencias.
- [x] Documentar revisores, flujo de entrega y ruleset importable.
- [x] Verificar ejecucion de GitHub Actions en el PR: primera ejecucion de CI completada correctamente.
- [x] Revisión humana y fusión a `develop` con promoción por PR a `main`. Integración inicial en el PR #1 y promoción en el PR #2. Las actualizaciones de la SPEC-017 se promovieron a `main` en el PR #82.
- [x] Administrador: habilitar escritura del segundo revisor si corresponde y actualizar `CODEOWNERS`. GitHub confirma permiso `write` para JosephYanez27. Ambos revisores figuran en `.github/CODEOWNERS` desde `1c1dc2f`.
- [ ] Administrador: activar ruleset y ajustes de seguridad; verificar que un PR con controles fallidos no pueda fusionarse.

- [x] T009: exigir `tipo/SCRUM-NN-SPEC-NN-TNNN` en ramas de trabajo, actualizar la guía y probar nombres válidos e inválidos. Implementado en `2330e2a`. La política vigente admite `SCRUM-NN-` opcional desde `63eaf3d` y ramas por fase desde `0cf36e3`.

- [x] T010: pruebas de humo (scripts/smoke.sh), carga y fatiga con k6 (tests/load/api.js) y mutación con gremlins (scripts/mutation.sh) como workflows de GitHub Actions; ver docs/pruebas-ci.md.
