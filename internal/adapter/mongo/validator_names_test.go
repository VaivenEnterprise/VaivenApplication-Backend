package mongo_test

import (
	"slices"
	"testing"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	aimongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/adapter/mongo"
	assistantmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/adapter/mongo"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
)

// Los nombres de OptionalFieldSchemas se repiten como texto (este paquete no
// puede importar a los módulos); aquí se comprueba que coinciden con las
// constantes de cada módulo, para no crear colecciones con un nombre mal
// escrito. users, refresh_tokens e items tienen su propio esquema (auth e items).
func TestOptionalFieldSchemaNamesMatchModules(t *testing.T) {
	want := []string{
		bookingsmongo.BookingsCollection,
		chatmongo.ConversationsCollection, chatmongo.MessagesCollection,
		ratingsmongo.RatingsCollection,
		incidentsmongo.IncidentsCollection,
		reportsmongo.ReportsCollection,
		sanctionsmongo.SanctionsCollection,
		notificationsmongo.NotificationsCollection, notificationsmongo.PushTokensCollection,
		aimongo.UsageCollection, assistantmongo.ConversationsCollection,
		recomongo.InteractionsCollection, recomongo.MetaCollection, recomongo.ModelCollection, recomongo.PopularCollection,
	}
	var got []string
	for name := range adaptermongo.OptionalFieldSchemas() {
		got = append(got, name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("colecciones con validador de opcionales\n got: %v\nwant: %v", got, want)
	}
}
