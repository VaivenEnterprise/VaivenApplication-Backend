package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // la imagen distroless no trae zoneinfo (America/Mexico_City)

	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// TimeZone es la zona de las fechas de calendario (plan P-02).
const TimeZone = "America/Mexico_City"

// Deps son las dependencias del Service.
type Deps struct {
	Repo   Repository
	Images imgport.Service
	// Now nil = time.Now.
	Now func() time.Time
	// Location nil = America/Mexico_City.
	Location *time.Location
}

// Service implementa la búsqueda.
type Service struct {
	d Deps
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Images == nil {
		return nil, errors.New("search: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		loc, err := time.LoadLocation(TimeZone)
		if err != nil {
			return nil, fmt.Errorf("search: zona horaria: %w", err)
		}
		d.Location = loc
	}
	return &Service{d: d}, nil
}

// Search valida los parámetros (RF-08), ejecuta la consulta y agrega la
// miniatura de cada tarjeta (RF-06). viewerID no ve sus propios objetos.
func (s *Service) Search(ctx context.Context, viewerID string, raw domain.RawQuery) (Result, error) {
	today := s.d.Now().In(s.d.Location).Format("2006-01-02")
	q, err := domain.Parse(raw, viewerID, today)
	if err != nil {
		return Result{}, err
	}
	page, err := s.d.Repo.Search(ctx, q)
	if err != nil {
		return Result{}, err
	}
	cards, err := s.views(page.Cards)
	if err != nil {
		return Result{}, err
	}
	return Result{Cards: cards, Next: page.Next}, nil
}

// MaxByIDs es el máximo de objetos que acepta CardsByIDs.
const MaxByIDs = 50

// CardsByIDs devuelve las tarjetas de ids que viewerID puede ver, en el orden
// pedido y con las mismas reglas de visibilidad que Search (spec 015, T005;
// plan P-06): activos, de otras personas y de cuentas activas y no
// suspendidas. Los ids inválidos o repetidos se ignoran y se usan como máximo
// MaxByIDs. Nunca devuelve nil.
func (s *Service) CardsByIDs(ctx context.Context, viewerID string, ids []string) ([]CardView, error) {
	valid := make([]string, 0, min(len(ids), MaxByIDs))
	seen := map[string]bool{}
	for _, id := range ids {
		if len(valid) == MaxByIDs {
			break
		}
		if domain.IsObjectIDHex(id) && !seen[id] {
			seen[id] = true
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return []CardView{}, nil
	}
	cards, err := s.d.Repo.CardsByIDs(ctx, viewerID, valid)
	if err != nil {
		return nil, err
	}
	return s.views(cards)
}

// views agrega la miniatura de la primera foto a cada tarjeta (RF-06).
func (s *Service) views(cards []domain.Card) ([]CardView, error) {
	out := make([]CardView, 0, len(cards))
	for _, c := range cards {
		v := CardView{Card: c}
		if c.FirstPhoto != nil {
			u, err := s.d.Images.URLs(*c.FirstPhoto)
			if err != nil {
				return nil, fmt.Errorf("search: miniatura: %w", err)
			}
			v.ThumbURL = &u.ThumbURL
		}
		out = append(out, v)
	}
	return out, nil
}
