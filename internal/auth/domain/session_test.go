package domain

import (
	"testing"
	"time"
)

func TestRefreshTokenState(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	at := now.Add(-time.Minute)
	tests := []struct {
		name           string
		tok            RefreshToken
		usable, reused bool
	}{
		{"vigente", RefreshToken{ExpiresAt: now.Add(time.Hour)}, true, false},
		{"vencido", RefreshToken{ExpiresAt: now}, false, false},
		{"usado", RefreshToken{ExpiresAt: now.Add(time.Hour), UsedAt: &at}, false, true},
		{"revocado", RefreshToken{ExpiresAt: now.Add(time.Hour), RevokedAt: &at}, false, false},
		{"usado y revocado", RefreshToken{ExpiresAt: now.Add(time.Hour), UsedAt: &at, RevokedAt: &at}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tok.Usable(now); got != tt.usable {
				t.Errorf("Usable = %v, want %v", got, tt.usable)
			}
			if got := tt.tok.Reused(); got != tt.reused {
				t.Errorf("Reused = %v, want %v", got, tt.reused)
			}
		})
	}
}
