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

// --- C-10: escaneo, modo sintético y entrega (spec 018) ---
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, readFileSync as readF, chmodSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fromScan, synthetic, issueBody, SYNTHETIC } from '../scripts/security-alerts.mjs';

test('C-10: el escaneo de la VM se evalúa por conteos', () => {
  const ok = fromScan('ruido\nALERT_COUNTS {"env":"staging","requests":120,"s5xx":0,"loginFail":2,"reuse":0,"restarts":0}');
  assert.equal(ok.env, 'staging');
  assert.equal(ok.counts.requests, 120);
  const bad = fromScan('ALERT_COUNTS {"requests":100,"s5xx":10,"loginFail":0,"reuse":1,"restarts":3}');
  assert.equal(bad.counts.reuse, 1);
  assert.throws(() => fromScan('sin conteos'));
  assert.throws(() => fromScan('ALERT_COUNTS {"error":"no existe el contenedor de la API"}'));
});

test('C-10: cada alerta sintética se dispara y el cuerpo no lleva datos sensibles', () => {
  for (const id of Object.keys(SYNTHETIC)) {
    const alerts = synthetic(id);
    assert.equal(alerts.length, 1, id);
    assert.ok(alerts[0].id.startsWith(id), id);
    const body = issueBody(alerts, { env: 'staging', runUrl: 'https://example.invalid/r/1', syntheticRun: true });
    assert.match(body, /Prueba sintética/);
    assert.doesNotMatch(body, /@|Bearer|password|refreshToken|accessToken|eyJ/i, id);
  }
  assert.throws(() => synthetic('A9'));
});

// fakeGh simula la CLI de GitHub: registra cada llamada y responde según el
// escenario (issue abierto existente, fallas temporales).
function fakeGh({ existing = '', failFirst = 0 } = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'gh-'));
  const slash = (p) => p.split(String.fromCharCode(92)).join('/');
  const log = slash(join(dir, 'calls.log'));
  const counter = slash(join(dir, 'fails'));
  writeFileSync(counter, String(failFirst));
  const gh = join(dir, 'gh');
  writeFileSync(gh, [
    '#!/usr/bin/env bash',
    `echo "$*" >> "${log}"`,
    `n=$(cat "${counter}"); if [ "$n" -gt 0 ] && [ "$1 $2" = "issue create" ]; then echo $((n-1)) > "${counter}"; echo "HTTP 502" >&2; exit 1; fi`,
    `[ "$1 $2" = "issue list" ] && { echo "${existing}"; exit 0; }`,
    '[ "$1 $2" = "issue create" ] && echo "https://github.com/o/r/issues/7"',
    'exit 0',
    '',
  ].join('\n'));
  chmodSync(gh, 0o755);
  return { dir, log };
}

function notify(env, scenario) {
  const f = fakeGh(scenario);
  const r = spawnSync('bash', ['scripts/notify-alert.sh'], {
    env: {
      ...process.env,
      PATH: `${f.dir}${process.platform === 'win32' ? ';' : ':'}${process.env.PATH}`,
      GITHUB_REPOSITORY: 'o/r', GITHUB_STEP_SUMMARY: '', ALERT_WEBHOOK_URL: '', RETRY_BASE_SECONDS: '0',
      ALERT_SUMMARY: 'A1-refresh-reuse [high]', ALERT_ENV: 'staging',
      ...env,
    },
    encoding: 'utf8',
  });
  let calls = '';
  try { calls = readF(f.log, 'utf8'); } catch { /* sin llamadas */ }
  return { code: r.status, out: r.stdout + r.stderr, calls };
}

test('C-10: sin destinatarios el control queda NO operativo', () => {
  const r = notify({ ALERT_ASSIGNEES: '' });
  assert.equal(r.code, 3);
  assert.match(r.out, /C-10 no operativo/);
  assert.equal(r.calls, '', 'no crea ningún issue');
});

test('C-10: abre un issue por alerta, asignado y sin datos sensibles', () => {
  const r = notify({ ALERT_ASSIGNEES: 'JosephYanez27', ALERT_SUMMARY: 'A1-refresh-reuse [high], A3-5xx [high]' });
  assert.equal(r.code, 0, r.out);
  // El cuerpo tiene saltos de línea: se analiza el registro completo.
  assert.equal(r.calls.split('issue create').length - 1, 2);
  assert.match(r.calls, /--title Alerta de seguridad A1-refresh-reuse \(staging\)/);
  assert.match(r.calls, /--title Alerta de seguridad A3-5xx \(staging\)/);
  assert.equal(r.calls.split('--label security-alert --assignee JosephYanez27').length - 1, 2);
  // El cuerpo público no lleva conteos, IP, usuarios ni tokens.
  assert.doesNotMatch(r.calls, /reutilizaciones|requests|\d+\.\d+\.\d+\.\d+|@|Bearer|eyJ/);
});

test('C-10: una alerta repetida comenta el issue abierto en lugar de duplicarlo', () => {
  const r = notify({ ALERT_ASSIGNEES: 'JosephYanez27' }, { existing: '12' });
  assert.equal(r.code, 0, r.out);
  assert.match(r.calls, /issue list .*--label security-alert/);
  assert.match(r.calls, /issue comment 12/);
  assert.doesNotMatch(r.calls, /issue create/);
});

test('C-10: la prueba sintética siempre abre un issue nuevo marcado como prueba', () => {
  const r = notify({ ALERT_ASSIGNEES: 'JosephYanez27', ALERT_SYNTHETIC: 'true' }, { existing: '12' });
  assert.equal(r.code, 0, r.out);
  assert.doesNotMatch(r.calls, /issue list/);
  assert.match(r.calls, /issue create .*\[PRUEBA\] Alerta sintética A1-refresh-reuse/);
});

test('C-10: reintenta ante una falla temporal de GitHub', () => {
  const r = notify({ ALERT_ASSIGNEES: 'JosephYanez27' }, { failFirst: 2 });
  assert.equal(r.code, 0, r.out);
  assert.equal(r.calls.split('\n').filter((l) => l.startsWith('issue create')).length, 3);
  const bad = notify({ ALERT_ASSIGNEES: 'JosephYanez27' }, { failFirst: 5 });
  assert.notEqual(bad.code, 0, 'tras 3 fallas no finge éxito');
});

test('C-10: rechaza destinatarios y entradas inválidas', () => {
  assert.equal(notify({ ALERT_ASSIGNEES: 'alguien@correo.com' }).code, 2);
  assert.equal(notify({ ALERT_ASSIGNEES: 'JosephYanez27', ALERT_SUMMARY: 'rm -rf / [high]' }).code, 2);
  assert.equal(notify({ ALERT_ASSIGNEES: 'JosephYanez27', ALERT_ENV: 'prod; x' }).code, 2);
});
