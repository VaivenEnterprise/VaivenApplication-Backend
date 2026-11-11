// Package catalog reúne en un solo lugar las declaraciones de índices de los
// repositorios (spec 012, T004, plan §7.2). No construye repositorios ni
// ejecuta EnsureIndexes: solo concatena datos. Vive fuera de
// internal/adapter/mongo para no crear ciclos (los módulos importan aquel
// paquete por IndexSpec).
package catalog

import (
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

// sources son las funciones declarativas, una por repositorio con índices.
// La prueba de catálogo (go/parser) falla si un EnsureIndexes del proyecto
// no tiene su declaración aquí.
var sources = []func() []adaptermongo.IndexSpec{
	authmongo.UserIndexes,
	authmongo.RefreshTokenIndexes,
	itemsmongo.ItemIndexes,
	searchmongo.SearchIndexes,
	bookingsmongo.BookingIndexes,
	paymentsmongo.PaymentIndexes,
	chatmongo.ConversationIndexes,
	chatmongo.MessageIndexes,
	ratingsmongo.RatingIndexes,
	incidentsmongo.IncidentIndexes,
	sanctionsmongo.SanctionIndexes,
	notificationsmongo.NotificationIndexes,
	notificationsmongo.PushTokenIndexes,
	reportsmongo.ReportIndexes,
}

// All devuelve todos los índices declarados, en el orden de sources.
func All() []adaptermongo.IndexSpec {
	var out []adaptermongo.IndexSpec
	for _, src := range sources {
		out = append(out, src()...)
	}
	return out
}
