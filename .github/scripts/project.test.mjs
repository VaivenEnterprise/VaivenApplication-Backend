import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {detect, listGoPackages, runGo} from './project.mjs';

// Estas pruebas usan el compilador real de Go: es quien decide que es un paquete valido.
// Sin red ni descargas de toolchain: los modulos de prueba no tienen dependencias.
process.env.GOTOOLCHAIN = 'local';
process.env.GOPROXY = 'off';
process.env.GOFLAGS = '';
process.env.GOWORK = 'off';
const GO_MOD = 'module example.com/fixture\n\ngo 1.21\n';
const script = path.join(path.dirname(fileURLToPath(import.meta.url)), 'project.mjs');
const hasGo = !spawnSync('go', ['version']).error;
const needsGo = {skip: !hasGo && !process.env.CI && 'go no esta instalado'};

// Crea un proyecto temporal con los archivos indicados y devuelve su ruta y la lista de archivos.
function fixture(content) {
  const cwd = fs.mkdtempSync(path.join(os.tmpdir(), 'vaiven-ci-'));
  for (const [file, text] of Object.entries(content)) {
    fs.mkdirSync(path.dirname(path.join(cwd, file)), {recursive: true});
    fs.writeFileSync(path.join(cwd, file), text);
  }
  return {cwd, files: Object.keys(content)};
}
const never = () => { throw new Error('No debe invocar go.'); };

test('modulo ausente: solo documentacion, sin invocar go', () => {
  const {cwd, files} = fixture({'README.md': '# Vaiven', 'docs/a.md': 'a', 'specs/002/tasks.md': 't'});
  assert.deepEqual(detect('Backend', {cwd, files, run: never}), {mode: 'docs', ready: false, message: 'Solo documentacion: las pruebas de aplicacion aun no aplican.'});
});

test('codigo Go sin go.mod falla en vez de omitirse', () => {
  const {cwd, files} = fixture({'cmd/api/main.go': 'package main\nfunc main() {}\n'});
  assert.throws(() => detect('Backend', {cwd, files, run: never}), /sin go\.mod/);
});

test('modulo sin codigo (go.mod, .gitkeep y sin go.sum) se omite con motivo explicito', needsGo, () => {
  const {cwd, files} = fixture({'go.mod': GO_MOD, 'cmd/api/.gitkeep': '', 'internal/domain/.gitkeep': '', '.editorconfig': 'root = true\n'});
  const result = detect('Backend', {cwd, files});
  assert.equal(result.mode, 'module');
  assert.equal(result.ready, false);
  assert.match(result.message, /sin codigo.*no encontro paquetes/);
  assert.match(result.message, /go vet.*CodeQL/);
  assert.equal(fs.existsSync(path.join(cwd, 'go.sum')), false);
});

test('paquete Go valido activa todos los controles', needsGo, () => {
  const {cwd, files} = fixture({'go.mod': GO_MOD, 'cmd/api/main.go': 'package main\nfunc main() {}\n'});
  const result = detect('Backend', {cwd, files});
  assert.equal(result.mode, 'full');
  assert.equal(result.ready, true);
});

test('un paquete con solo pruebas tambien es codigo', needsGo, () => {
  const {cwd, files} = fixture({'go.mod': GO_MOD, 'x/x_test.go': 'package x\nimport "testing"\nfunc TestX(t *testing.T) {}\n'});
  assert.equal(detect('Backend', {cwd, files}).mode, 'full');
});

test('go.mod invalido falla', needsGo, () => {
  const {cwd, files} = fixture({'go.mod': 'module\n\ngo 1.21 sobrante\n', 'cmd/api/.gitkeep': ''});
  assert.throws(() => detect('Backend', {cwd, files}), /No se pudieron detectar los paquetes Go/);
});

test('codigo invalido falla: clausula package, prueba con sintaxis rota e import inexistente', needsGo, () => {
  for (const content of [
    {'x.go': 'esto no es go\n'},
    {'x.go': 'package x\n', 'x_test.go': 'package x\nfunc TestX( {\n'},
    {'x.go': 'package x\nimport "no/existe/paquete"\nvar _ = paquete.X\n'}
  ]) {
    const {cwd, files} = fixture({'go.mod': GO_MOD, ...content});
    assert.throws(() => detect('Backend', {cwd, files}), /Go no pudo cargar paquetes|No se pudieron detectar/, JSON.stringify(Object.keys(content)));
  }
});

test('archivos .go que Go no reconoce como paquete no se tratan como proyecto vacio', needsGo, () => {
  const {cwd, files} = fixture({'go.mod': GO_MOD, 'tools/gen.go': '//go:build ignore\n\npackage main\nfunc main() {}\n'});
  assert.throws(() => detect('Backend', {cwd, files}), /Go no detecto ningun paquete/);
});

test('un go ausente o que falla es un error y no un proyecto vacio', () => {
  const {cwd, files} = fixture({'go.mod': GO_MOD});
  assert.throws(() => detect('Backend', {cwd, files, run: () => ({status: null, stdout: '', stderr: '', error: new Error('spawn go ENOENT')})}), /spawn go ENOENT/);
  assert.throws(() => detect('Backend', {cwd, files, run: () => ({status: 1, stdout: '', stderr: 'go: errors parsing go.mod', error: undefined})}), /errors parsing go\.mod/);
  assert.throws(() => listGoPackages(cwd, () => ({status: 0, stdout: 'BAD\texample.com/x\t"boom"\n', stderr: '', error: undefined})), /boom/);
  assert.deepEqual(listGoPackages(cwd, () => ({status: 0, stdout: '', stderr: 'matched no packages', error: undefined})), []);
});

test('Mobile y Landing-Page conservan su comportamiento', () => {
  const none = fixture({'README.md': '# x'});
  assert.equal(detect('Mobile', {cwd: none.cwd, files: none.files}).mode, 'docs');
  assert.equal(detect('Landing-Page', {cwd: none.cwd, files: none.files}).mode, 'docs');
  const app = fixture({'package.json': JSON.stringify({scripts: {typecheck: 'x', lint: 'x', test: 'x'}}), 'package-lock.json': '{}', 'src/a.ts': 'export {}'});
  assert.equal(detect('Mobile', {cwd: app.cwd, files: app.files}).mode, 'full');
  const noLock = fixture({'package.json': '{}'});
  assert.throws(() => detect('Mobile', {cwd: noLock.cwd, files: noLock.files}), /package-lock\.json/);
});

// Ejecuta el script como lo hace el workflow: con repositorio git y archivos de salida de GitHub.
function runCli(content) {
  const {cwd} = fixture(content);
  const git = args => spawnSync('git', args, {cwd, encoding: 'utf8'});
  git(['init', '-q']);
  git(['add', '-A']);
  const output = path.join(cwd, '..', path.basename(cwd) + '.output');
  const summary = path.join(cwd, '..', path.basename(cwd) + '.summary');
  fs.writeFileSync(output, '');
  fs.writeFileSync(summary, '');
  const result = spawnSync('node', [script, 'Backend'], {cwd, encoding: 'utf8', env: {...process.env, GITHUB_OUTPUT: output, GITHUB_STEP_SUMMARY: summary}});
  return {result, output: fs.readFileSync(output, 'utf8'), summary: fs.readFileSync(summary, 'utf8')};
}

test('CLI: modulo sin codigo publica ready=false, mode=module y el motivo en el resumen', needsGo, () => {
  const {result, output, summary} = runCli({'go.mod': GO_MOD, 'cmd/api/.gitkeep': ''});
  assert.equal(result.status, 0, result.stderr);
  assert.match(output, /^ready=false$/m);
  assert.match(output, /^mode=module$/m);
  assert.match(summary, /omitidos/);
  assert.match(summary, /Policy y Secrets siguen siendo obligatorios/);
});

test('CLI: paquete Go publica ready=true y mode=full; go.mod invalido sale con error', needsGo, () => {
  const ok = runCli({'go.mod': GO_MOD, 'main.go': 'package main\nfunc main() {}\n'});
  assert.equal(ok.result.status, 0, ok.result.stderr);
  assert.match(ok.output, /^ready=true$/m);
  assert.match(ok.output, /^mode=full$/m);
  assert.equal(ok.summary, '');
  const bad = runCli({'go.mod': 'module\n', 'main.go': 'package main\nfunc main() {}\n'});
  assert.notEqual(bad.result.status, 0);
  assert.equal(bad.output, '');
});

test('runGo informa de un ejecutable ausente sin lanzar', () => {
  const saved = process.env.PATH;
  process.env.PATH = os.tmpdir();
  try { assert.ok(runGo(['version'], '.').error); } finally { process.env.PATH = saved; }
});
