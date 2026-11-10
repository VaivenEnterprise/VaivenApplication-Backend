#!/usr/bin/env python3
"""Busca secretos dentro de archivos comprimidos del repositorio (SEC-004).

gitleaks no abre .tgz/.zip versionados: así entró un .env real en 7ad4f00 sin
que CI lo viera. Este escáner abre tar, tar.gz/tgz, zip y gz de forma
recursiva y revisa cada archivo interno con reglas de nombre y de contenido.

Seguridad del propio escáner:
  - Todo se lee en memoria, nunca se extrae al disco: un nombre como
    «../../etc/x» o «/abs» no puede escribir fuera de ningún directorio. Aun
    así se reporta como hallazgo «unsafe-path» (intento de zip-slip).
  - Límites de profundidad, número de entradas, tamaño por entrada y tamaño
    total descomprimido (defensa contra bombas de compresión). Superar un
    límite es un hallazgo: el escáner falla cerrado.
  - Enlaces simbólicos, duros y dispositivos dentro de un tar se reportan y no
    se siguen.
  - Nunca imprime el valor encontrado: solo contenedor!ruta:línea y la regla.

Uso:
  scan-archives.py                    archivos versionados y no ignorados
  scan-archives.py --history          además, todo blob comprimido del historial
  scan-archives.py --baseline FILE    blobs ya conocidos (pendientes de limpieza)
  scan-archives.py RUTA...            solo esas rutas (pruebas)

Salida: 0 sin hallazgos, 1 con hallazgos, 2 error de uso o de git.
"""

from __future__ import annotations

import argparse
import bz2
import gzip
import io
import lzma
import re
import subprocess
import sys
import tarfile
import zipfile
from dataclasses import dataclass, field

MAX_DEPTH = 4  # comprimido dentro de comprimido dentro de …
MAX_ENTRIES = 10_000  # entradas sumadas de todos los niveles de un archivo
MAX_MEMBER_BYTES = 50 * 1024 * 1024  # una entrada descomprimida
MAX_TOTAL_BYTES = 300 * 1024 * 1024  # todo lo descomprimido de un archivo
MAX_TEXT_BYTES = 5 * 1024 * 1024  # solo se revisa el contenido de textos menores

ARCHIVE_SUFFIXES = (".tgz", ".tar.gz", ".tar", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".gz", ".zip", ".jar", ".war")

# Nombres que nunca deben ir dentro de un comprimido versionado.
SENSITIVE_NAME = re.compile(
    r"(^|/)("
    r"\.env(\.(?!example$|[^/]*\.example$)[^/]+)?"  # .env, .env.local; no .env.example
    r"|id_(rsa|dsa|ecdsa|ed25519)"
    r"|[^/]+\.(pem|key|p12|pfx|jks|keystore)"
    r"|credentials\.json|service-account[^/]*\.json|\.npmrc|\.pypirc|\.netrc"
    r")$"
)

PLACEHOLDER = re.compile(
    r"^(<[^>]*>|\$\{?[^}]*\}?|\{\{.*\}\}|%[^%]+%|changeme|change-me|change_me|example|test|dummy|"
    r"x{3,}|\*{3,}|\.{3,}|your[-_].*|redacted|\[redacted\]|none|null|true|false)$",
    re.IGNORECASE,
)

CONTENT_RULES: list[tuple[str, re.Pattern[str]]] = [
    ("cloudinary-url", re.compile(r"cloudinary://[A-Za-z0-9_-]{4,}:[A-Za-z0-9_-]{4,}@")),
    ("private-key", re.compile(r"-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----")),
    ("uri-with-password", re.compile(r"\b(?:mongodb(?:\+srv)?|rediss?|postgres(?:ql)?|mysql|amqps?)://[^\s:/@<>{}$]+:[^\s@/<>{}$]+@")),
    ("github-token", re.compile(r"\bgh[pousr]_[A-Za-z0-9]{36,}\b")),
    ("aws-access-key", re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
]

ASSIGNMENT = re.compile(
    r"^\s*(?:export\s+)?(?P<name>[A-Za-z0-9_]*(?:SECRET|PASSWORD|PASSWD|TOKEN|API_KEY|APIKEY|PRIVATE_KEY|ACCESS_KEY)[A-Za-z0-9_]*)"
    r"\s*[=:]\s*[\"']?(?P<value>[^\"'\s#]*)"
)


@dataclass
class Budget:
    entries: int = 0
    total: int = 0


@dataclass
class Report:
    findings: list[tuple[str, str]] = field(default_factory=list)  # (regla, ubicación)
    known: list[str] = field(default_factory=list)
    scanned: int = 0

    def add(self, rule: str, where: str) -> None:
        self.findings.append((rule, where))


def kind_of(name: str, data: bytes) -> str | None:
    """Tipo de comprimido por firma (y nombre para tar sin comprimir)."""
    head = data[:262]
    if head.startswith(b"\x1f\x8b"):
        return "gzip"
    if head.startswith(b"PK\x03\x04") or head.startswith(b"PK\x05\x06"):
        return "zip"
    if head.startswith(b"BZh"):
        return "bz2"
    if head.startswith(b"\xfd7zXZ\x00"):
        return "xz"
    if len(data) >= 262 and data[257:262] == b"ustar":
        return "tar"
    if name.lower().endswith(".tar"):
        return "tar"
    return None


def bounded(stream: io.BufferedIOBase, where: str, budget: Budget, report: Report) -> bytes | None:
    """Lee como máximo MAX_MEMBER_BYTES y respeta el total del archivo."""
    data = stream.read(MAX_MEMBER_BYTES + 1)
    if len(data) > MAX_MEMBER_BYTES:
        report.add("limit-member-size", where)
        return None
    budget.total += len(data)
    if budget.total > MAX_TOTAL_BYTES:
        report.add("limit-total-size", where)
        return None
    return data


def unsafe(name: str) -> bool:
    n = name.replace("\\", "/")
    return n.startswith("/") or re.match(r"^[A-Za-z]:", n) is not None or ".." in n.split("/")


def scan_text(where: str, data: bytes, report: Report) -> None:
    if len(data) > MAX_TEXT_BYTES or b"\x00" in data[:8192]:
        return
    text = data.decode("utf-8", errors="ignore")
    for lineno, line in enumerate(text.splitlines(), 1):
        for rule, rx in CONTENT_RULES:
            if rx.search(line):
                report.add(rule, f"{where}:{lineno}")
        m = ASSIGNMENT.match(line)
        if m:
            value = m.group("value")
            if len(value) >= 8 and not PLACEHOLDER.match(value):
                report.add(f"secret-assignment({m.group('name')})", f"{where}:{lineno}")


def scan_member(where: str, name: str, data: bytes, depth: int, budget: Budget, report: Report) -> None:
    if SENSITIVE_NAME.search(name.replace("\\", "/")):
        report.add("sensitive-file", where)
    if kind_of(name, data):
        scan_archive(where, name, data, depth + 1, budget, report)
    else:
        scan_text(where, data, report)


def scan_archive(where: str, name: str, data: bytes, depth: int, budget: Budget, report: Report) -> None:
    if depth > MAX_DEPTH:
        report.add("limit-depth", where)
        return
    kind = kind_of(name, data)
    try:
        if kind in ("gzip", "bz2", "xz"):
            opener = {"gzip": gzip.GzipFile, "bz2": bz2.BZ2File, "xz": lzma.LZMAFile}[kind]
            with opener(fileobj=io.BytesIO(data)) as f:
                inner = bounded(f, where, budget, report)
            if inner is None:
                return
            inner_name = re.sub(r"\.(gz|tgz|bz2|tbz2|xz|txz)$", lambda m: ".tar" if m.group(1).startswith("t") else "", name)
            if kind_of(inner_name, inner) == "tar":
                scan_archive(where, inner_name or "x.tar", inner, depth, budget, report)
            else:
                scan_member(f"{where}!{inner_name or '(gz)'}", inner_name, inner, depth, budget, report)
        elif kind == "tar":
            with tarfile.open(fileobj=io.BytesIO(data), mode="r:") as tf:
                for ti in tf:
                    budget.entries += 1
                    if budget.entries > MAX_ENTRIES:
                        report.add("limit-entries", where)
                        return
                    loc = f"{where}!{ti.name}"
                    if unsafe(ti.name) or (ti.islnk() or ti.issym()) and unsafe(ti.linkname):
                        report.add("unsafe-path", loc)
                    if ti.issym() or ti.islnk() or ti.isdev() or ti.isfifo():
                        if not (ti.issym() or ti.islnk()):
                            report.add("special-file", loc)
                        continue
                    if not ti.isfile():
                        continue
                    if ti.size > MAX_MEMBER_BYTES:
                        report.add("limit-member-size", loc)
                        continue
                    f = tf.extractfile(ti)
                    if f is None:
                        continue
                    member = bounded(f, loc, budget, report)
                    if member is None:
                        return
                    scan_member(loc, ti.name, member, depth, budget, report)
        elif kind == "zip":
            with zipfile.ZipFile(io.BytesIO(data)) as zf:
                for zi in zf.infolist():
                    budget.entries += 1
                    if budget.entries > MAX_ENTRIES:
                        report.add("limit-entries", where)
                        return
                    loc = f"{where}!{zi.filename}"
                    if unsafe(zi.filename):
                        report.add("unsafe-path", loc)
                    if zi.is_dir():
                        continue
                    if zi.file_size > MAX_MEMBER_BYTES:
                        report.add("limit-member-size", loc)
                        continue
                    if zi.flag_bits & 0x1:
                        report.add("encrypted-entry", loc)  # no se puede revisar: falla cerrado
                        continue
                    with zf.open(zi) as f:
                        member = bounded(f, loc, budget, report)
                    if member is None:
                        return
                    scan_member(loc, zi.filename, member, depth, budget, report)
    except (tarfile.TarError, zipfile.BadZipFile, OSError, EOFError, lzma.LZMAError, ValueError, NotImplementedError):
        report.add("unreadable-archive", where)  # corrupto o truncado: no se puede afirmar que esté limpio


def is_candidate(path: str) -> bool:
    return path.lower().endswith(ARCHIVE_SUFFIXES)


def git(*args: str) -> bytes:
    return subprocess.run(["git", *args], check=True, capture_output=True).stdout


def scan_blob(where: str, name: str, data: bytes, report: Report) -> None:
    report.scanned += 1
    if kind_of(name, data):
        scan_archive(where, name, data, 1, Budget(), report)
    elif is_candidate(name):
        report.add("unreadable-archive", where)


def scan_worktree(report: Report, paths: list[str]) -> None:
    if not paths:
        out = git("ls-files", "-z", "--cached", "--others", "--exclude-standard")
        paths = [p for p in out.decode("utf-8", "surrogateescape").split("\0") if p]
    for p in paths:
        try:
            with open(p, "rb") as f:
                head = f.read(262)
                if not (is_candidate(p) or kind_of(p, head)):
                    continue
                f.seek(0)
                data = f.read(MAX_TOTAL_BYTES + 1)
        except (FileNotFoundError, IsADirectoryError, PermissionError):
            continue  # borrado en el árbol (pero no en el índice) o directorio
        if len(data) > MAX_TOTAL_BYTES:
            report.add("limit-total-size", p)
            continue
        scan_blob(p, p, data, report)


def scan_history(report: Report, baseline: set[str]) -> None:
    seen: dict[str, str] = {}
    for line in git("rev-list", "--all", "--objects").decode("utf-8", "surrogateescape").splitlines():
        sha, _, path = line.partition(" ")
        if path and is_candidate(path) and sha not in seen:
            seen[sha] = path
    for sha, path in seen.items():
        where = f"historial:{sha[:12]}:{path}"
        if sha in baseline:
            report.known.append(where)
            continue
        size = int(git("cat-file", "-s", sha))
        if size > MAX_TOTAL_BYTES:
            report.add("limit-total-size", where)
            continue
        scan_blob(where, path, git("cat-file", "blob", sha), report)


def load_baseline(path: str | None) -> set[str]:
    if not path:
        return set()
    shas = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            token = line.split("#", 1)[0].strip()
            if token:
                if not re.fullmatch(r"[0-9a-f]{40}", token):
                    raise ValueError(f"línea inválida en {path}: se espera un SHA de blob de 40 caracteres")
                shas.add(token)
    return shas


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description="Busca secretos dentro de archivos comprimidos.")
    ap.add_argument("--history", action="store_true", help="revisa también todos los blobs comprimidos del historial")
    ap.add_argument("--baseline", help="archivo con SHA de blobs ya conocidos (pendientes de limpiar el historial)")
    ap.add_argument("paths", nargs="*", help="rutas concretas (por defecto, los archivos del repositorio)")
    args = ap.parse_args(argv)
    report = Report()
    try:
        baseline = load_baseline(args.baseline)
        scan_worktree(report, args.paths)
        if args.history:
            scan_history(report, baseline)
    except (subprocess.CalledProcessError, ValueError, OSError) as e:
        print(f"scan-archives: error: {e}", file=sys.stderr)
        return 2
    for where in report.known:
        print(f"CONOCIDO (pendiente de limpiar el historial)  {where}")
    for rule, where in report.findings:
        print(f"HALLAZGO  {rule}  {where}")
    print(f"scan-archives: {report.scanned} comprimidos revisados, {len(report.findings)} hallazgos, {len(report.known)} conocidos.")
    return 1 if report.findings else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
