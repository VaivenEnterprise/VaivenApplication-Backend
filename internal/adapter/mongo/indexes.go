// Package mongo reúne los repositorios de MongoDB y el registro de sus índices
// (spec 002, plan P-06; Art. II). Los índices se declaran en código y se crean
// al arrancar, de forma idempotente.
package mongo

import (
	"context"
	"errors"
	"fmt"

	drv "go.mongodb.org/mongo-driver/v2/mongo"
)

// IndexSpec declara un índice de una colección (spec 012, plan §7.1). Usa el
// IndexModel del driver para no perder ninguna opción (texto, 2dsphere,
// filtro parcial, TTL, collation…). Es la fuente única de verdad: la consumen
// EnsureIndexes, el catálogo y el comando `admin indexes`.
type IndexSpec struct {
	Collection string
	Model      drv.IndexModel
}

// IndexDeclarer lo implementa cada repositorio que tiene índices. Indexes
// devuelve datos puros: no abre conexiones ni depende de infraestructura.
type IndexDeclarer interface {
	Indexes() []IndexSpec
}

// SpecsFor declara varios índices de una misma colección, en el orden dado.
func SpecsFor(collection string, models ...drv.IndexModel) []IndexSpec {
	out := make([]IndexSpec, len(models))
	for i, m := range models {
		out[i] = IndexSpec{Collection: collection, Model: m}
	}
	return out
}

// Models devuelve los IndexModel de specs, en el mismo orden, para pasarlos a
// CreateMany.
func Models(specs []IndexSpec) []drv.IndexModel {
	out := make([]drv.IndexModel, len(specs))
	for i, s := range specs {
		out[i] = s.Model
	}
	return out
}

// IndexEnsurer lo implementa cada repositorio que necesita índices. Debe ser
// idempotente: crear un índice que ya existe no es un error.
type IndexEnsurer interface {
	EnsureIndexes(ctx context.Context) error
}

// EnsureAll ejecuta EnsureIndexes de cada ensurer, en orden. No se detiene en
// el primer fallo: sigue con los demás y devuelve todos los errores juntos
// (cada uno indica la posición del ensurer). Sin ensurers no hace nada.
// Es idempotente siempre que lo sean los ensurers, por lo que main puede
// llamarlo en cada arranque.
func EnsureAll(ctx context.Context, ensurers ...IndexEnsurer) error {
	var errs []error
	for i, e := range ensurers {
		if e == nil {
			errs = append(errs, fmt.Errorf("índices: el ensurer %d es nil", i))
			continue
		}
		if err := e.EnsureIndexes(ctx); err != nil {
			errs = append(errs, fmt.Errorf("índices: ensurer %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}
