#!/usr/bin/env node
// Valida .trivyignore.yaml (spec 018, C-03): cada excepción es un CVE concreto
// con motivo, responsable y vencimiento de 90 días como máximo. Sin
// dependencias: lee el subconjunto de YAML que usa ese archivo.
//
//   node scripts/check-trivyignore.mjs [.trivyignore.yaml] [AAAA-MM-DD]
import { readFileSync } from 'node:fs';

const MAX_DAYS = 90;

export function parseIgnore(text) {
  const entries = [];
  let cur = null;
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.replace(/\s+#.*$/, '').trimEnd();
    if (!line.trim() || line.trim().startsWith('#')) continue;
    const item = line.match(/^\s*-\s+(\w+):\s*(.*)$/);
    const field = line.match(/^\s+(\w+):\s*(.*)$/);
    if (item) {
      cur = { [item[1]]: unquote(item[2]) };
      entries.push(cur);
    } else if (field && cur) {
      cur[field[1]] = unquote(field[2]);
    } else if (/^(vulnerabilities|misconfigurations|secrets|licenses):/.test(line)) {
      cur = null;
    }
  }
  return entries;
}

function unquote(v) {
  return v.replace(/^["']|["']$/g, '');
}

export function problems(entries, today) {
  const out = [];
  const now = new Date(`${today}T00:00:00Z`);
  for (const e of entries) {
    const id = e.id ?? '(sin id)';
    if (!/^(CVE|GHSA|GO)-[\w-]+$/.test(e.id ?? '')) out.push(`${id}: solo se admite un identificador de vulnerabilidad concreto`);
    if (!e.statement || e.statement.length < 20) out.push(`${id}: falta el motivo (statement)`);
    else if (!/Responsable:\s*\S+/.test(e.statement)) out.push(`${id}: el motivo debe nombrar «Responsable: <rol>»`);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(e.expired_at ?? '')) {
      out.push(`${id}: falta expired_at (AAAA-MM-DD)`);
      continue;
    }
    const days = (new Date(`${e.expired_at}T00:00:00Z`) - now) / 86400000;
    if (days > MAX_DAYS) out.push(`${id}: vence en ${days} días; el máximo es ${MAX_DAYS}`);
    if (days < 0) out.push(`${id}: venció el ${e.expired_at}; corregir o renovar con nueva justificación`);
  }
  return out;
}

if (import.meta.url === `file://${process.argv[1]}` || process.argv[1]?.endsWith('check-trivyignore.mjs')) {
  const file = process.argv[2] ?? '.trivyignore.yaml';
  const today = process.argv[3] ?? new Date().toISOString().slice(0, 10);
  const found = problems(parseIgnore(readFileSync(file, 'utf8')), today);
  for (const p of found) console.error(`::error::${p}`);
  if (found.length) process.exit(1);
  console.log(`${file}: excepciones válidas`);
}
