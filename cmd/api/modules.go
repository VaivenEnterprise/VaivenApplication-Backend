package main

import (
	"log/slog"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	bookingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/http"
	chathttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/http"
	handoverhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/adapter/http"
	imageshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/http"
	incidentshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/http"
	itemshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	notificationshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	paymentshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/adapter/http"
	ratingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/http"
	reportshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/http"
	sanctionshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/http"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// moduleDeps son las dependencias compartidas por los módulos. Los puertos
// compartidos (Art. XII.4) empiezan con su implementación vacía.
type moduleDeps struct {
	Log        *slog.Logger
	Notifier   notify.Notifier
	Moderation moderation.Checker

	// Auth es el módulo de la spec 003; Auth.Authenticate() protege rutas.
	Auth *authhttp.Module
	// Images es el módulo de la spec 004 (firmas de subida).
	Images *imageshttp.Module
	// Items es el módulo de la spec 005 (objetos).
	Items *itemshttp.Module
	// Search es el módulo de la spec 006 (búsqueda).
	Search *searchhttp.Module
	// Bookings es el módulo de la spec 007 (solicitudes y reservas).
	Bookings *bookingshttp.Module
	// Chat es el módulo de la spec 008 (chat privado).
	Chat *chathttp.Module
	// Handover es el módulo de la spec 009 (entrega y devolución).
	Handover *handoverhttp.Module
	// Payments es el módulo de pagos y retrasos de la spec 009.
	Payments *paymentshttp.Module
	// Ratings es el módulo de calificaciones de la spec 010.
	Ratings *ratingshttp.Module
	// Incidents es el módulo de incidencias de la spec 010.
	Incidents *incidentshttp.Module
	// Sanctions es el módulo de sanciones de la spec 010.
	Sanctions *sanctionshttp.Module
	// Notifications es el módulo de avisos y tokens de push de la spec 011.
	Notifications *notificationshttp.Module
	// Reports es el módulo de reportes y su revisión de la spec 011.
	Reports *reportshttp.Module
	// Users da perfiles públicos por lote y acciones de administración.
	Users interface {
		users.Directory
		users.Admin
	}
}

// modules devuelve los módulos de negocio que se montan bajo /api/v1 (spec 003,
// RF-11; Art. XII.3). Cada spec agrega su módulo con UNA línea en la lista y no
// edita las de los demás. deps reúne lo que los módulos comparten.
func modules(deps moduleDeps) []apihttp.Module {
	return []apihttp.Module{
		// Una línea por módulo, en orden de spec:
		deps.Auth,          // 003 cuenta y autenticación
		deps.Images,        // 004 imágenes
		deps.Items,         // 005 objetos
		deps.Search,        // 006 búsqueda
		deps.Bookings,      // 007 solicitudes y reservas
		deps.Chat,          // 008 chat privado
		deps.Handover,      // 009 entrega y devolución
		deps.Payments,      // 009 pagos y retrasos
		deps.Ratings,       // 010 calificaciones
		deps.Incidents,     // 010 incidencias
		deps.Sanctions,     // 010 sanciones
		deps.Notifications, // 011 avisos y push
		deps.Reports,       // 011 reportes y moderación
	}
}
