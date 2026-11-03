import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
const kind = process.argv[2];
const files=execFileSync('git',['ls-files','-z'],{encoding:'utf8'}).split('\0').filter(Boolean);
const appFiles=files.filter(p=>!/^(.github|.opencode|specs|docs)\//.test(p));
const source = kind==='Backend' ? appFiles.some(p=>p.endsWith('.go')) : kind==='Mobile' ? appFiles.some(p=>/\.(tsx?|jsx?)$/.test(p)) : appFiles.some(p=>/\.(html|css|js)$/.test(p));
const manifest = kind==='Backend' ? 'go.mod' : kind==='Mobile' ? 'package.json' : 'index.html';
const ready=fs.existsSync(manifest);
if (source && !ready) throw new Error('Hay codigo sin '+manifest+'; no se permite omitir sus pruebas.');
if (ready && kind==='Mobile') {
  const pkg=JSON.parse(fs.readFileSync('package.json','utf8'));
  if (!fs.existsSync('package-lock.json')) throw new Error('Se requiere package-lock.json para npm ci.');
  for (const script of ['typecheck','lint','test']) if (!pkg.scripts?.[script]) throw new Error('Falta npm run '+script);
}
if (ready && kind==='Landing-Page' && !fs.existsSync('scripts/check.mjs')) throw new Error('La landing debe incluir scripts/check.mjs.');
if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT,'ready='+ready+'\n');
console.log(ready ? 'Proyecto inicializado: se ejecutan sus controles.' : 'Solo documentacion: las pruebas de aplicacion aun no aplican.');
