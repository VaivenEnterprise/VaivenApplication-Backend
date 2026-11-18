#!/usr/bin/env python3
"""Trazabilidad de requisitos a pruebas (spec 016, T003; plan P-08).

Lee las tablas de requisitos (RF-NN, RNF-NN) y de criterios de aceptación
(CA-NN) de cada specs/NNN-*/spec.md y busca, en los archivos de prueba de los
módulos de esa spec, las pruebas que citan cada ID en su nombre o en un
comentario. Escribe una tabla en Markdown por spec. Solo lee el repositorio;
nunca escribe en él (la salida va a stdout).

    python3 -I scripts/trace.py > docs/calidad/trazabilidad.md
"""
import argparse
import os
import re
import sys

# Módulos y carpetas de prueba de cada spec (ROADMAP §3). Una spec sin módulo
# propio usa las carpetas que integran o administran.
SPEC_PATHS = {
    '003': ['internal/auth', 'internal/users', 'internal/platform/security'],
    '004': ['internal/images'],
    '005': ['internal/items'],
    '006': ['internal/search'],
    '007': ['internal/bookings'],
    '008': ['internal/chat'],
    '009': ['internal/handover', 'internal/payments'],
    '010': ['internal/ratings', 'internal/incidents', 'internal/sanctions'],
    '011': ['internal/notifications', 'internal/reports', 'internal/moderation', 'internal/notify'],
    '012': ['test', 'cmd/admin', 'internal/adapter'],
    '013': ['cmd/admin', 'scripts', 'deploy'],
    '014': ['internal/ai', 'internal/suggest', 'internal/assistant', 'internal/config'],
    '015': ['internal/reco', 'cmd/admin', 'internal/search'],
}

ID_ROW = re.compile(r'^\|\s*((?:RF|RNF|CA)-\d{2,})\b', re.M)
TEST_FUNC = re.compile(r'^func (Test\w+|Benchmark\w+)\(', re.M)
TEST_FILE = re.compile(r'(_test\.go|_test\.py|\.test\.mjs)$')


def spec_ids(text):
    """IDs en el orden en que aparecen en las tablas, sin repetir."""
    seen, out = set(), []
    for m in ID_ROW.finditer(text):
        if m.group(1) not in seen:
            seen.add(m.group(1))
            out.append(m.group(1))
    return out


def test_files(root, paths):
    for p in paths:
        base = os.path.join(root, p)
        if os.path.isfile(base) and TEST_FILE.search(base):
            yield base
        for dirpath, _, files in os.walk(base):
            for f in sorted(files):
                if TEST_FILE.search(f):
                    yield os.path.join(dirpath, f)


def references(path, ids):
    """Pruebas de un archivo que citan cada ID: {id: [nombre de prueba]}.

    Un ID cuenta si aparece en la función de prueba (nombre, comentario o
    cuerpo) o en el comentario inmediatamente anterior a ella.
    """
    text = open(path, encoding='utf-8', errors='ignore').read()
    funcs = list(TEST_FUNC.finditer(text))
    found = {}
    for i, m in enumerate(funcs):
        start = text.rfind('\n\n', 0, m.start()) + 1
        end = funcs[i + 1].start() if i + 1 < len(funcs) else len(text)
        chunk = text[start:end]
        for id_ in ids:
            if re.search(r'(?<![\w-])' + re.escape(id_) + r'(?!\d)', chunk):
                found.setdefault(id_, []).append(m.group(1))
    return found


def trace(root, spec_dirs=None):
    """Devuelve [(spec, [(id, [archivo:prueba])])] para las specs de SPEC_PATHS."""
    out = []
    specs_root = os.path.join(root, 'specs')
    for d in sorted(os.listdir(specs_root)):
        num = d[:3]
        if num not in SPEC_PATHS or (spec_dirs and num not in spec_dirs):
            continue
        spec_md = os.path.join(specs_root, d, 'spec.md')
        if not os.path.isfile(spec_md):
            continue
        ids = spec_ids(open(spec_md, encoding='utf-8').read())
        hits = {i: [] for i in ids}
        for f in test_files(root, SPEC_PATHS[num]):
            for id_, tests in references(f, ids).items():
                rel = os.path.relpath(f, root).replace(os.sep, '/')
                hits[id_].extend(f'{rel}:{t}' for t in tests)
        out.append((d, [(i, sorted(set(hits[i]))) for i in ids]))
    return out


def render(result):
    lines = ['# Trazabilidad de requisitos a pruebas (spec 016, T003)', '',
             'Generado con `python3 -I scripts/trace.py`. Una prueba cuenta si cita el ID en su nombre, '
             'su comentario o su cuerpo. «Sin prueba» no siempre es un hueco: algunos requisitos se '
             'verifican por despliegue, documentación o revisión; esos casos se revisan a mano en '
             '`deuda.md`.', '']
    total = covered = 0
    for spec, rows in result:
        n = sum(1 for _, t in rows if t)
        total += len(rows)
        covered += n
        lines += [f'## {spec} ({n}/{len(rows)} con prueba)', '', '| ID | Pruebas |', '|---|---|']
        for id_, tests in rows:
            cell = '<br>'.join(f'`{t}`' for t in tests[:4]) + (f' (+{len(tests) - 4})' if len(tests) > 4 else '')
            lines.append(f'| {id_} | {cell or "**sin prueba**"} |')
        lines.append('')
    lines.insert(4, f'**Resumen:** {covered} de {total} IDs con al menos una prueba que los cita.\n')
    return '\n'.join(lines) + '\n'


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split('\n')[0])
    ap.add_argument('--root', default='.')
    ap.add_argument('--spec', action='append', help='limitar a una spec (p. ej. 015)')
    args = ap.parse_args(argv)
    sys.stdout.reconfigure(encoding='utf-8')
    sys.stdout.write(render(trace(args.root, args.spec)))
    return 0


if __name__ == '__main__':
    sys.exit(main())
