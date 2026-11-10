package main

import (
	"context"
	"strings"
	"testing"
	"time"

	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	paymentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/adapter/mongo"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
	searchmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/mongo"
)

// namedEnsurer es un repositorio con índices y el prefijo de error que
// EnsureIndexes tenía antes del refactor de T004 (prueba 3).
type namedEnsurer struct {
	ensurer adaptermongo.IndexEnsurer
	prefix  string
}

// ensurersWithPrefix son los 14 repositorios con índices, construidos como en
// cmd/api. ratings no usa su puerto users.Admin para crear índices: va nil.
func ensurersWithPrefix(db *drv.Database) []namedEnsurer {
	return []namedEnsurer{
		{authmongo.NewUserRepo(db), "users: crear índices: "},
		{authmongo.NewRefreshRepo(db), "refresh_tokens: crear índices: "},
		{itemsmongo.NewItemRepo(db), "items: crear índices: "},
		{searchmongo.NewIndexes(db), "search: crear índices: "},
		{bookingsmongo.NewRepo(db), "bookings: crear índices: "},
		{chatmongo.NewConversationRepo(db), "chat: crear índices de conversaciones: "},
		{chatmongo.NewMessageRepo(db), "chat: crear índices de mensajes: "},
		{paymentsmongo.NewRepo(db), "payments: crear índice: "},
		{ratingsmongo.NewRepo(db, nil), "ratings: crear índices: "},
		{incidentsmongo.NewRepo(db), "incidents: crear índices: "},
		{sanctionsmongo.NewRepo(db), "sanctions: crear índices: "},
		{notificationsmongo.NewNotificationRepo(db), "notifications: crear índices: "},
		{notificationsmongo.NewTokenRepo(db), "push_tokens: crear índices: "},
		{reportsmongo.NewRepo(db), "reports: crear índices: "},
	}
}

func ensurers(db *drv.Database) []adaptermongo.IndexEnsurer {
	var out []adaptermongo.IndexEnsurer
	for _, e := range ensurersWithPrefix(db) {
		out = append(out, e.ensurer)
	}
	return out
}

// Prueba 3 (plan 012 §7.6): cada EnsureIndexes conserva su prefijo de error.
// No necesita servidor: el cliente del driver es perezoso y la creación
// falla al no poder seleccionar uno.
func TestEnsureIndexesKeepErrorPrefix(t *testing.T) {
	client, err := drv.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1").
		SetServerSelectionTimeout(50 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("sin_servidor")
	for _, e := range ensurersWithPrefix(db) {
		err := e.ensurer.EnsureIndexes(context.Background())
		if err == nil || !strings.HasPrefix(err.Error(), e.prefix) {
			t.Errorf("error = %v, want prefijo %q", err, e.prefix)
		}
	}
}
