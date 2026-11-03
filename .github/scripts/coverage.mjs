import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
const files=execFileSync('git',['ls-files','-z'],{encoding:'utf8'}).split('\0');
if (process.argv[2]==='go') {
  const groups=new Map();
  for (const row of fs.readFileSync('coverage.out','utf8').trim().split('\n').slice(1)) {
    const match=row.match(/^(.*):(\d+\.\d+,\d+\.\d+) (\d+) (\d+)$/);
    if (!match) throw new Error('Formato de cobertura Go invalido.');
    const layer=match[1].match(/\/(domain|usecase)\//)?.[1];
    if (!layer) continue;
    const value=groups.get(layer)||[0,0]; value[0]+=Number(match[3]); if(Number(match[4])>0)value[1]+=Number(match[3]); groups.set(layer,value);
  }
  for (const layer of ['domain','usecase']) {
    const exists=files.some(p=>new RegExp('(^|/)'+layer+'/.*\\.go$').test(p)&&!p.endsWith('_test.go')&&!p.endsWith('/doc.go'));
    const [total,covered]=groups.get(layer)||[0,0];
    if(exists && (!total||covered/total<.8)) throw new Error(layer+': cobertura menor a 80% o ausente.');
    if(total) console.log(layer+': '+(100*covered/total).toFixed(2)+'%');
  }
} else {
  const report=JSON.parse(fs.readFileSync('coverage/coverage-final.json','utf8'));
  const relevant=p=>/(^|\/)src\/(lib\/|features\/[^/]+\/hooks\/)/.test(p.replaceAll('\\','/')) && /\.[jt]sx?$/.test(p) && !/\.d\.ts$|\.(test|spec)\.[jt]sx?$/.test(p);
  const normalized=new Map(Object.entries(report).map(([p,v])=>[p.replaceAll('\\','/'),v]));
  for(const file of files.filter(relevant)) {
    const entry=[...normalized].find(([p])=>p.endsWith('/'+file))?.[1];
    if(!entry) throw new Error('Sin cobertura de '+file+'; configura collectCoverageFrom en Jest.');
    for(const [metric,counts] of [['statements',Object.values(entry.s)],['functions',Object.values(entry.f)],['branches',Object.values(entry.b).flat()]]) {
      if(counts.length && counts.filter(n=>n>0).length/counts.length<.8) throw new Error(file+': '+metric+' menor a 80%.');
    }
  }
}
