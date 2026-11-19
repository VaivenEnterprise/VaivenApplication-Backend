// Pruebas de los controles de seguridad sin dependencias (spec 018):
// excepciones de Trivy (C-03), puerta de ZAP (C-09) y activación de las seis
// alertas (C-10).  node --test tests/security-controls.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseIgnore, problems } from '../scripts/check-trivyignore.mjs';
import { evaluate as zapEvaluate } from '../scripts/zap-gate.mjs';
import { evaluate as alertEvaluate, parse } from '../scripts/security-alerts.mjs';

const today = '2026-11-18';

test('trivyignore: el archivo del repositorio es válido', () => {
  assert.deepEqual(problems(parseIgnore(readFileSync('.trivyignore.yaml', 'utf8')), today), []);
});

test('trivyignore: rechaza excepciones generales, sin dueño, vencidas o largas', () => {
  const yaml = `vulnerabilities:
  - id: CVE-2026-1111
    statement: "Sin explotación en la API. Responsable: BE-B. Se actualiza pronto."
    expired_at: 2026-12-31
  - id: HIGH
    statement: "Ocultar todo lo alto porque sí, sin más."
    expired_at: 2026-12-01
  - id: CVE-2026-2222
    statement: "Motivo largo pero sin dueño asignado aquí."
    expired_at: 2027-06-01
  - id: CVE-2026-3333
    statement: "Venció. Responsable: BE-A. Debió renovarse."
    expired_at: 2026-10-01`;
  const p = problems(parseIgnore(yaml), today);
  assert.ok(!p.some((x) => x.startsWith('CVE-2026-1111')), 'la válida pasa');
  assert.ok(p.some((x) => x.startsWith('HIGH')), 'no se ignora una severidad completa');
  assert.ok(p.some((x) => x.includes('CVE-2026-2222') && x.includes('Responsable')));
  assert.ok(p.some((x) => x.includes('CVE-2026-2222') && x.includes('90')));
  assert.ok(p.some((x) => x.includes('CVE-2026-3333') && x.includes('venció')));
});

test('ZAP: una High bloquea; una excepción vigente la libera; una vencida no', () => {
  const report = JSON.parse(readFileSync('tests/fixtures/zap/report-high.json', 'utf8'));
  assert.equal(zapEvaluate(report, [], today).ok, false);
  const ok = zapEvaluate(report, [{ pluginId: '40018', reason: 'falso positivo verificado', owner: 'BE-B', expires: '2026-12-15' }], today);
  assert.equal(ok.ok, true);
  assert.equal(ok.review.length, 1, 'la Medium queda para revisión');
  const old = zapEvaluate(report, [{ pluginId: '40018', reason: 'x', owner: 'BE-B', expires: '2026-11-01' }], today);
  assert.equal(old.ok, false);
  const clean = JSON.parse(readFileSync('tests/fixtures/zap/report-clean.json', 'utf8'));
  assert.equal(zapEvaluate(clean, [], today).ok, true);
});

const req = (method, path, status) => `time=2026-11-18T10:00:00Z level=INFO msg=request method=${method} path=${path} status=${status} durationMs=1 requestId=r`;

test('alertas: nada se dispara en tráfico normal', () => {
  const lines = Array.from({ length: 100 }, () => req('GET', '/api/v1/items', 200));
  assert.deepEqual(alertEvaluate(lines).alerts, []);
});

test('A1: la primera reutilización de refresh alerta; tres escalan a crítica', () => {
  const one = ['{"msg":"security_event","event":"auth.refresh_reuse","severity":"high"}'];
  assert.equal(alertEvaluate(one).alerts[0].id, 'A1-refresh-reuse');
  assert.equal(alertEvaluate(one).alerts[0].severity, 'high');
  assert.equal(alertEvaluate([...one, ...one, ...one]).alerts[0].severity, 'critical');
});

test('A2: fallos de login por encima del umbral (registros de petición o eventos)', () => {
  const fails = Array.from({ length: 20 }, () => req('POST', '/api/v1/auth/login', 401));
  assert.ok(alertEvaluate(fails).alerts.some((a) => a.id === 'A2-login-failures'));
  assert.ok(!alertEvaluate(fails.slice(0, 19)).alerts.length);
});

test('A3: proporción de 5xx', () => {
  const lines = [...Array.from({ length: 90 }, () => req('GET', '/x', 200)), ...Array.from({ length: 10 }, () => req('GET', '/x', 503))];
  assert.ok(alertEvaluate(lines).alerts.some((a) => a.id === 'A3-5xx'));
  assert.ok(!alertEvaluate(lines.slice(0, 49)).alerts.length, 'pocas peticiones: no alerta');
});

test('A4: reinicios del contenedor', () => {
  assert.ok(alertEvaluate([], { restarts: 2 }).alerts.some((a) => a.id === 'A4-restarts'));
  assert.equal(alertEvaluate([], { restarts: 1 }).alerts.length, 0);
});

test('A5 y A6: vulnerabilidad crítica y firma fallida', () => {
  const ids = alertEvaluate([
    'msg=security_event event=supply_chain.critical_vulnerability severity=critical',
    'msg=security_event event=supply_chain.signature_failed severity=critical',
  ]).alerts.map((a) => a.id);
  assert.deepEqual(ids, ['A5-critical-vulnerability', 'A6-signature-failure']);
});

test('parse: texto con comillas, JSON y basura', () => {
  assert.equal(parse('msg="un texto" event=x').msg, 'un texto');
  assert.equal(parse('{"event":"y"}').event, 'y');
  assert.equal(parse('{roto'), null);
  assert.equal(parse('   '), null);
});
