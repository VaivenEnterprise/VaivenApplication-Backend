import fs from 'node:fs';
import path from 'node:path';
const visit=dir=>fs.readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?visit(path.join(dir,e.name)):e.name.endsWith('.sarif')?[path.join(dir,e.name)]:[]);
const files=visit('codeql-results');
if(!files.length) throw new Error('CodeQL no genero resultados SARIF.');
let failures=0;
for(const file of files) for(const run of JSON.parse(fs.readFileSync(file,'utf8')).runs||[]) {
  const rules=[...(run.tool?.driver?.rules||[]),...(run.tool?.extensions||[]).flatMap(e=>e.rules||[])];
  for(const result of run.results||[]) {
    const rule=rules.find(r=>r.id===result.ruleId);
    const severity=Number(rule?.properties?.['security-severity']||0);
    if(severity>=7||result.level==='error') {console.error('CodeQL bloquea: '+result.ruleId+' (severity '+severity+')'); failures++;}
  }
}
if(failures) process.exitCode=1;
else console.log('CodeQL: sin hallazgos high/critical.');
