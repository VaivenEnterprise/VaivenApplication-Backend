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
  for (const head of ['chore/descripcion','spec/017-devsecops','chore/SCRUM-01-SPEC-01','chore/SCRUM-01-T001','chore/scrum-01-spec-01-t001','Chore/SCRUM-01-SPEC-01-T001','chore/SCRUM-1-SPEC-01-T001','chore/SCRUM-01-SPEC-1-T001','chore/SCRUM-01-SPEC-01-T01','chore/SCRUM-01-SPEC-01-T001-extra','chore/SCRUM-01-SPEC-01-T001/extra']) assert.ok(validatePR(pr(head)).length,head);
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

test('SCRUM es opcional para specs, tareas y urgencias', () => {
  for (const [head,base] of [
    ['spec/SPEC-02','develop'],
    ['feat/SPEC-02-T001','develop'],
    ['feat/SPEC-02-T001','spec/SPEC-02'],
    ['feat/SCRUM-15-SPEC-02-T001','spec/SPEC-02'],
    ['fix/SPEC-002-T002','spec/SCRUM-01-SPEC-02'],
    ['develop','spec/SPEC-02'],
    ['hotfix/SPEC-02-T001','main']
  ]) assert.deepEqual(validatePR(pr(head,base)),[],head+' -> '+base);
});
test('SPEC y tarea conservan formato y limites aunque SCRUM sea opcional', () => {
  for (const [head,base] of [
    ['spec/SPEC-2','develop'],['spec/SPEC-02-T001','develop'],
    ['feat/SPEC-02','develop'],['feat/SPEC-02-T01','develop'],
    ['feat/SCRUM-1-SPEC-02-T001','develop'],
    ['spec/SCRUM-1-SPEC-02','develop'],['spec/spec-02','develop'],
    ['feat/SPEC-03-T001','spec/SPEC-02'],
    ['spec/SPEC-02','main'],['feat/SPEC-02-T001','main']
  ]) assert.ok(validatePR(pr(head,base)).length,head+' -> '+base);
});
test('release sin SCRUM conserva promocion a main', () => { assert.deepEqual(validatePR(pr('release/SPEC-02-T001','main')),[]); });

test('permite fases con SCRUM opcional hacia develop o su spec', () => {
  for (const [head,base] of [
    ['feat/SPEC-02-Fase-3','develop'],
    ['feat/SPEC-02-Fase-3','spec/SPEC-02'],
    ['docs/SCRUM-15-SPEC-002-Fase-12','spec/SPEC-02'],
    ['fix/SPEC-02-Fase-0','spec/SCRUM-01-SPEC-02']
  ]) assert.deepEqual(validatePR(pr(head,base)),[],head+' -> '+base);
});
test('fases conservan formato y no saltan la integracion', () => {
  for (const [head,base] of [
    ['feat/SPEC-02-fase-3','develop'],
    ['feat/SPEC-2-Fase-3','develop'],
    ['feat/SPEC-02-Fase-','develop'],
    ['feat/SPEC-02-Fase-3-extra','develop'],
    ['feat/SPEC-02-Fase-3-T001','develop'],
    ['feat/SPEC-02-Fase-3','spec/SPEC-03'],
    ['feat/SPEC-02-Fase-3','main'],
    ['hotfix/SPEC-02-Fase-3','main'],
    ['release/SPEC-02-Fase-3','main'],
    ['spec/SPEC-02-Fase-3','develop'],
    ['feat/SPEC-02-T001','feat/SPEC-02-Fase-3']
  ]) assert.ok(validatePR(pr(head,base)).length,head+' -> '+base);
});
test('fases requieren evidencia y aceptan titulos libres', () => {
  const item=pr('feat/SPEC-02-Fase-3'); item.title='Implementar tercera fase';
  assert.deepEqual(validatePR(item),[]);
  item.body=''; assert.ok(validatePR(item).length);
});
