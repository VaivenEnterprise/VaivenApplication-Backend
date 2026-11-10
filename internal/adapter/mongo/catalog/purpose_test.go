package catalog

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

// purposeKind es el propósito funcional de un índice (CA-10a).
type purposeKind string

const (
	forQuery  purposeKind = "consulta y orden"
	forUnique purposeKind = "unicidad"
	forTTL    purposeKind = "expiración TTL"
	forText   purposeKind = "búsqueda de texto"
	forGeo    purposeKind = "consulta geoespacial"
)

// purpose documenta por qué existe cada uno de los 34 índices. Los de
// consulta tienen además una consulta representativa en
// catalog_integration_test.go, cuyo explain debe elegir el índice.
var purpose = map[string]struct {
	kind purposeKind
	why  string
}{
	"refresh_tokens.tokenHash_unique": {forUnique, "un refresh token (su hash) no se repite; búsqueda al canjear"},
	"refresh_tokens.familyId":         {forQuery, "revocar la familia al detectar reutilización (CA-03 de la 003)"},
	"refresh_tokens.userId":           {forQuery, "revocar todas las sesiones de una persona (baja, rotación)"},
	"refresh_tokens.expiresAt_ttl":    {forTTL, "borrar refresh tokens vencidos"},
	"users.email_unique":              {forUnique, "una cuenta por correo; inicio de sesión por correo"},

	"bookings.item_status_dates":          {forQuery, "detectar choques de fechas de un objeto por estado"},
	"bookings.requester_status_createdAt": {forQuery, "GET /me/bookings como solicitante, por estado y recientes"},
	"bookings.owner_status_createdAt":     {forQuery, "GET /me/bookings como dueño, por estado y recientes"},
	"bookings.status_expiresAt":           {forQuery, "job que vence solicitudes sin respuesta"},
	"bookings.status_lateAt":              {forQuery, "job de retrasos de devolución (spec 009)"},

	"items.owner_status_createdAt":    {forQuery, "GET /me/items por estado, más recientes primero"},
	"items.location_2dsphere":         {forGeo, "búsqueda por cercanía"},
	"items.status_category_createdAt": {forQuery, "listado por estado y categoría, recientes primero"},
	"items.search_text":               {forText, "búsqueda de texto en título y descripción (RF-04 de la 006)"},
	"items.search_filters":            {forQuery, "búsqueda con filtros de categoría, modalidad y municipio"},
	"items.search_recent":             {forQuery, "modo reciente: solo por estado, orden createdAt y _id"},

	"conversations.bookingId_unique":          {forUnique, "una conversación por reserva"},
	"conversations.participant_lastMessageAt": {forQuery, "bandeja de una persona por último mensaje"},
	"messages.conversation_seq":               {forQuery, "mensajes de una conversación por secuencia (paginación)"},
	"messages.conversation_sender_clientId_unique": {forUnique,
		"idempotencia por clientId (CA-03 de la 008); parcial: solo si clientId es texto"},

	"ratings.booking_rater_unique": {forUnique, "una calificación por persona y reserva"},
	"ratings.ratee_id":             {forQuery, "calificaciones recibidas, más recientes primero"},

	"incidents.booking_status":     {forQuery, "incidencias abiertas de una reserva"},
	"incidents.reporter_createdAt": {forQuery, "GET /me/incidents como quien reporta"},
	"incidents.reported_createdAt": {forQuery, "incidencias contra una persona"},
	"incidents.status_createdAt":   {forQuery, "cola de administración por estado, más antiguas primero"},

	"sanctions.user_createdAt": {forQuery, "sanciones de una persona, más recientes primero"},

	"notifications.user_id":     {forQuery, "avisos de una persona, más recientes primero (cursor por _id)"},
	"notifications.user_read":   {forQuery, "contar y marcar los no leídos de una persona"},
	"notifications.created_ttl": {forTTL, "borrar avisos a los 90 días (RNF-01 de la 011)"},
	"push_tokens.token_unique":  {forUnique, "un token de push pertenece a un solo dispositivo"},
	"push_tokens.user":          {forQuery, "tokens de una persona para enviarle push"},

	"reports.reporter_target_unique": {forUnique, "una persona reporta el mismo contenido una sola vez"},
	"reports.status_created":         {forQuery, "cola de moderación por estado, más antiguos primero"},
}

// justifiedPrefixes registra los pares (a es prefijo de b en la misma
// colección) ya analizados y por qué se conservan. Hoy no hay ninguno.
var justifiedPrefixes = map[string]string{}

// Prueba 13, parte documental (CA-10a): cada índice del catálogo tiene un
// propósito, y no hay propósitos de índices que ya no existen.
func TestEveryIndexHasAPurpose(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range All() {
		name, _, err := adaptermongo.ExpectedDocument(s)
		if err != nil {
			t.Fatal(err)
		}
		id := s.Collection + "." + name
		seen[id] = true
		p, ok := purpose[id]
		if !ok || p.why == "" {
			t.Errorf("%s: sin propósito documentado", id)
		}
	}
	for id := range purpose {
		if !seen[id] {
			t.Errorf("%s: propósito de un índice que no está en el catálogo", id)
		}
	}
}

// Prueba 14 (CA-10b): análisis de redundancia. Un índice cuyas claves son
// prefijo de otro NO se declara redundante solo por eso: el par debe estar
// razonado en justifiedPrefixes (orden, unique, sparse, filtro parcial, TTL,
// collation y consultas reales). Hoy ningún par es prefijo.
func TestPrefixPairsAreReasoned(t *testing.T) {
	type idx struct {
		id   string
		keys bson.D
	}
	byCol := map[string][]idx{}
	for _, s := range All() {
		name, _, err := adaptermongo.ExpectedDocument(s)
		if err != nil {
			t.Fatal(err)
		}
		keys, _ := s.Model.Keys.(bson.D)
		byCol[s.Collection] = append(byCol[s.Collection], idx{id: s.Collection + "." + name, keys: keys})
	}
	for _, list := range byCol {
		for _, a := range list {
			for _, b := range list {
				if a.id == b.id || len(a.keys) > len(b.keys) || !isPrefix(a.keys, b.keys) {
					continue
				}
				pair := a.id + " ⊂ " + b.id
				if justifiedPrefixes[pair] == "" {
					t.Errorf("%s: prefijo sin análisis razonado en justifiedPrefixes", pair)
				}
			}
		}
	}
}

func isPrefix(a, b bson.D) bool {
	for i := range a {
		if a[i].Key != b[i].Key || !sameDirection(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func sameDirection(a, b any) bool {
	na, oka := toFloat(a)
	nb, okb := toFloat(b)
	if oka && okb {
		return na == nb
	}
	return a == b
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
