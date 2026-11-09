package usecase

import (
	"context"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

// MarkRead marca avisos como leídos (RF-03): all=true marca todos los de
// userID; si no, los de ids. Se exige uno u otro, no ambos ni ninguno. Los ids
// que no son del usuario o no existen se ignoran sin error (no se revela su
// existencia).
func (s *Service) MarkRead(ctx context.Context, userID string, ids []string, all bool) error {
	switch {
	case all && len(ids) > 0:
		return invalid("all", "no se puede enviar junto con ids")
	case !all && len(ids) == 0:
		return invalid("ids", "envía ids o all: true")
	case all:
		return s.d.Repo.MarkAllRead(ctx, userID, s.now())
	}
	if len(ids) > domain.MaxReadIDs {
		return invalid("ids", "máximo 100 ids")
	}
	for _, id := range ids {
		if !domain.IsObjectID(id) {
			return invalid("ids", "contiene un id inválido")
		}
	}
	return s.d.Repo.MarkRead(ctx, userID, ids, s.now())
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}
