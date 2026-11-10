package mongo

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// IndexStatus clasifica un índice al comparar lo declarado con lo real
// (spec 012, plan §7.4).
type IndexStatus string

// Estados de la comparación. NOMBRE DISTINTO no es un estado: es una nota
// informativa (IndexFinding.SameShapeAs) sobre un FALTANTE y un SOBRANTE.
const (
	IndexOK           IndexStatus = "OK"
	IndexIncompatible IndexStatus = "INCOMPATIBLE"
	IndexMissing      IndexStatus = "FALTANTE"
	IndexExtra        IndexStatus = "SOBRANTE"
	IndexUnverifiable IndexStatus = "NO VERIFICABLE"
)

// IndexFinding es el resultado para un índice.
type IndexFinding struct {
	Collection string
	Name       string
	Status     IndexStatus
	// Detail explica la diferencia o la opción que no se pudo interpretar.
	Detail string
	// SameShapeAs, en un FALTANTE o un SOBRANTE, nombra el índice de la misma
	// colección que es estructuralmente equivalente con otro nombre (NOMBRE
	// DISTINTO). Es informativo: ninguno de los dos pasa a ser correcto.
	SameShapeAs string
}

// Discrepancy indica si el hallazgo debe hacer fallar la verificación.
func (f IndexFinding) Discrepancy() bool { return f.Status != IndexOK }

// idIndexName es el índice que MongoDB crea en toda colección.
const idIndexName = "_id_"

// serverMetadata son campos que agrega el servidor y no forman parte de la
// declaración. Las versiones internas solo se ignoran si no se declararon.
var serverMetadata = map[string]bool{"v": true, "ns": true}

var internalVersions = map[string]bool{"textIndexVersion": true, "2dsphereIndexVersion": true}

// knownOptions son las opciones que el comparador sabe interpretar. Una
// opción real fuera de esta lista deja el índice NO VERIFICABLE.
var knownOptions = map[string]bool{
	"key": true, "name": true, "unique": true, "sparse": true, "expireAfterSeconds": true,
	"partialFilterExpression": true, "weights": true, "default_language": true,
	"language_override": true, "textIndexVersion": true, "2dsphereIndexVersion": true,
	"collation": true, "hidden": true, "wildcardProjection": true,
	"bits": true, "min": true, "max": true,
}

// unorderedOptions se comparan sin importar el orden de sus campos: el
// servidor reordena los pesos y completa la collation.
var unorderedOptions = map[string]bool{"weights": true, "collation": true}

// textDefaults son los valores por defecto documentados de MongoDB para los
// índices de texto; solo se aplican a índices de texto declarados sin ellos.
var textDefaults = map[string]any{"default_language": "english", "language_override": "language"}

// expectedIndex es la forma de un IndexSpec tal como la devolvería listIndexes.
type expectedIndex struct {
	collection string
	name       string
	doc        bson.D // sin "name"
}

// ExpectedDocument convierte spec en el documento que MongoDB guardaría en
// listIndexes (sin el nombre): aplica las opciones del builder del driver
// sin perder ninguna y representa los índices de texto como _fts/_ftsx.
func ExpectedDocument(spec IndexSpec) (name string, doc bson.D, err error) {
	keys, ok := spec.Model.Keys.(bson.D)
	if !ok {
		return "", nil, fmt.Errorf("índices: %s: las claves deben ser bson.D (ordenadas), no %T", spec.Collection, spec.Model.Keys)
	}
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("índices: %s: índice sin claves", spec.Collection)
	}
	var o options.IndexOptions
	if spec.Model.Options != nil {
		for _, set := range spec.Model.Options.Opts {
			if err := set(&o); err != nil {
				return "", nil, fmt.Errorf("índices: %s: opción inválida: %w", spec.Collection, err)
			}
		}
	}
	if o.StorageEngine != nil || o.BucketSize != nil {
		return "", nil, fmt.Errorf("índices: %s: storageEngine y bucketSize no están soportados por el comparador", spec.Collection)
	}
	if o.Name != nil {
		name = *o.Name
	} else {
		name = defaultIndexName(keys)
	}

	key, textFields := textKey(keys)
	doc = bson.D{{Key: "key", Value: key}}
	add := func(k string, v any) { doc = append(doc, bson.E{Key: k, Value: v}) }
	if o.Unique != nil {
		add("unique", *o.Unique)
	}
	if o.Sparse != nil {
		add("sparse", *o.Sparse)
	}
	if o.ExpireAfterSeconds != nil {
		add("expireAfterSeconds", *o.ExpireAfterSeconds)
	}
	if o.PartialFilterExpression != nil {
		add("partialFilterExpression", o.PartialFilterExpression)
	}
	if len(textFields) > 0 {
		if o.Weights != nil {
			add("weights", o.Weights)
		} else {
			w := bson.D{}
			for _, f := range textFields {
				w = append(w, bson.E{Key: f, Value: int32(1)})
			}
			add("weights", w)
		}
		lang, override := textDefaults["default_language"], textDefaults["language_override"]
		if o.DefaultLanguage != nil {
			lang = *o.DefaultLanguage
		}
		if o.LanguageOverride != nil {
			override = *o.LanguageOverride
		}
		add("default_language", lang)
		add("language_override", override)
	} else if o.Weights != nil || o.DefaultLanguage != nil || o.LanguageOverride != nil {
		return "", nil, fmt.Errorf("índices: %s.%s: opciones de texto en un índice sin campos de texto", spec.Collection, name)
	}
	if o.TextVersion != nil {
		add("textIndexVersion", *o.TextVersion)
	}
	if o.SphereVersion != nil {
		add("2dsphereIndexVersion", *o.SphereVersion)
	}
	if o.Collation != nil {
		add("collation", collationDoc(o.Collation))
	}
	if o.Hidden != nil {
		add("hidden", *o.Hidden)
	}
	if o.WildcardProjection != nil {
		add("wildcardProjection", o.WildcardProjection)
	}
	if o.Bits != nil {
		add("bits", *o.Bits)
	}
	if o.Min != nil {
		add("min", *o.Min)
	}
	if o.Max != nil {
		add("max", *o.Max)
	}
	return name, doc, nil
}

// textKey representa los campos "text" como lo hace MongoDB: {_fts: "text",
// _ftsx: 1} en la posición del primer campo de texto. Devuelve también los
// campos de texto, para los pesos por defecto.
func textKey(keys bson.D) (bson.D, []string) {
	var out bson.D
	var fields []string
	for _, e := range keys {
		if s, ok := e.Value.(string); ok && s == "text" {
			if len(fields) == 0 {
				out = append(out, bson.E{Key: "_fts", Value: "text"}, bson.E{Key: "_ftsx", Value: int32(1)})
			}
			fields = append(fields, e.Key)
			continue
		}
		out = append(out, e)
	}
	return out, fields
}

// defaultIndexName reproduce el nombre que MongoDB asigna si no se declara:
// campo_valor unidos por "_".
func defaultIndexName(keys bson.D) string {
	parts := make([]string, 0, 2*len(keys))
	for _, e := range keys {
		parts = append(parts, e.Key, fmt.Sprint(e.Value))
	}
	return strings.Join(parts, "_")
}

// collationDoc usa los nombres de campo de MongoDB (no los de la etiqueta
// bson del driver). Solo incluye lo declarado.
func collationDoc(c *options.Collation) bson.D {
	d := bson.D{}
	add := func(k string, v any, set bool) {
		if set {
			d = append(d, bson.E{Key: k, Value: v})
		}
	}
	add("locale", c.Locale, c.Locale != "")
	add("caseLevel", c.CaseLevel, c.CaseLevel)
	add("caseFirst", c.CaseFirst, c.CaseFirst != "")
	add("strength", int64(c.Strength), c.Strength != 0)
	add("numericOrdering", c.NumericOrdering, c.NumericOrdering)
	add("alternate", c.Alternate, c.Alternate != "")
	add("maxVariable", c.MaxVariable, c.MaxVariable != "")
	add("normalization", c.Normalization, c.Normalization)
	add("backwards", c.Backwards, c.Backwards)
	return d
}

// CompareIndexes compara lo declarado (expected) con lo que devuelve
// listIndexes por colección (actual; una colección ausente del mapa no
// existe). No escribe nada: solo clasifica. Devuelve error si una
// declaración no se puede interpretar.
func CompareIndexes(expected []IndexSpec, actual map[string][]bson.D) ([]IndexFinding, error) {
	byCol := map[string][]expectedIndex{}
	seen := map[string]bool{}
	var errs []error
	for _, s := range expected {
		name, doc, err := ExpectedDocument(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		id := s.Collection + "." + name
		if seen[id] {
			errs = append(errs, fmt.Errorf("índices: %s declarado dos veces", id))
			continue
		}
		seen[id] = true
		byCol[s.Collection] = append(byCol[s.Collection], expectedIndex{collection: s.Collection, name: name, doc: doc})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	cols := map[string]bool{}
	for c := range byCol {
		cols[c] = true
	}
	for c := range actual {
		cols[c] = true
	}
	names := make([]string, 0, len(cols))
	for c := range cols {
		names = append(names, c)
	}
	sort.Strings(names)

	var out []IndexFinding
	for _, col := range names {
		out = append(out, compareCollection(col, byCol[col], actual[col])...)
	}
	return out, nil
}

func compareCollection(col string, want []expectedIndex, have []bson.D) []IndexFinding {
	realByName := map[string]bson.D{}
	var realNames []string
	for _, d := range have {
		n, _ := lookup(d, "name").(string)
		if n == idIndexName {
			continue
		}
		realByName[n] = d
		realNames = append(realNames, n)
	}

	var out []IndexFinding
	used := map[string]bool{}
	for _, w := range want {
		r, ok := realByName[w.name]
		if !ok {
			out = append(out, IndexFinding{Collection: col, Name: w.name, Status: IndexMissing, Detail: "no existe un índice con este nombre"})
			continue
		}
		used[w.name] = true
		status, detail := compareDoc(w.doc, r)
		out = append(out, IndexFinding{Collection: col, Name: w.name, Status: status, Detail: detail})
	}
	sort.Strings(realNames)
	for _, n := range realNames {
		if !used[n] {
			out = append(out, IndexFinding{Collection: col, Name: n, Status: IndexExtra, Detail: "existe y no está declarado"})
		}
	}

	// NOMBRE DISTINTO (informativo): un faltante y un sobrante con la misma
	// estructura. Ambos siguen siendo discrepancias.
	for i := range out {
		if out[i].Status != IndexMissing {
			continue
		}
		var wdoc bson.D
		for _, w := range want {
			if w.name == out[i].Name {
				wdoc = w.doc
			}
		}
		for j := range out {
			if out[j].Status != IndexExtra || out[j].SameShapeAs != "" {
				continue
			}
			if st, _ := compareDoc(wdoc, realByName[out[j].Name]); st == IndexOK {
				out[i].SameShapeAs = out[j].Name
				out[j].SameShapeAs = out[i].Name
				break
			}
		}
	}
	return out
}

// compareDoc compara la forma declarada con un documento de listIndexes,
// ignorando el nombre y los metadatos del servidor.
func compareDoc(want, have bson.D) (IndexStatus, string) {
	declared := map[string]bool{}
	for _, e := range want {
		declared[e.Key] = true
	}
	got := bson.D{}
	var unknown []string
	for _, e := range have {
		switch {
		case e.Key == "name" || serverMetadata[e.Key]:
			continue
		case internalVersions[e.Key] && !declared[e.Key]:
			continue
		case !knownOptions[e.Key]:
			unknown = append(unknown, e.Key)
			continue
		}
		got = append(got, e)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return IndexUnverifiable, "opciones que el comparador no sabe interpretar: " + strings.Join(unknown, ", ")
	}

	var diffs []string
	keys := unionKeys(want, got)
	collationOnlyExtra := false
	for _, k := range keys {
		wv, wok := lookupOK(want, k)
		gv, gok := lookupOK(got, k)
		switch {
		case !wok:
			diffs = append(diffs, fmt.Sprintf("%s: no declarado, real %s", k, show(gv)))
		case !gok:
			diffs = append(diffs, fmt.Sprintf("%s: declarado %s, ausente en la base", k, show(wv)))
		case !equalValue(wv, gv, !unorderedOptions[k]):
			if k == "collation" && containsAll(gv, wv) {
				collationOnlyExtra = true
				continue
			}
			diffs = append(diffs, fmt.Sprintf("%s: declarado %s, real %s", k, show(wv), show(gv)))
		}
	}
	if len(diffs) > 0 {
		return IndexIncompatible, strings.Join(diffs, "; ")
	}
	if collationOnlyExtra {
		return IndexUnverifiable, "collation: el servidor completó valores por defecto que no se declararon"
	}
	return IndexOK, ""
}

func unionKeys(a, b bson.D) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range []bson.D{a, b} {
		for _, e := range d {
			if !seen[e.Key] {
				seen[e.Key] = true
				out = append(out, e.Key)
			}
		}
	}
	return out
}

func lookup(d bson.D, key string) any {
	v, _ := lookupOK(d, key)
	return v
}

func lookupOK(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// equalValue compara valores BSON: los números son iguales si su valor lo es
// (int32, int64, double); los documentos respetan el orden cuando ordered es
// true (claves del índice, filtros parciales).
func equalValue(a, b any, ordered bool) bool {
	if fa, ok := number(a); ok {
		fb, ok := number(b)
		return ok && fa == fb
	}
	if da, ok := asDoc(a); ok {
		db, ok := asDoc(b)
		if !ok || len(da) != len(db) {
			return false
		}
		if ordered {
			for i := range da {
				if da[i].Key != db[i].Key || !equalValue(da[i].Value, db[i].Value, ordered) {
					return false
				}
			}
			return true
		}
		for _, e := range da {
			v, ok := lookupOK(db, e.Key)
			if !ok || !equalValue(e.Value, v, ordered) {
				return false
			}
		}
		return true
	}
	if aa, ok := asArray(a); ok {
		ab, ok := asArray(b)
		if !ok || len(aa) != len(ab) {
			return false
		}
		for i := range aa {
			if !equalValue(aa[i], ab[i], ordered) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// containsAll indica si el documento real incluye todos los campos declarados
// con el mismo valor (para la collation completada por el servidor).
func containsAll(have, declared any) bool {
	dr, ok1 := asDoc(have)
	dd, ok2 := asDoc(declared)
	if !ok1 || !ok2 {
		return false
	}
	for _, e := range dd {
		v, ok := lookupOK(dr, e.Key)
		if !ok || !equalValue(e.Value, v, false) {
			return false
		}
	}
	return true
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}

func asDoc(v any) (bson.D, bool) {
	switch d := v.(type) {
	case bson.D:
		return d, true
	case bson.M:
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(bson.D, 0, len(d))
		for _, k := range keys {
			out = append(out, bson.E{Key: k, Value: d[k]})
		}
		return out, true
	case map[string]any:
		return asDoc(bson.M(d))
	}
	return nil, false
}

func asArray(v any) ([]any, bool) {
	switch a := v.(type) {
	case bson.A:
		return []any(a), true
	case []any:
		return a, true
	}
	return nil, false
}

// show presenta un valor para el detalle (estructura de índices, no datos de
// personas: no lleva secretos).
func show(v any) string {
	b, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: v}}, false, false)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	s = strings.TrimPrefix(s, `{"v":`)
	return strings.TrimSuffix(s, "}")
}
