package usecase

import (
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// SuggestPrice devuelve el precio sugerido por día para un valor declarado
// (RF-03, POL-04). No publica nada.
func (s *Service) SuggestPrice(declaredValueCents int64) (int64, error) {
	if !domain.ValidDeclaredValue(declaredValueCents) {
		return 0, shared.NewValidation(domain.MessageInvalidData, shared.Detail{
			Field: "declaredValueCents", Issue: "debe estar entre $50 y $100,000",
		})
	}
	return domain.SuggestPrice(declaredValueCents), nil
}
