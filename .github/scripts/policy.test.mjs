import {test} from 'node:test';
import assert from 'node:assert/strict';
import {validatePR,validateFiles} from './policy.mjs';
const pr = (head='ci/devsecops', base='develop') => ({head:{ref:head},base:{ref:base},title:'ci: configurar validaciones',body:'## Resumen\nAgrega controles de CI.\n## Validación\nPruebas de politicas correctas.\n## Seguridad\nSin secretos ni permisos nuevos.',user:{login:'developer'}});
test('acepta contribuciones, promociones y sincronizacion', () => {
  for (const [head,base] of [['feat/auth','develop'],['develop','main'],['hotfix/login','main'],['main','develop']]) assert.deepEqual(validatePR(pr(head,base)),[]);
});
test('bloquea saltarse develop y nombres no conformes', () => {
  for (const [head,base] of [['feat/auth','main'],['hotfix/INVALID','main'],['arbitrary','develop'],['feat/login','other']]) assert.ok(validatePR(pr(head,base)).length);
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
