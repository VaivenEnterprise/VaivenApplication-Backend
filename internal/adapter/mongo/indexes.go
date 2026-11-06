// Package mongo reúne los repositorios de MongoDB y el registro de sus índices
// (spec 002, plan P-06; Art. II). Los índices se declaran en código y se crean
// al arrancar, de forma idempotente.
package mongo

import (
	"context"
	"errors"
	"fmt"
)

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
