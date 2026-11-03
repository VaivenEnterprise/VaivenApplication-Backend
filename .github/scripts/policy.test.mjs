import {test} from 'node:test';
import assert from 'node:assert/strict';
import {validatePR,validateFiles} from './policy.mjs';
const pr = (head='ci/SCRUM-01-SPEC-017-T001', base='develop') => ({head:{ref:head},base:{ref:base},title:'ci: configurar validaciones',body:'## Resumen\nAgrega controles de CI.\n## Validación\nPruebas de politicas correctas.\n## Seguridad\nSin secretos ni permisos nuevos.',user:{login:'developer'}});
test('acepta contribuciones, promociones y sincronizacion', () => {
  for (const [head,base] of [['feat/SCRUM-01-SPEC-01-T001','develop'],['develop','main'],['hotfix/SCRUM-01-SPEC-01-T001','main'],['main','develop']]) assert.deepEqual(validatePR(pr(head,base)),[]);
});
test('bloquea saltarse develop y nombres no conformes', () => {
  for (const [head,base] of [['feat/SCRUM-01-SPEC-01-T001','main'],['hotfix/INVALID','main'],['arbitrary','develop'],['feat/login','other']]) assert.ok(validatePR(pr(head,base)).length);
});
test('no ejecuta titulos y exige evidencia escrita', () => {
  const item=pr(); item.title='$(touch injected)'; assert.ok(validatePR(item).length);
  item.title='fix: validar sesion'; item.body='## Resumen\n<!-- pendiente -->\n## Validación\n\n## Seguridad\n'; assert.ok(validatePR(item).length);
});
test('excepcion Dependabot requiere su identidad y destino seguro', () => {
  const item=pr('dependabot/npm_and_yarn/dep-1'); item.user.login='dependabot[bot]'; item.body=''; assert.deepEqual(validatePR(item),[]);
  item.base.ref='main'; assert.ok(validatePR(item).length);
  item.base.ref='develop'; item.user.login='intruder'; assert.ok(validatePR(item).length);
});
test('bloquea credenciales y permite ejemplos sin valores reales', () => {
  assert.equal(validateFiles(['.env','.env.production','sub/private.pem','release.jks','google-services.json']).length,5);
  assert.deepEqual(validateFiles(['.env.example','.env.sample','README.md','id_ed25519.pub']),[]);
});

test('exige trazabilidad completa y mayusculas exactas', () => {
  for (const head of ['chore/SCRUM-01-SPEC-01-T001','feat/SCRUM-123-SPEC-017-T1000','hotfix/SCRUM-22-SPEC-003-T009']) assert.deepEqual(validatePR(pr(head)),[]);
  for (const head of ['chore/descripcion','spec/017-devsecops','chore/SCRUM-01-SPEC-01','chore/SCRUM-01-T001','chore/SPEC-01-T001','chore/scrum-01-spec-01-t001','Chore/SCRUM-01-SPEC-01-T001','chore/SCRUM-1-SPEC-01-T001','chore/SCRUM-01-SPEC-1-T001','chore/SCRUM-01-SPEC-01-T01','chore/SCRUM-01-SPEC-01-T001-extra','chore/SCRUM-01-SPEC-01-T001/extra']) assert.ok(validatePR(pr(head)).length,head);
  assert.ok(validatePR(pr('chore/SCRUM-01-SPEC-01-T001','main')).length);
  assert.ok(validatePR(pr('hotfix/login','main')).length);
});

