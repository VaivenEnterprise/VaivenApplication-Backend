package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// messageInvalidData es el mensaje general de validation_failed.
const messageInvalidData = "Revisa los datos marcados."

// RegisterInput son los datos de registro (RF-01, POL-12).
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	AcceptTerms bool
	IsAdult     bool
}

// Register crea la cuenta (rol user) y abre la sesión. El correo se normaliza
// y debe ser único (RF-02); la contraseña se guarda con argon2id (RF-03).
func (s *Service) Register(ctx context.Context, in RegisterInput) (AuthResult, error) {
	email := domain.NormalizeEmail(in.Email)
	name := domain.NormalizeDisplayName(in.DisplayName)

	var details []shared.Detail
	details = append(details, domain.ValidateEmail(email)...)
	details = append(details, domain.ValidatePassword(in.Password)...)
	details = append(details, domain.ValidateDisplayName(name)...)
	if !in.AcceptTerms {
		details = append(details, shared.Detail{Field: "acceptTerms", Issue: "debes aceptar los términos y el aviso de privacidad"})
	}
	if !in.IsAdult {
		details = append(details, shared.Detail{Field: "isAdult", Issue: "debes ser mayor de edad"})
	}
	if len(details) > 0 {
		return AuthResult{}, shared.NewValidation(messageInvalidData, details...)
	}

	hash, err := s.d.Hasher.Hash(ctx, in.Password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("auth: registro: %w", err)
	}
	now := s.now()
	u, err := s.d.Users.Create(ctx, domain.User{
		Email:           email,
		PasswordHash:    hash,
		DisplayName:     name,
		Role:            domain.RoleUser,
		Status:          domain.StatusActive,
		AcceptedTermsAt: now,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	})
	if err != nil {
		if domain.Is(err, domain.ErrEmailTaken) {
			return AuthResult{}, domain.ErrEmailTaken
		}
		return AuthResult{}, fmt.Errorf("auth: registro: %w", err)
	}

	tokens, err := s.issueSession(ctx, u, "")
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: u, Tokens: tokens}, nil
}
