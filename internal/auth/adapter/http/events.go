package http

import (
	"context"
	"log/slog"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

// SlogEvents registra los eventos de seguridad del caso de uso (spec 018,
// C-02) con el formato común (middleware.LogSecurityEvent).
type SlogEvents struct{ Log *slog.Logger }

// Record implementa usecase.SecurityEvents.
func (s SlogEvents) Record(ctx context.Context, e usecase.SecurityEvent) {
	middleware.LogSecurityEvent(ctx, s.Log, e.Name, e.Result, e.Severity, "auth", e.Subject)
}
