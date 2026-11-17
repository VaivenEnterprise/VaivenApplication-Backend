package usecase

import (
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// URLs devuelve url y thumbUrl de una imagen guardada (RF-07): públicas con
// transformación o, para evidencias, URL firmada que caduca a los 10 min.
func (s *Service) URLs(img domain.Image) (domain.URLs, error) {
	u, err := s.d.URLs.URLs(img)
	if err != nil {
		return domain.URLs{}, fmt.Errorf("images: urls: %w", err)
	}
	return u, nil
}
