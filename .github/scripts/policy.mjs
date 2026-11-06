import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
import {pathToFileURL} from 'node:url';

export function validatePR(pr) {
  const errors = [];
  const head = pr.head.ref, base = pr.base.ref;
  const branch = /^(feat|feature|fix|bugfix|hotfix|chore|ci|docs|refactor|test|perf|security|release|build|revert)\/SCRUM-\d{2,}-SPEC-\d{2,}-T\d{3,}$/;
  const spec = /^spec\/SCRUM-\d{2,}-SPEC-(\d{2,})$/;
  const targetSpec = base.match(spec);
  if (!['main','develop'].includes(base) && !targetSpec)
    errors.push('El destino debe ser develop, main o spec/SCRUM-01-SPEC-02.');
  const bot = pr.user?.login === 'dependabot[bot]' && head.startsWith('dependabot/');
  if (base === 'main' && head !== 'develop' && !(/^(hotfix|release)\//.test(head) && branch.test(head)))
    errors.push('main solo recibe develop, release/SCRUM-01-SPEC-01-T001 o hotfix/SCRUM-01-SPEC-01-T001.');
  if (base === 'develop' && head !== 'main' && !branch.test(head) && !spec.test(head) && !bot)
    errors.push('Usa spec/SCRUM-01-SPEC-01 o tipo/SCRUM-01-SPEC-01-T001: tipo en minusculas, SCRUM/SPEC/T en mayusculas y todos los numeros.');
  if (targetSpec && head !== 'develop') {
    const taskSpec = branch.test(head) && head.match(/-SPEC-(\d+)-T\d+$/);
    // Los tickets SCRUM pueden diferir entre tareas; SPEC identifica la integracion.
    if (!taskSpec || BigInt(taskSpec[1]) !== BigInt(targetSpec[1]))
      errors.push('La rama de spec solo recibe tareas de la misma SPEC o sincronizacion desde develop.');
  }
  // Los titulos son texto libre; nunca se ejecutan como comandos.
  if (!bot) for (const heading of ['Resumen','Validación','Seguridad']) {
    const section = (pr.body || '').match(new RegExp('^## ' + heading + '\\s*\\n([\\s\\S]*?)(?=^## |$(?![\\s\\S]))','m'));
    const text = section?.[1].replace(/<!--[\s\S]*?-->/g,'').trim();
    if (!text || text.length < 8) errors.push('Completa la seccion ' + heading + ' del PR.');
  }
  return errors;
}

export function validateFiles(files) {
  return files.filter(file => {
    const name = file.split('/').at(-1);
    return (/^\.env(?:\.|$)/i.test(name) && !/^\.env\.(example|sample|template)$/i.test(name)) ||
      /^(id_rsa|id_ed25519)(\.pub)?$/.test(name) && !name.endsWith('.pub') ||
      /\.(p12|pfx|jks|keystore|pem|key)$/i.test(name) || /(^|\/)google-services\.json$/.test(file);
  }).map(file => 'Archivo sensible no permitido: ' + file);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const files = execFileSync('git',['ls-files','-z'],{encoding:'utf8'}).split('\0').filter(Boolean);
  const errors = validateFiles(files);
  if (process.env.GITHUB_EVENT_NAME === 'pull_request') {
    const event = JSON.parse(fs.readFileSync(process.env.GITHUB_EVENT_PATH,'utf8'));
    errors.push(...validatePR(event.pull_request));
  }
  if (errors.length) { errors.forEach(e => console.error(e)); process.exitCode = 1; }
  else console.log('Politicas de archivos, ramas y PR: OK');
}

