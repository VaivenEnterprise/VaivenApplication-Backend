"""Pruebas de scripts/scan-archives.py (SEC-004): casos positivos y negativos.

Todos los secretos son ficticios y se generan al vuelo con secrets.token_hex:
no hay ningún valor fijo en este archivo. Cada prueba comprueba además que la
salida del escáner nunca contenga el valor.

Ejecutar: python3 -I -m unittest discover -s scripts -p 'scan_archives_test.py'
"""

from __future__ import annotations

import gzip
import io
import os
import secrets
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile

SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "scan-archives.py")


def tgz(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tf:
        for name, data in files.items():
            ti = tarfile.TarInfo(name)
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))
    return buf.getvalue()


def zipped(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for name, data in files.items():
            zf.writestr(name, data)
    return buf.getvalue()


def fake() -> str:
    return "f" + secrets.token_hex(16)


class ScanArchivesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def write(self, name: str, data: bytes) -> str:
        path = os.path.join(self.dir, name)
        with open(path, "wb") as f:
            f.write(data)
        return path

    def run_scan(self, *args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.run([sys.executable, "-I", SCRIPT, *args], capture_output=True, text=True, cwd=cwd)

    def assert_finding(self, res: subprocess.CompletedProcess[str], rule: str, *hidden: str) -> None:
        self.assertEqual(res.returncode, 1, res.stdout + res.stderr)
        self.assertIn(f"HALLAZGO  {rule}", res.stdout)
        for value in hidden:
            self.assertNotIn(value, res.stdout + res.stderr, "el escáner imprimió el valor")

    def assert_clean(self, res: subprocess.CompletedProcess[str]) -> None:
        self.assertEqual(res.returncode, 0, res.stdout + res.stderr)
        self.assertNotIn("HALLAZGO", res.stdout)

    # --- Casos positivos -------------------------------------------------

    def test_env_with_secret_inside_tgz(self) -> None:
        secret = fake()
        p = self.write("repo.tgz", tgz({"app/.env": f"JWT_ACCESS_SECRET={secret}\nPORT=8080\n".encode()}))
        res = self.run_scan(p)
        self.assert_finding(res, "sensitive-file", secret)
        self.assertIn("HALLAZGO  secret-assignment(JWT_ACCESS_SECRET)", res.stdout)
        self.assertIn("repo.tgz!app/.env:1", res.stdout)

    def test_cloudinary_url_in_zip_nested_in_tgz(self) -> None:
        key, sec = secrets.token_hex(8), fake()
        url = "cloudinary" + f"://{key}:{sec}@demo-cloud"
        inner = zipped({"config/app.yaml": f"images:\n  url: {url}\n".encode()})
        p = self.write("bundle.tgz", tgz({"deps/conf.zip": inner}))
        res = self.run_scan(p)
        self.assert_finding(res, "cloudinary-url", sec, key)
        self.assertIn("bundle.tgz!deps/conf.zip!config/app.yaml:2", res.stdout)

    def test_private_key_in_plain_gz(self) -> None:
        body = "-----BEGIN " + "RSA PRIVATE KEY-----\n" + secrets.token_hex(32) + "\n"
        p = self.write("notes.gz", gzip.compress(body.encode()))
        self.assert_finding(self.run_scan(p), "private-key")

    def test_uri_with_password(self) -> None:
        pw = fake()
        p = self.write("db.zip", zipped({"compose.yml": f"MONGO: mongodb://admin:{pw}@db:27017/x\n".encode()}))
        self.assert_finding(self.run_scan(p), "uri-with-password", pw)

    def test_path_traversal_entry_is_reported(self) -> None:
        p = self.write("slip.zip", zipped({"../../outside.txt": b"hola"}))
        self.assert_finding(self.run_scan(p), "unsafe-path")
        self.assertFalse(os.path.exists(os.path.join(self.dir, "..", "..", "outside.txt")))

    def test_absolute_path_in_tar(self) -> None:
        p = self.write("abs.tgz", tgz({"/etc/passwd": b"root:x:0:0"}))
        self.assert_finding(self.run_scan(p), "unsafe-path")

    def test_depth_limit_fails_closed(self) -> None:
        data = b"nada"
        for i in range(7):
            data = zipped({f"n{i}.zip" if i else "x.txt": data})
        p = self.write("deep.zip", data)
        self.assert_finding(self.run_scan(p), "limit-depth")

    def test_decompression_bomb_fails_closed(self) -> None:
        p = self.write("bomb.gz", gzip.compress(b"\0" * (51 * 1024 * 1024)))
        self.assert_finding(self.run_scan(p), "limit-member-size")

    def test_corrupt_archive_fails_closed(self) -> None:
        good = tgz({"a.txt": b"hola" * 1000})
        p = self.write("broken.tgz", good[: len(good) // 2])
        self.assert_finding(self.run_scan(p), "unreadable-archive")

    def test_archive_detected_by_signature_without_extension(self) -> None:
        secret = fake()
        p = self.write("datos.bin", tgz({".env": f"API_KEY={secret}\n".encode()}))
        self.assert_finding(self.run_scan(p), "sensitive-file", secret)

    # --- Casos negativos -------------------------------------------------

    def test_env_example_with_placeholders_is_clean(self) -> None:
        example = (
            "JWT_ACCESS_SECRET=\n"
            "CLOUDINARY_API_SECRET=<api_secret>\n"
            "MONGO_PASSWORD=${MONGO_PASSWORD}\n"
            "CLOUDINARY_URL=cloudinary://<api_key>:<api_secret>@<cloud_name>\n"
            "MONGO_URI=mongodb://localhost:27017/vaiven\n"
            "EXPO_ACCESS_TOKEN=changeme\n"
        )
        p = self.write("ok.tgz", tgz({".env.example": example.encode(), "deploy/.env.staging.example": b"TOKEN=\n"}))
        self.assert_clean(self.run_scan(p))

    def test_source_code_archive_is_clean(self) -> None:
        go = b'package main\n\nfunc main() { token := os.Getenv("TOKEN"); _ = token }\n'
        p = self.write("src.zip", zipped({"cmd/main.go": go, "README.md": b"# Hola\n", "img.png": b"\x89PNG\x00\x00"}))
        self.assert_clean(self.run_scan(p))

    def test_plain_text_file_is_ignored(self) -> None:
        p = self.write("notes.txt", f"JWT_ACCESS_SECRET={fake()}\n".encode())
        res = self.run_scan(p)
        self.assert_clean(res)  # gitleaks revisa los textos; este escáner, los comprimidos
        self.assertIn("0 comprimidos revisados", res.stdout)

    # --- Historial de git y lista de conocidos ---------------------------

    def git(self, *args: str) -> str:
        env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com", GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")
        return subprocess.run(["git", "-c", "commit.gpgsign=false", *args], cwd=self.dir, check=True, capture_output=True, text=True, env=env).stdout

    def test_history_detects_deleted_archive_and_baseline_suppresses_it(self) -> None:
        secret = fake()
        self.git("init", "-q")
        self.write("leak.tgz", tgz({".env": f"CLOUDINARY_API_SECRET={secret}\n".encode()}))
        self.git("add", "-f", "leak.tgz")
        self.git("commit", "-qm", "leak")
        self.git("rm", "-q", "leak.tgz")
        self.git("commit", "-qm", "remove")

        self.assert_clean(self.run_scan(cwd=self.dir))  # el árbol ya está limpio
        res = self.run_scan("--history", cwd=self.dir)  # pero el historial no
        self.assert_finding(res, "secret-assignment(CLOUDINARY_API_SECRET)", secret)
        self.assertIn("historial:", res.stdout)

        blob = self.git("rev-list", "--all", "--objects").split("leak.tgz")[0].split()[-1]
        self.write("baseline", f"# conocido\n{blob}  # pendiente P-3\n".encode())
        res = self.run_scan("--history", "--baseline", "baseline", cwd=self.dir)
        self.assert_clean(res)
        self.assertIn("CONOCIDO", res.stdout)

        # Un comprimido nuevo con secreto no queda cubierto por la lista.
        self.write("otro.zip", zipped({".env": f"TOKEN={fake()}\n".encode()}))
        self.git("add", "-f", "otro.zip")
        self.git("commit", "-qm", "otro")
        res = self.run_scan("--history", "--baseline", "baseline", cwd=self.dir)
        self.assert_finding(res, "sensitive-file")

    def test_invalid_baseline_is_usage_error(self) -> None:
        self.git("init", "-q")
        self.write("baseline", b"no-es-un-sha\n")
        self.assertEqual(self.run_scan("--history", "--baseline", "baseline", cwd=self.dir).returncode, 2)


if __name__ == "__main__":
    unittest.main()
