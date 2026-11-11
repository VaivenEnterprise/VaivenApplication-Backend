package catalog

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

const modulePath = "github.com/VaivenEnterprise/VaivenApplication-Backend"

// moduleRoot sube desde el directorio de la prueba hasta go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no se encontró go.mod")
		}
		dir = parent
	}
}

// declarer describe un tipo del proyecto que tiene EnsureIndexes.
type declarer struct {
	pos           string // archivo:línea, para el mensaje
	importPath    string
	typeName      string
	hasIndexes    bool   // tiene el método Indexes()
	indexesFunc   string // función de paquete que devuelve Indexes()
	ensureUsesDcl bool   // EnsureIndexes llama a <recv>.Indexes()
}

// scanDeclarers recorre internal/ con go/parser (sin compilar ni modificar
// nada) y devuelve cada tipo con método EnsureIndexes, sin archivos de prueba.
func scanDeclarers(t *testing.T, root string) []*declarer {
	t.Helper()
	byType := map[string]*declarer{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		imp := modulePath + "/" + filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			typ := recvType(fn.Recv.List[0].Type)
			key := imp + "." + typ
			get := func() *declarer {
				if byType[key] == nil {
					byType[key] = &declarer{importPath: imp, typeName: typ}
				}
				return byType[key]
			}
			switch fn.Name.Name {
			case "EnsureIndexes":
				dcl := get()
				dcl.pos = fset.Position(fn.Pos()).String()
				dcl.ensureUsesDcl = callsIndexes(fn)
			case "Indexes":
				dcl := get()
				dcl.hasIndexes = true
				dcl.indexesFunc = returnedFunc(fn)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recorrer internal/: %v", err)
	}
	var out []*declarer
	for _, d := range byType {
		if d.pos != "" { // solo los que tienen EnsureIndexes
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	return out
}

func recvType(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// callsIndexes busca una llamada <algo>.Indexes() sin argumentos dentro del
// cuerpo (la de la colección, col.Indexes(), no cuenta: no es del receptor).
func callsIndexes(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Recv.List[0].Names) == 0 {
		return false
	}
	recv := fn.Recv.List[0].Names[0].Name
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Indexes" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == recv {
			found = true
		}
		return true
	})
	return found
}

// returnedFunc devuelve F si el cuerpo de Indexes es `return F()`.
func returnedFunc(fn *ast.FuncDecl) string {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return ""
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return ""
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return ""
	}
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// catalogued son las funciones de sources como "rutaDeImport.Nombre".
func catalogued() map[string]bool {
	m := map[string]bool{}
	for _, src := range sources {
		m[runtime.FuncForPC(reflect.ValueOf(src).Pointer()).Name()] = true
	}
	return m
}

// Prueba 15 (plan 012 §7.6): todo EnsureIndexes del proyecto consume su
// declaración Indexes(), y esa declaración está en el catálogo. Si alguien
// agrega un repositorio con índices y no lo cataloga, esta prueba falla.
func TestCatalogCoversEveryEnsureIndexes(t *testing.T) {
	root := moduleRoot(t)
	decls := scanDeclarers(t, root)
	if len(decls) == 0 {
		t.Fatal("no se encontró ningún EnsureIndexes: el recorrido está roto")
	}
	inCatalog := catalogued()
	used := map[string]bool{}
	for _, d := range decls {
		where := d.pos + " (" + d.typeName + ")"
		if !d.hasIndexes {
			t.Errorf("%s: tiene EnsureIndexes pero no Indexes()", where)
			continue
		}
		if !d.ensureUsesDcl {
			t.Errorf("%s: EnsureIndexes no consume su declaración Indexes()", where)
		}
		if d.indexesFunc == "" {
			t.Errorf("%s: Indexes() debe ser `return <Función>()` de su paquete", where)
			continue
		}
		full := d.importPath + "." + d.indexesFunc
		if !inCatalog[full] {
			t.Errorf("%s: %s no está en catalog.sources", where, full)
		}
		used[full] = true
	}
	for f := range inCatalog {
		if !used[f] {
			t.Errorf("catalog.sources tiene %s, que ningún EnsureIndexes usa", f)
		}
	}
}

func TestAllHas34UniqueIndexes(t *testing.T) {
	all := All()
	if len(all) != 34 {
		t.Errorf("catálogo con %d índices, want 34 (plan 012 §7.3)", len(all))
	}
	seen := map[string]bool{}
	for _, s := range all {
		name, _, err := adaptermongo.ExpectedDocument(s)
		if err != nil {
			t.Errorf("%s: %v", s.Collection, err)
			continue
		}
		id := s.Collection + "." + name
		if seen[id] {
			t.Errorf("%s declarado dos veces", id)
		}
		seen[id] = true
	}
}

// All no debe compartir el slice interno: quien lo modifique no altera el
// catálogo.
func TestAllReturnsFreshSlice(t *testing.T) {
	a := All()
	if len(a) == 0 {
		t.Skip("catálogo vacío")
	}
	a[0].Collection = "otra"
	if All()[0].Collection == "otra" {
		t.Fatal("All devuelve un slice compartido")
	}
}
