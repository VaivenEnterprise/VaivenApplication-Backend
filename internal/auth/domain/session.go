package domain

import "time"

// RefreshToken es un refresh token guardado (plan P-03 y P-04). Solo se guarda
// el hash del token; el valor lo tiene únicamente el cliente. Todos los tokens
// que nacen de un mismo inicio de sesión comparten FamilyID: reutilizar uno ya
// usado revoca la familia entera (RF-05).
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Usable indica si el token aún puede canjearse en el instante now: no fue
// usado, no está revocado y no venció.
func (t RefreshToken) Usable(now time.Time) bool {
	return t.UsedAt == nil && t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

// Reused indica un token que ya se canjeó y llega otra vez: señal de robo
// (plan P-04). Un token revocado no cuenta: su familia ya está cerrada.
func (t RefreshToken) Reused() bool {
	return t.UsedAt != nil && t.RevokedAt == nil
}
