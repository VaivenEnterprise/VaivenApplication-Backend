package main

// Pruebas de seguridad por ruta generadas desde el contrato (spec 020, T003;
// RF-14, RF-15, RF-16; CA-09, CA-10).
//
// El router es el real: NewRouter con la lista de módulos de modules.go y el
// middleware real de sesión y de rol. Solo los casos de uso son dobles vacíos
// (una interfaz embebida en nil): si una ruta protegida llegara al caso de
// uso sin pasar por la sesión, la prueba entraría en pánico y fallaría.
// Así, una ruta nueva sin Authenticate, o una operación nueva del contrato sin
// clasificar en routeClasses, rompe la CI.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/pb33f/libopenapi"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/contract"
	assistanthttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/adapter/http"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	bookingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/http"
	chathttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/http"
	handoverhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/adapter/http"
	imageshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/http"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	incidentshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/http"
	itemshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	notificationshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	paymentshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	ratingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/http"
	recohttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/http"
	reportshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/http"
	sanctionshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/http"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	suggesthttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Clases de la tabla de clasificación (RF-15).
const (
	classPublic = "public" // sin sesión por diseño (contrato: security vacío)
	classAuth   = "auth"   // cualquier sesión válida
	classOwner  = "owner"  // sesión y recurso de una persona: otra recibe 403/404 (IDOR)
	classAdmin  = "admin"  // sesión y rol admin: un usuario recibe 403
)

// routeClasses clasifica CADA operación del contrato. Una operación nueva sin
// entrada aquí, o una entrada sin operación, hace fallar la prueba. Las pruebas
// de IDOR por módulo (bookings, items, chat y notifications) están en el
// archivo idor_integration_test.go de cada módulo.
var routeClasses = map[string]string{
	"GET /healthz": classPublic,
	"GET /readyz":  classPublic,

	"GET /api/v1/meta":           classPublic,
	"POST /api/v1/auth/login":    classPublic,
	"POST /api/v1/auth/logout":   classPublic, // el refresh token va en el cuerpo
	"POST /api/v1/auth/refresh":  classPublic,
	"POST /api/v1/auth/register": classPublic,

	"GET /api/v1/me":                                 classAuth,
	"PATCH /api/v1/me":                               classAuth,
	"DELETE /api/v1/me":                              classAuth,
	"GET /api/v1/users/{id}":                         classAuth,
	"GET /api/v1/users/{id}/ratings":                 classAuth,
	"GET /api/v1/categories":                         classAuth,
	"GET /api/v1/items":                              classAuth,
	"GET /api/v1/items/price-suggestion":             classAuth,
	"GET /api/v1/items/{id}":                         classAuth,
	"GET /api/v1/items/{id}/quote":                   classAuth,
	"POST /api/v1/items":                             classAuth,
	"GET /api/v1/me/items":                           classAuth,
	"POST /api/v1/uploads/signature":                 classAuth,
	"POST /api/v1/bookings":                          classAuth,
	"GET /api/v1/me/bookings":                        classAuth,
	"GET /api/v1/conversations":                      classAuth,
	"GET /api/v1/conversations/unread-count":         classAuth,
	"GET /api/v1/me/incidents":                       classAuth,
	"GET /api/v1/me/notifications":                   classAuth,
	"GET /api/v1/me/notifications/unread-count":      classAuth,
	"POST /api/v1/me/notifications/read":             classAuth,
	"POST /api/v1/me/push-tokens":                    classAuth,
	"DELETE /api/v1/me/push-tokens":                  classAuth,
	"POST /api/v1/reports":                           classAuth,
	"GET /api/v1/recommendations":                    classAuth,
	"POST /api/v1/recommendations/events":            classAuth,
	"DELETE /api/v1/recommendations/history":         classAuth,
	"POST /api/v1/ai/listing-suggestions":            classAuth,
	"POST /api/v1/ai/assistant/messages":             classAuth,
	"DELETE /api/v1/ai/assistant/conversations/{id}": classOwner,

	"PATCH /api/v1/items/{id}":       classOwner,
	"DELETE /api/v1/items/{id}":      classOwner,
	"POST /api/v1/items/{id}/pause":  classOwner,
	"POST /api/v1/items/{id}/resume": classOwner,

	"GET /api/v1/bookings/{id}":                           classOwner,
	"POST /api/v1/bookings/{id}/accept":                   classOwner,
	"POST /api/v1/bookings/{id}/reject":                   classOwner,
	"POST /api/v1/bookings/{id}/cancel":                   classOwner,
	"POST /api/v1/bookings/{id}/meeting-point":            classOwner,
	"GET /api/v1/bookings/{id}/delivery":                  classOwner,
	"POST /api/v1/bookings/{id}/delivery/qr":              classOwner,
	"POST /api/v1/bookings/{id}/delivery/confirm":         classOwner,
	"PUT /api/v1/bookings/{id}/delivery/evidence":         classOwner,
	"GET /api/v1/bookings/{id}/return":                    classOwner,
	"POST /api/v1/bookings/{id}/return/qr":                classOwner,
	"POST /api/v1/bookings/{id}/return/confirm":           classOwner,
	"PUT /api/v1/bookings/{id}/return/evidence":           classOwner,
	"POST /api/v1/bookings/{id}/payments/{stage}/confirm": classOwner,
	"POST /api/v1/bookings/{id}/payments/{stage}/report":  classOwner,
	"POST /api/v1/bookings/{id}/ratings":                  classOwner,
	"GET /api/v1/bookings/{id}/rating-status":             classOwner,
	"POST /api/v1/bookings/{id}/incidents":                classOwner,
	"GET /api/v1/incidents/{id}":                          classOwner,
	"GET /api/v1/conversations/{id}/messages":             classOwner,
	"POST /api/v1/conversations/{id}/messages":            classOwner,
	"POST /api/v1/conversations/{id}/read":                classOwner,

	"GET /api/v1/admin/incidents":             classAdmin,
	"POST /api/v1/admin/incidents/{id}/close": classAdmin,
	"POST /api/v1/admin/incidents/{id}/notes": classAdmin,
	"GET /api/v1/admin/reports":               classAdmin,
	"POST /api/v1/admin/reports/{id}/resolve": classAdmin,
	"POST /api/v1/admin/users/{id}/sanctions": classAdmin,
}

// Dobles vacíos: la interfaz embebida es nil y nunca debe llamarse.
type (
	stubAuth          struct{ authhttp.Service }
	stubImages        struct{ imgport.Service }
	stubItems         struct{ itemshttp.Service }
	stubSearch        struct{ searchhttp.Service }
	stubBookings      struct{ bookingshttp.Service }
	stubChat          struct{ chathttp.Service }
	stubHandover      struct{ handoverhttp.Service }
	stubPayments      struct{ paymentshttp.Service }
	stubRatings       struct{ ratingshttp.Service }
	stubIncidents     struct{ incidentshttp.Service }
	stubSanctions     struct{ sanctionshttp.Service }
	stubNotifications struct{ notificationshttp.Service }
	stubReports       struct{ reportshttp.Service }
	stubSuggest       struct{ suggesthttp.Suggester }
	stubAssistant     struct{ assistanthttp.Assistant }
	stubReco          struct{ recohttp.Recommender }
	stubUsers         struct {
		users.Directory
		users.Admin
	}
)

const testSecret = "0123456789abcdef0123456789abcdef-secure-test-secret"

// securityRouter arma el router real con todos los módulos y dobles vacíos.
func securityRouter(t *testing.T) (http.Handler, *security.JWT) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwt, err := security.NewJWT(testSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	lim := redis.NewMemoryLimiter()
	authModule, err := authhttp.NewModule(authhttp.ModuleDeps{
		Service: stubAuth{}, Tokens: jwt, Log: log, Limiter: lim, LimiterFallback: lim, LimitPerMinute: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	authn := authModule.Authenticate()
	lookup := func(_ context.Context, _ string) (authdomain.User, error) {
		return authdomain.User{}, authdomain.ErrUserNotFound
	}
	active := authhttp.RequireActive(lookup, nil)
	admin := authhttp.RequireRole(authdomain.RoleAdmin, lookup, log)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	imagesMod, err := imageshttp.NewModule(imageshttp.ModuleDeps{Service: stubImages{}, Authenticate: authn, Log: log, Limiter: lim, LimiterFallback: lim, SignaturesPerHour: 1_000_000})
	must(err)
	itemsMod, err := itemshttp.NewModule(itemshttp.ModuleDeps{Service: stubItems{}, Authenticate: authn, RequireActive: active, Log: log})
	must(err)
	searchMod, err := searchhttp.NewModule(searchhttp.ModuleDeps{Service: stubSearch{}, Authenticate: authn, Log: log})
	must(err)
	bookingsMod, err := bookingshttp.NewModule(bookingshttp.ModuleDeps{Service: stubBookings{}, Authenticate: authn, RequireActive: active, Log: log})
	must(err)
	chatMod, err := chathttp.NewModule(chathttp.ModuleDeps{Service: stubChat{}, Authenticate: authn, Log: log, Limiter: lim, LimiterFallback: lim})
	must(err)
	handoverMod, err := handoverhttp.NewModule(handoverhttp.ModuleDeps{Service: stubHandover{}, Authenticate: authn, WriteBooking: bookingsMod.WriteBooking, Log: log})
	must(err)
	paymentsMod, err := paymentshttp.NewModule(paymentshttp.ModuleDeps{Service: stubPayments{}, Authenticate: authn, WriteBooking: bookingsMod.WriteBooking, Log: log})
	must(err)
	ratingsMod, err := ratingshttp.NewModule(ratingshttp.ModuleDeps{Service: stubRatings{}, Authenticate: authn, Log: log})
	must(err)
	incidentsMod, err := incidentshttp.NewModule(incidentshttp.ModuleDeps{Service: stubIncidents{}, Authenticate: authn, RequireAdmin: admin, Log: log})
	must(err)
	sanctionsMod, err := sanctionshttp.NewModule(sanctionshttp.ModuleDeps{Service: stubSanctions{}, Authenticate: authn, RequireAdmin: admin, Log: log})
	must(err)
	notificationsMod, err := notificationshttp.NewModule(notificationshttp.ModuleDeps{Service: stubNotifications{}, Authenticate: authn, Log: log})
	must(err)
	reportsMod, err := reportshttp.NewModule(reportshttp.ModuleDeps{Service: stubReports{}, Authenticate: authn, RequireAdmin: admin, Log: log})
	must(err)
	suggestMod, err := suggesthttp.NewModule(suggesthttp.ModuleDeps{Service: stubSuggest{}, Authenticate: authn, Log: log, Limiter: lim, LimiterFallback: lim})
	must(err)
	assistantMod, err := assistanthttp.NewModule(assistanthttp.ModuleDeps{Service: stubAssistant{}, Authenticate: authn, Log: log, Limiter: lim, LimiterFallback: lim})
	must(err)
	recoMod, err := recohttp.NewModule(recohttp.ModuleDeps{Service: stubReco{}, Authenticate: authn, Log: log, Limiter: lim, LimiterFallback: lim})
	must(err)

	mods := modules(moduleDeps{
		Log: log, Notifier: notify.Noop{}, Moderation: moderation.AllowAll{},
		Auth: authModule, Images: imagesMod, Items: itemsMod, Search: searchMod, Bookings: bookingsMod,
		Chat: chatMod, Handover: handoverMod, Payments: paymentsMod, Ratings: ratingsMod, Incidents: incidentsMod,
		Sanctions: sanctionsMod, Notifications: notificationsMod, Reports: reportsMod,
		Suggest: suggestMod, Assistant: assistantMod, Reco: recoMod, Users: stubUsers{},
	})
	return apihttp.NewRouter(apihttp.Deps{Log: log, Modules: mods}), jwt
}

// operation es una operación del contrato con su exigencia de sesión efectiva.
type operation struct {
	key       string // «POST /api/v1/bookings/{id}/reject»
	method    string
	path      string // con la plantilla
	protected bool
}

// contractOperations lee el contrato empaquetado. La seguridad global (bearer)
// aplica a toda operación que no declare la suya; «security: []» la deja pública.
func contractOperations(t *testing.T) []operation {
	t.Helper()
	raw, err := os.ReadFile(contract.SpecPath())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := libopenapi.NewDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	m, errs := doc.BuildV3Model()
	if errs != nil {
		t.Fatal(errs)
	}
	global := len(m.Model.Security) > 0
	var out []operation
	for p, item := range m.Model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			protected := global
			if op.Security != nil {
				protected = len(op.Security) > 0
			}
			out = append(out, operation{key: strings.ToUpper(method) + " " + p, method: strings.ToUpper(method), path: p, protected: protected})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// concrete sustituye los parámetros de ruta por valores con la forma válida.
func concrete(path string) string {
	r := strings.NewReplacer("{id}", "6731a2b3c4d5e6f7a8b9c0d1", "{stage}", "delivery")
	return r.Replace(path)
}

func do(h http.Handler, method, path, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	} `json:"error"`
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, quería %d (cuerpo %q)", rec.Code, status, rec.Body.String())
	}
	var b errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("el cuerpo no es el sobre de error estándar: %v (%q)", err, rec.Body.String())
	}
	if b.Error.Code != code || b.Error.Message == "" {
		t.Fatalf("error = %+v, quería code=%q con mensaje", b.Error, code)
	}
}

// TestEveryOperationIsClassified: la tabla y el contrato coinciden (CA-09).
func TestEveryOperationIsClassified(t *testing.T) {
	ops := contractOperations(t)
	if len(ops) < 69 {
		t.Fatalf("el contrato tiene %d operaciones; se esperaban al menos 69", len(ops))
	}
	seen := map[string]bool{}
	for _, op := range ops {
		seen[op.key] = true
		class, ok := routeClasses[op.key]
		if !ok {
			t.Errorf("operación sin clasificar: %s (agrégala a routeClasses)", op.key)
			continue
		}
		if (class == classPublic) == op.protected {
			t.Errorf("%s: clase %q no coincide con el contrato (protegida=%v)", op.key, class, op.protected)
		}
		if isAdminPath := strings.Contains(op.path, "/admin/"); isAdminPath != (class == classAdmin) {
			t.Errorf("%s: las rutas /admin/ deben ser de clase %q y solo ellas", op.key, classAdmin)
		}
	}
	for key := range routeClasses {
		if !seen[key] {
			t.Errorf("routeClasses tiene %q, que no está en el contrato", key)
		}
	}
}

// TestProtectedOperationsRequireSession: sin sesión, o con una inválida, toda
// operación protegida responde 401 con el sobre estándar (RF-14).
func TestProtectedOperationsRequireSession(t *testing.T) {
	h, jwt := securityRouter(t)
	h = contract.Wrap(t, h, "")

	otherJWT, err := security.NewJWT("otro-secreto-0123456789abcdef0123456789abcdef", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := otherJWT.Issue("6731a2b3c4d5e6f7a8b9c0d1", "user")
	if err != nil {
		t.Fatal(err)
	}
	// Token con firma y claims correctos, pero vencido hace una hora.
	past := time.Now().Add(-time.Hour)
	expired, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"iss": security.Issuer, "aud": security.Audience, "sub": "6731a2b3c4d5e6f7a8b9c0d1", "role": "user",
		"jti": "abc", "iat": past.Add(-time.Minute).Unix(), "exp": past.Unix(),
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := jwt.Issue("6731a2b3c4d5e6f7a8b9c0d1", "user")
	if err != nil {
		t.Fatal(err)
	}
	// «alg: none» sin firma, con las reclamaciones de un token legítimo.
	parts := strings.Split(valid, ".")
	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	noneToken := noneHeader + "." + parts[1] + "."

	attempts := map[string]string{
		"sin cabecera":       "",
		"esquema Basic":      "Basic dXNlcjpwYXNz",
		"Bearer vacío":       "Bearer ",
		"token basura":       "Bearer no-es-un-jwt",
		"firma de otro":      "Bearer " + foreign,
		"token vencido":      "Bearer " + expired,
		"alg none":           "Bearer " + noneToken,
		"firma alterada":     "Bearer " + valid[:len(valid)-2] + "xx",
		"sin esquema Bearer": valid,
	}
	protected := 0
	for _, op := range contractOperations(t) {
		if !op.protected {
			continue
		}
		protected++
		for name, authz := range attempts {
			t.Run(op.key+"/"+name, func(t *testing.T) {
				assertError(t, do(h, op.method, concrete(op.path), authz), http.StatusUnauthorized, "unauthorized")
			})
		}
	}
	if protected < 60 {
		t.Fatalf("solo %d operaciones protegidas; ¿se leyó mal el contrato?", protected)
	}
}

// TestAdminOperationsRejectUsers: una sesión válida de persona usuaria recibe
// 403 en toda operación de administración (RF-15, rol insuficiente).
func TestAdminOperationsRejectUsers(t *testing.T) {
	h, jwt := securityRouter(t)
	h = contract.Wrap(t, h, "")
	tok, err := jwt.Issue("6731a2b3c4d5e6f7a8b9c0d1", "user")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, op := range contractOperations(t) {
		if routeClasses[op.key] != classAdmin {
			continue
		}
		n++
		t.Run(op.key, func(t *testing.T) {
			assertError(t, do(h, op.method, concrete(op.path), "Bearer "+tok), http.StatusForbidden, "forbidden")
		})
	}
	if n != 6 {
		t.Fatalf("operaciones de administración = %d, se esperaban 6", n)
	}
}

// TestKnownGapRejectAndCancelWithoutSession cierra la brecha de la 020 (RF-16,
// CA-10): reject y cancel de reservas, sin sesión, con prueba propia.
func TestKnownGapRejectAndCancelWithoutSession(t *testing.T) {
	h, _ := securityRouter(t)
	for _, p := range []string{
		"/api/v1/bookings/6731a2b3c4d5e6f7a8b9c0d1/reject",
		"/api/v1/bookings/6731a2b3c4d5e6f7a8b9c0d1/cancel",
	} {
		t.Run(p, func(t *testing.T) {
			assertError(t, do(h, http.MethodPost, p, ""), http.StatusUnauthorized, "unauthorized")
		})
	}
}
