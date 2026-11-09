package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
)

// IssueQR genera el QR de un solo uso de la etapa, vigente 15 minutos (RF-02,
// POL-06). Generar otro invalida el anterior. El token se devuelve una vez y
// solo se guarda su hash (RNF-01); nunca se registra en logs.
func (s *Service) IssueQR(ctx context.Context, v Viewer, bookingID string, k domain.Kind) (QRIssued, error) {
	if err := checkKind(k); err != nil {
		return QRIssued{}, err
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return QRIssued{}, err
	}
	if err := domain.CanIssueQR(b, k, v.ID); err != nil {
		return QRIssued{}, err
	}
	token, err := domain.NewToken(s.d.Rand)
	if err != nil {
		return QRIssued{}, err
	}
	qr := domain.NewQR(domain.Of(b, k), token, s.now())
	if err := s.d.Repo.SaveQR(ctx, b.ID, k, qr); err != nil {
		return QRIssued{}, s.explain(ctx, v, b.ID, k, err, domain.CanIssueQR)
	}
	return QRIssued{Token: token, Payload: domain.Payload(b.ID, k, token), ExpiresAt: qr.ExpiresAt}, nil
}
