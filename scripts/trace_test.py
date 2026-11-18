"""Pruebas de scripts/trace.py (spec 016, T003). Usan un repositorio de
ejemplo pequeño en una carpeta temporal; nunca leen el repositorio real."""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import trace  # noqa: E402

SPEC = '''# Spec 015
| ID | Requisito |
|---|---|
| RF-01 | algo |
| RF-02 | otra cosa |
| RNF-01 | rápido |
| CA-01 | dado... |
| CA-10 | dado... |
| RF-01 | repetido en otra tabla |
'''

GO_TEST = '''package reco

// CA-01: lo primero es una herramienta.
func TestHybrid(t *testing.T) {
}

func TestOther(t *testing.T) { // RF-02 y RNF-01
}

func TestCA10Like(t *testing.T) {
	// menciona CA-1 y CA-100, que no son el criterio diez
}

func BenchmarkRecompute(b *testing.B) { // RNF-01
}
'''


class TraceTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        r = self.tmp.name
        os.makedirs(os.path.join(r, 'specs', '015-reco'))
        os.makedirs(os.path.join(r, 'internal', 'reco', 'usecase'))
        os.makedirs(os.path.join(r, 'specs', '099-otra'))
        with open(os.path.join(r, 'specs', '015-reco', 'spec.md'), 'w', encoding='utf-8') as f:
            f.write(SPEC)
        with open(os.path.join(r, 'specs', '099-otra', 'spec.md'), 'w', encoding='utf-8') as f:
            f.write('| RF-01 | fuera del mapa |\n')
        with open(os.path.join(r, 'internal', 'reco', 'usecase', 'x_test.go'), 'w', encoding='utf-8') as f:
            f.write(GO_TEST)
        with open(os.path.join(r, 'internal', 'reco', 'usecase', 'x.go'), 'w', encoding='utf-8') as f:
            f.write('// CA-10 en código que no es prueba\n')
        self.root = r

    def tearDown(self):
        self.tmp.cleanup()

    def test_ids_in_order_without_duplicates(self):
        self.assertEqual(trace.spec_ids(SPEC), ['RF-01', 'RF-02', 'RNF-01', 'CA-01', 'CA-10'])

    def test_trace_maps_ids_to_tests(self):
        result = dict(trace.trace(self.root))
        self.assertEqual(list(result), ['015-reco'])  # 099 no está en el mapa
        rows = dict(result['015-reco'])
        f = 'internal/reco/usecase/x_test.go'
        self.assertEqual(rows['CA-01'], [f + ':TestHybrid'])
        self.assertEqual(rows['RF-02'], [f + ':TestOther'])
        self.assertEqual(rows['RNF-01'], [f + ':BenchmarkRecompute', f + ':TestOther'])
        self.assertEqual(rows['CA-10'], [])  # CA-1, CA-100 y el código no cuentan
        self.assertEqual(rows['RF-01'], [])

    def test_render_marks_missing(self):
        md = trace.render(trace.trace(self.root))
        self.assertIn('## 015-reco (3/5 con prueba)', md)
        self.assertIn('| CA-10 | **sin prueba** |', md)
        self.assertIn('**Resumen:** 3 de 5 IDs', md)

    def test_does_not_write(self):
        before = sorted(os.walk(self.root))
        trace.trace(self.root)
        self.assertEqual(before, sorted(os.walk(self.root)))

    def test_spec_filter(self):
        self.assertEqual(trace.trace(self.root, ['014']), [])


if __name__ == '__main__':
    unittest.main()
