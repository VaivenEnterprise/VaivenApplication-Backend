import {test} from 'node:test';
import assert from 'node:assert/strict';
import {validatePR,validateFiles} from './policy.mjs';
const pr = (head='ci/SCRUM-01-SPEC-017-T001', base='develop') => ({head:{ref:head},base:{ref:base},title:'ci: configurar validaciones',body:'## Resumen\nAgrega controles de CI.\n## Validación\nPruebas de politicas correctas.\n## Seguridad\nSin secretos ni permisos nuevos.',user:{login:'developer'}});
test('acepta contribuciones, promociones y sincronizacion', () => {
  for (const [head,base] of [['feat/SCRUM-01-SPEC-01-T001','develop'],['develop','main'],['hotfix/SCRUM-01-SPEC-01-T001','main'],['release/SCRUM-01-SPEC-01-T011','main'],['main','develop']]) assert.deepEqual(validatePR(pr(head,base)),[]);
});
test('bloquea saltarse develop y nombres no conformes', () => {
  for (const [head,base] of [['feat/SCRUM-01-SPEC-01-T001','main'],['hotfix/INVALID','main'],['release/v1','main'],['release/SCRUM-01-SPEC-01','main'],['arbitrary','develop'],['feat/login','other']]) assert.ok(validatePR(pr(head,base)).length);
});
test('acepta titulos libres y conserva las validaciones de rama y evidencia', () => {
  for (const title of ['MCP Cloudinary', 'Inicializar módulo Go — tarea 1', 'feature(SPEC-02): estructura', '$(touch injected)']) {
    const item=pr(); item.title=title; assert.deepEqual(validatePR(item),[]);
    item.head.ref='rama-invalida'; assert.ok(validatePR(item).length);
  }
  const item=pr(); item.title='MCP Cloudinary'; item.body='## Resumen\n<!-- pendiente -->\n## Validación\n\n## Seguridad\n'; assert.ok(validatePR(item).length);
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


test('permite specs completas, tareas de la misma spec y sincronizacion', () => {
  for (const [head,base] of [
    ['spec/SCRUM-01-SPEC-02','develop'],
    ['feat/SCRUM-02-SPEC-02-T001','spec/SCRUM-01-SPEC-02'],
    ['docs/SCRUM-03-SPEC-002-T002','spec/SCRUM-01-SPEC-02'],
    ['develop','spec/SCRUM-01-SPEC-02']
  ]) assert.deepEqual(validatePR(pr(head,base)),[],head+' -> '+base);
});
test('rechaza specs mal nombradas, cruces de spec y saltos a main', () => {
  for (const [head,base] of [
    ['spec/SCRUM-01-SPEC-02','main'],
    ['spec/SCRUM-1-SPEC-02','develop'],
    ['spec/SCRUM-01-SPEC-2','develop'],
    ['spec/SCRUM-01-SPEC-02-T001','develop'],
    ['spec/SCRUM-01-SPEC-02-extra','develop'],
    ['feat/SCRUM-01-SPEC-03-T001','spec/SCRUM-01-SPEC-02'],
    ['arbitrary','spec/SCRUM-01-SPEC-02'],
    ['main','spec/SCRUM-01-SPEC-02'],
    ['spec/SCRUM-01-SPEC-03','spec/SCRUM-01-SPEC-02'],
    ['feat/SCRUM-01-SPEC-02-T001','spec/invalid']
  ]) assert.ok(validatePR(pr(head,base)).length,head+' -> '+base);
});
test('las specs conservan evidencia requerida y no admiten excepcion del bot', () => {
  const item=pr('spec/SCRUM-01-SPEC-02'); item.body=''; assert.ok(validatePR(item).length);
  const bot=pr('dependabot/npm_and_yarn/dep-1','spec/SCRUM-01-SPEC-02');
  bot.user.login='dependabot[bot]'; bot.body=''; assert.ok(validatePR(bot).length);
});
