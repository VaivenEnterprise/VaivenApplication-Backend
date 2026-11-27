package mongo

import (
	"context"
	"errors"
	"fmt"

	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// TxRunner ejecuta una función dentro de una transacción de MongoDB (spec 021,
// RF-02). Cumple auth/usecase.TxRunner sin importarlo (Art. III): el ctx que
// recibe fn lleva la sesión, así que toda operación de los repositorios hecha
// con él entra en la misma transacción.
//
// Usa lectura snapshot y escritura majority. El driver repite fn cuando el
// servidor responde TransientTransactionError (p. ej. WriteConflict) y repite el
// commit ante UnknownTransactionCommitResult: fn debe poder ejecutarse más de
// una vez y no dejar estado fuera de la base de datos.
type TxRunner struct {
	client *drv.Client
}

// NewTxRunner crea el ejecutor sobre el cliente de db.
func NewTxRunner(db *drv.Database) *TxRunner { return &TxRunner{client: db.Client()} }

// Do ejecuta fn en una transacción. Si fn devuelve un error, la transacción se
// aborta y se devuelve ese mismo error sin envolver (así errors.Is y los
// errores de dominio siguen funcionando).
func (r *TxRunner) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return errors.New("transacción: falta la función")
	}
	sess, err := r.client.StartSession()
	if err != nil {
		return fmt.Errorf("transacción: iniciar sesión de MongoDB: %w", err)
	}
	defer sess.EndSession(ctx)
	_, err = sess.WithTransaction(ctx, func(tx context.Context) (any, error) {
		return nil, fn(tx)
	}, options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority()))
	return err
}
