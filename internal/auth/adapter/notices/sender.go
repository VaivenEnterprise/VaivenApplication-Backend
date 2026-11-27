// Package notices adapta el puerto compartido notify.Notifier al puerto
// usecase.NoticeSender de la baja de cuenta (spec 021): el aviso de un job de la
// bandeja de salida llega a la contraparte por el mismo camino que cualquier
// otro aviso. Lo comparten cmd/api y cmd/admin.
package notices

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// Sender implementa usecase.NoticeSender sobre notify.Notifier.
type Sender struct{ n notify.Notifier }

// New crea el adaptador. Falla si no hay notificador.
func New(n notify.Notifier) (*Sender, error) {
	if n == nil {
		return nil, errors.New("notices: falta el notificador")
	}
	return &Sender{n: n}, nil
}

// Send entrega el aviso. Un error se devuelve para que el job se reintente;
// notify.Service ignora sin error las cuentas inexistentes o dadas de baja.
func (s *Sender) Send(ctx context.Context, n usecase.Notice) error {
	return s.n.Notify(ctx, n.UserID, notify.Event{Type: n.Type, Title: n.Title, Body: n.Body, Data: n.Data})
}

// Comprobación en compilación del puerto de la baja.
var _ usecase.NoticeSender = (*Sender)(nil)
