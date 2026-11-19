#!/usr/bin/env node
// Puerta del análisis ZAP aislado (spec 018, C-09). Lee el informe JSON de ZAP
// y falla si queda alguna alerta High (riskcode 3) sin una excepción vigente
// en .zap/exceptions.json. Las Medium se listan para revisión, sin bloquear.
//
//   node scripts/zap-gate.mjs <informe.json> [.zap/exceptions.json] [AAAA-MM-DD]
//
// Excepción: { "pluginId": "40018", "reason": "...", "owner": "BE-B",
//              "expires": "2027-01-31" } (máximo 90 días). Una excepción
// vencida no cuenta.
import { readFileSync, existsSync } from 'node:fs';

export function alerts(report) {
  const out = [];
  for (const site of report.site ?? []) {
    for (const a of site.alerts ?? []) {
      out.push({
        pluginId: String(a.pluginid),
        name: a.alert ?? a.name ?? '',
        risk: Number(a.riskcode),
        count: Number(a.count ?? (a.instances?.length ?? 0)),
      });
    }
  }
  return out;
}

export function evaluate(report, exceptions, today) {
  const now = new Date(`${today}T00:00:00Z`);
  const valid = new Map();
  const problems = [];
  for (const e of exceptions) {
    const days = (new Date(`${e.expires}T00:00:00Z`) - now) / 86400000;
    if (!e.pluginId || !e.reason || !e.owner || Number.isNaN(days)) {
      problems.push(`excepción inválida: ${JSON.stringify(e)}`);
    } else if (days < 0) {
      problems.push(`excepción vencida (${e.pluginId}, ${e.expires})`);
    } else if (days > 90) {
      problems.push(`excepción de más de 90 días (${e.pluginId})`);
    } else {
      valid.set(String(e.pluginId), e);
    }
  }
  const all = alerts(report);
  const blocking = all.filter((a) => a.risk >= 3 && !valid.has(a.pluginId));
  const review = all.filter((a) => a.risk === 2);
  return { blocking, review, problems, ok: blocking.length === 0 && problems.length === 0 };
}

if (process.argv[1]?.endsWith('zap-gate.mjs')) {
  const [reportFile, excFile = '.zap/exceptions.json', today = new Date().toISOString().slice(0, 10)] = process.argv.slice(2);
  const report = JSON.parse(readFileSync(reportFile, 'utf8'));
  const exceptions = existsSync(excFile) ? JSON.parse(readFileSync(excFile, 'utf8')) : [];
  const r = evaluate(report, exceptions, today);
  for (const a of r.review) console.log(`::warning::ZAP Medium ${a.pluginId} ${a.name} (${a.count}): revisar y registrar en deuda`);
  for (const p of r.problems) console.error(`::error::${p}`);
  for (const a of r.blocking) console.error(`::error::ZAP High ${a.pluginId} ${a.name} (${a.count}) sin excepción vigente`);
  console.log(`ZAP: ${r.blocking.length} High bloqueantes, ${r.review.length} Medium a revisar`);
  process.exit(r.ok ? 0 : 1);
}
