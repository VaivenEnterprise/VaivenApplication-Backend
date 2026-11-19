#!/usr/bin/env node
// Evaluador de alertas de seguridad (spec 018, C-10). Lee registros de la API
// (slog en texto o JSON) y eventos de la cadena de suministro, y dispara las
// seis alertas de docs/seguridad/respuesta-incidentes.md. No imprime líneas de
// registro: solo conteos y el id de la alerta.
//
//   node scripts/security-alerts.mjs <registros.log> [--restarts N] [--window-min 15]
//
// Sale con 2 si se dispara alguna alerta, 0 si no.
import { readFileSync } from 'node:fs';

export const THRESHOLDS = {
  // 1. Reutilización de refresh: la PRIMERA confirmada ya alerta (sev. alta);
  //    3 o más en la ventana escalan a crítica (posible robo masivo).
  refreshReuseEscalate: 3,
  // 2. Fallos de login en la ventana (por todos los clientes).
  loginFailures: 20,
  // 3. Proporción de 5xx en la ventana, con un mínimo de peticiones.
  serverErrorRatio: 0.05,
  serverErrorMinRequests: 50,
  // 4. Reinicios del contenedor de la API desde la última revisión.
  restarts: 2,
};

// parse lee un registro slog en texto (k=v) o JSON.
export function parse(line) {
  const s = line.trim();
  if (!s) return null;
  if (s.startsWith('{')) {
    try {
      return JSON.parse(s);
    } catch {
      return null;
    }
  }
  const out = {};
  for (const m of s.matchAll(/(\w+)=("(?:[^"\\]|\\.)*"|\S+)/g)) {
    out[m[1]] = m[2].startsWith('"') ? m[2].slice(1, -1) : m[2];
  }
  return out;
}

export function evaluate(lines, { restarts = 0 } = {}) {
  const c = { requests: 0, s5xx: 0, loginFail: 0, reuse: 0, ratelimit: 0, signature: 0, critical: 0 };
  for (const raw of lines) {
    const r = parse(raw);
    if (!r) continue;
    const ev = r.event ?? '';
    if (r.msg === 'request') {
      c.requests++;
      const status = Number(r.status);
      if (status >= 500) c.s5xx++;
      if (r.method === 'POST' && r.path === '/api/v1/auth/login' && status === 401) c.loginFail++;
    }
    if (ev === 'auth.login_failed') c.loginFail++;
    if (ev === 'auth.refresh_reuse') c.reuse++;
    if (ev === 'ratelimit.blocked') c.ratelimit++;
    if (ev === 'supply_chain.signature_failed') c.signature++;
    if (ev === 'supply_chain.critical_vulnerability') c.critical++;
  }
  const alerts = [];
  const add = (id, severity, detail) => alerts.push({ id, severity, detail });
  if (c.reuse >= 1) add('A1-refresh-reuse', c.reuse >= THRESHOLDS.refreshReuseEscalate ? 'critical' : 'high', `${c.reuse} reutilizaciones confirmadas`);
  if (c.loginFail >= THRESHOLDS.loginFailures) add('A2-login-failures', 'medium', `${c.loginFail} fallos de login`);
  if (c.requests >= THRESHOLDS.serverErrorMinRequests && c.s5xx / c.requests >= THRESHOLDS.serverErrorRatio) {
    add('A3-5xx', 'high', `${c.s5xx} de ${c.requests} respuestas 5xx`);
  }
  if (restarts >= THRESHOLDS.restarts) add('A4-restarts', 'high', `${restarts} reinicios de la API`);
  if (c.critical >= 1) add('A5-critical-vulnerability', 'critical', `${c.critical} hallazgos críticos en la cadena de suministro`);
  if (c.signature >= 1) add('A6-signature-failure', 'critical', `${c.signature} verificaciones de firma fallidas`);
  return { counts: c, alerts };
}

if (process.argv[1]?.endsWith('security-alerts.mjs')) {
  const args = process.argv.slice(2);
  const file = args.find((a) => !a.startsWith('--'));
  const ri = args.indexOf('--restarts');
  const restarts = ri >= 0 ? Number(args[ri + 1]) : 0;
  const lines = readFileSync(file ?? 0, 'utf8').split(/\r?\n/);
  const { counts, alerts } = evaluate(lines, { restarts });
  console.log(`conteos: ${JSON.stringify(counts)}`);
  for (const a of alerts) console.log(`ALERTA ${a.id} [${a.severity}]: ${a.detail}`);
  process.exit(alerts.length ? 2 : 0);
}
