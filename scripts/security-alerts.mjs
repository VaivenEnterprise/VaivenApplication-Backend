#!/usr/bin/env node
// Evaluador de alertas de seguridad (spec 018, C-10). Lee registros de la API
// (slog en texto o JSON) y eventos de la cadena de suministro, y dispara las
// seis alertas de docs/seguridad/respuesta-incidentes.md. No imprime líneas de
// registro: solo conteos y el id de la alerta.
//
//   node scripts/security-alerts.mjs <registros.log> [--restarts N] [--window-min 15]
//
// Sale con 2 si se dispara alguna alerta, 0 si no.
import { readFileSync, writeFileSync } from 'node:fs';

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
  return { counts: c, alerts: decide(c, restarts) };
}

// decide aplica los umbrales a unos conteos (de evaluate o de
// scripts/alert-scan.sh, que cuenta en la VM sin copiar registros).
export function decide(c, restarts = 0) {
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
  return alerts;
}

// fromScan lee la línea «ALERT_COUNTS {...}» de scripts/alert-scan.sh.
export function fromScan(text) {
  const m = String(text).match(/ALERT_COUNTS (\{.*\})/);
  if (!m) throw new Error('sin ALERT_COUNTS en la salida del escaneo');
  const j = JSON.parse(m[1]);
  if (j.error) throw new Error(`escaneo: ${j.error}`);
  const num = (v) => (Number.isFinite(Number(v)) ? Number(v) : 0);
  return {
    counts: { requests: num(j.requests), s5xx: num(j.s5xx), loginFail: num(j.loginFail), reuse: num(j.reuse), ratelimit: 0, signature: 0, critical: 0 },
    restarts: num(j.restarts),
    env: String(j.env ?? ''),
  };
}

// SYNTHETIC son conteos que disparan cada alerta, para probar la entrega real
// sin tocar ningún entorno.
export const SYNTHETIC = {
  A1: { reuse: 1 },
  A2: { loginFail: 20 },
  A3: { requests: 100, s5xx: 10 },
  A4: { restarts: 2 },
  A5: { critical: 1 },
  A6: { signature: 1 },
};

export function synthetic(id) {
  const base = { requests: 0, s5xx: 0, loginFail: 0, reuse: 0, ratelimit: 0, signature: 0, critical: 0 };
  const extra = SYNTHETIC[id];
  if (!extra) throw new Error(`alerta sintética desconocida: ${id}`);
  const { restarts = 0, ...rest } = extra;
  return decide({ ...base, ...rest }, restarts);
}

// issueBody arma el texto de la notificación: solo id, severidad, conteos,
// entorno y enlace a la ejecución. Nunca líneas de registro ni datos de usuarios.
export function issueBody(alerts, { env = 'staging', runUrl = '', syntheticRun = false } = {}) {
  const lines = [
    syntheticRun ? '**Prueba sintética de entrega de alertas (C-10). No es un incidente real.**' : '**Alerta de seguridad (spec 018, C-10).**',
    '',
    `Entorno: \`${env}\``,
    '',
    '| Alerta | Severidad | Detalle |',
    '|---|---|---|',
    ...alerts.map((a) => `| ${a.id} | ${a.severity} | ${a.detail} |`),
    '',
    'Respuesta: docs/seguridad/respuesta-incidentes.md',
  ];
  if (runUrl) lines.push('', `Ejecución: ${runUrl}`);
  return lines.join('\n');
}

if (process.argv[1]?.endsWith('security-alerts.mjs')) {
  const args = process.argv.slice(2);
  const opt = (name) => {
    const i = args.indexOf(name);
    return i >= 0 ? args[i + 1] : undefined;
  };
  let alerts;
  let counts;
  if (opt('--synthetic')) {
    alerts = synthetic(opt('--synthetic'));
    counts = { synthetic: opt('--synthetic') };
  } else if (opt('--scan')) {
    const scan = fromScan(readFileSync(opt('--scan'), 'utf8'));
    counts = scan.counts;
    alerts = decide(scan.counts, scan.restarts);
  } else {
    const file = args.find((a) => !a.startsWith('--'));
    const restarts = Number(opt('--restarts') ?? 0);
    const r = evaluate(readFileSync(file ?? 0, 'utf8').split(/\r?\n/), { restarts });
    counts = r.counts;
    alerts = r.alerts;
  }
  const quiet = args.includes('--quiet');
  if (!quiet) console.log(`conteos: ${JSON.stringify(counts)}`);
  for (const a of alerts) console.log(quiet ? `ALERTA ${a.id} [${a.severity}]` : `ALERTA ${a.id} [${a.severity}]: ${a.detail}`);
  if (opt('--body-out') && alerts.length) {
    writeFileSync(opt('--body-out'), issueBody(alerts, { env: opt('--env') ?? 'staging', runUrl: opt('--run-url') ?? '', syntheticRun: Boolean(opt('--synthetic')) }));
  }
  if (opt('--summary-out') && alerts.length) {
    writeFileSync(opt('--summary-out'), alerts.map((a) => `${a.id} [${a.severity}]`).join(', '));
  }
  process.exit(alerts.length ? 2 : 0);
}
