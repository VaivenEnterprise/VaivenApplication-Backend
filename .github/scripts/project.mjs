import fs from 'node:fs';
import path from 'node:path';
import {execFileSync, spawnSync} from 'node:child_process';
import {pathToFileURL} from 'node:url';

// Modos de Quality: 'docs' (sin proyecto), 'module' (modulo Go sin paquetes) y 'full' (todos los controles).
const EXCLUDED = /^(.github|.opencode|specs|docs)\//;
const trackedFiles = cwd => execFileSync('git', ['ls-files', '-z'], {cwd, encoding: 'utf8'}).split('\0').filter(Boolean);

// Una linea por paquete (incluye los de prueba): BAD si Go no pudo cargarlo, OK si cargo bien.
const PACKAGE_ROW = '{{if or .Error .Incomplete .DepsErrors}}BAD\t{{.ImportPath}}{{with .Error}}\t{{printf "%q" .Err}}{{end}}{{range .DepsErrors}}\t{{printf "%q" .Err}}{{end}}{{else}}OK\t{{.ImportPath}}{{end}}';

export function runGo(args, cwd) {
  const result = spawnSync('go', args, {cwd, encoding: 'utf8'});
  return {status: result.status, stdout: result.stdout || '', stderr: result.stderr || '', error: result.error};
}

// Devuelve los paquetes Go del modulo. Cualquier fallo al detectarlos o cargarlos lanza un error:
// nunca se interpreta como "modulo sin codigo".
export function listGoPackages(cwd, run = runGo) {
  const result = run(['list', '-e', '-test', '-tags=integration', '-f', PACKAGE_ROW, './...'], cwd);
  if (result.error || result.status !== 0)
    throw new Error('No se pudieron detectar los paquetes Go (go list fallo): ' + (result.error?.message || result.stderr).trim());
  const rows = result.stdout.split('\n').map(line => line.trim()).filter(Boolean).map(line => line.split('\t'));
  const bad = rows.filter(row => row[0] !== 'OK');
  if (bad.length) throw new Error('Go no pudo cargar paquetes: ' + bad.map(row => row.slice(1).join(' ')).join('; '));
  return rows.map(row => row[1]);
}

export function detect(kind, {cwd = '.', files = trackedFiles(cwd), run = runGo} = {}) {
  const appFiles = files.filter(p => !EXCLUDED.test(p));
  const source = kind === 'Backend' ? appFiles.some(p => p.endsWith('.go')) : kind === 'Mobile' ? appFiles.some(p => /\.(tsx?|jsx?)$/.test(p)) : appFiles.some(p => /\.(html|css|js)$/.test(p));
  const manifest = kind === 'Backend' ? 'go.mod' : kind === 'Mobile' ? 'package.json' : 'index.html';
  const ready = fs.existsSync(path.join(cwd, manifest));
  if (source && !ready) throw new Error('Hay codigo sin ' + manifest + '; no se permite omitir sus pruebas.');
  if (ready && kind === 'Mobile') {
    const pkg = JSON.parse(fs.readFileSync(path.join(cwd, 'package.json'), 'utf8'));
    if (!fs.existsSync(path.join(cwd, 'package-lock.json'))) throw new Error('Se requiere package-lock.json para npm ci.');
    for (const script of ['typecheck', 'lint', 'test']) if (!pkg.scripts?.[script]) throw new Error('Falta npm run ' + script);
  }
  if (ready && kind === 'Landing-Page' && !fs.existsSync(path.join(cwd, 'scripts/check.mjs'))) throw new Error('La landing debe incluir scripts/check.mjs.');
  if (kind === 'Backend' && ready) {
    if (listGoPackages(cwd, run).length) return {mode: 'full', ready: true, message: 'Proyecto inicializado: se ejecutan sus controles.'};
    if (source) throw new Error('Hay archivos .go pero Go no detecto ningun paquete (restricciones de compilacion o ubicacion); no se permite omitir sus pruebas.');
    return {
      mode: 'module', ready: false,
      message: 'Modulo Go sin codigo: go list ./... no encontro paquetes y no hay archivos .go. Se validan go.mod y sus dependencias; ' +
        'se omiten gofmt, go vet, pruebas, cobertura, integracion, go build, golangci-lint, govulncheck y CodeQL porque no aplican sin codigo.'
    };
  }
  return ready
    ? {mode: 'full', ready: true, message: 'Proyecto inicializado: se ejecutan sus controles.'}
    : {mode: 'docs', ready: false, message: 'Solo documentacion: las pruebas de aplicacion aun no aplican.'};
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const result = detect(process.argv[2]);
    if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT, 'ready=' + result.ready + '\nmode=' + result.mode + '\n');
    if (process.env.GITHUB_STEP_SUMMARY && result.mode !== 'full')
      fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, '### Quality: controles de aplicacion omitidos\n\n' + result.message + '\n\nPolicy y Secrets siguen siendo obligatorios.\n');
    console.log(result.message);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
