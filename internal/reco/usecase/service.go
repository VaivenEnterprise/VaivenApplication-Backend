package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Deps son las dependencias del Service.
type Deps struct {
	Interactions Interactions
	Requests     Requests
	Items        Items
	Model        Model
	Meta         Meta
	Cards        Cards
	Log          *slog.Logger
	// Enabled es RECO_ENABLED (RF-10). Apagado, todo responde ErrDisabled.
	Enabled bool
	// Now nil = time.Now.
	Now func() time.Time
	// NewID genera ids de modelo y de dueño del candado; nil = 16 bytes al azar.
	NewID func() string
	// Go lanza el recálculo perezoso aparte de la petición; nil = una gorrutina.
	Go func(func())
}

// Service implementa las recomendaciones.
type Service struct {
	d Deps
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Interactions == nil || d.Requests == nil || d.Items == nil || d.Model == nil || d.Meta == nil || d.Cards == nil || d.Log == nil {
		return nil, errors.New("reco: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewID == nil {
		d.NewID = randomID
	}
	if d.Go == nil {
		d.Go = func(f func()) { go f() }
	}
	return &Service{d: d}, nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read no falla en las plataformas soportadas
	return hex.EncodeToString(b)
}

// Enabled indica si RECO_ENABLED está encendido (para /meta).
func (s *Service) Enabled() bool { return s.d.Enabled }

// RecordView registra que userID abrió el detalle de itemID (RF-01). Un objeto
// propio o no activo se ignora sin error, y una vista repetida en el mismo
// bloque de 10 min no cuenta otra vez. Un objeto inexistente es
// ErrItemNotFound.
func (s *Service) RecordView(ctx context.Context, userID, itemID, rawType string) error {
	if !s.d.Enabled {
		return domain.ErrDisabled
	}
	e, err := domain.NewView(userID, itemID, rawType, s.d.Now())
	if err != nil {
		return err
	}
	owner, status, found, err := s.d.Items.ItemState(ctx, itemID)
	switch {
	case err != nil:
		return err
	case !found:
		return domain.ErrItemNotFound
	case owner == userID || status != "active":
		return nil
	}
	_, err = s.d.Interactions.Record(ctx, e)
	return err
}

// DeleteHistory borra los eventos de userID y anota la fecha, para que sus
// solicitudes anteriores dejen de contar (RF-09, CA-05). Es idempotente.
func (s *Service) DeleteHistory(ctx context.Context, userID string) error {
	if !s.d.Enabled {
		return domain.ErrDisabled
	}
	if err := s.d.Interactions.DeleteUser(ctx, userID); err != nil {
		return err
	}
	return s.d.Meta.SetReset(ctx, userID, s.d.Now())
}
