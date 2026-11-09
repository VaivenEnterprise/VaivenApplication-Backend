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
	out := Result{Cards: make([]CardView, 0, len(page.Cards)), Next: page.Next}
	for _, c := range page.Cards {
		v := CardView{Card: c}
		if c.FirstPhoto != nil {
			u, err := s.d.Images.URLs(*c.FirstPhoto)
			if err != nil {
				return Result{}, fmt.Errorf("search: miniatura: %w", err)
			}
			v.ThumbURL = &u.ThumbURL
		}
		out.Cards = append(out.Cards, v)
	}
	return out, nil
}
