// Package mongo implementa la búsqueda de objetos con agregaciones de MongoDB
// (spec 006, plan §3–§4). Lee items, users y bookings solo en modo lectura y
// crea sus propios índices sin tocar el repositorio de items (Art. XII).
package mongo

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// Colecciones que lee la búsqueda.
const (
	ItemsCollection    = "items"
	UsersCollection    = "users"
	BookingsCollection = "bookings"
)

// maxTextWords acota las palabras del filtro por texto en modo cercanía.
const maxTextWords = 8

// BuildPipeline devuelve la agregación de la consulta (plan P-05: función
// pura, probada sin base de datos). Pide limit+1 documentos para saber si hay
// otra página. now decide qué cuentas siguen suspendidas.
func BuildPipeline(q domain.Query, now time.Time) []bson.D {
	var p []bson.D
	switch q.Mode() {
	case domain.ModeNear:
		p = nearStages(q)
	case domain.ModeText:
		p = textStages(q)
	default:
		p = recentStages(q)
	}
	if q.Period != nil {
		p = append(p, availabilityStages(*q.Period)...)
	}
	p = append(p, ownerStages(now)...)
	p = append(p,
		bson.D{{Key: "$limit", Value: int64(q.Limit) + 1}},
		projectStage(q.Mode()),
	)
	return p
}

// baseFilter son los filtros comunes (RF-02): activos, de otras personas y
// los filtros opcionales.
func baseFilter(q domain.Query) bson.D {
	f := bson.D{
		{Key: "status", Value: "active"},
		{Key: "ownerId", Value: bson.D{{Key: "$ne", Value: q.ViewerID}}},
	}
	if q.Category != "" {
		f = append(f, bson.E{Key: "category", Value: q.Category})
	}
	if q.Modality != "" {
		f = append(f, bson.E{Key: "modality", Value: q.Modality})
	}
	if q.Municipality != "" {
		// Igualdad sin importar mayúsculas: regex anclada y escapada.
		f = append(f, bson.E{Key: "zone.municipality", Value: bson.Regex{
			Pattern: "^" + regexp.QuoteMeta(q.Municipality) + "$", Options: "i",
		}})
	}
	return f
}

// recentStages: por fecha de publicación descendente; cursor (createdAt, _id).
func recentStages(q domain.Query) []bson.D {
	f := baseFilter(q)
	if c := q.After; c != nil {
		at := time.UnixMilli(int64(c.Key)).UTC()
		oid := mustOID(c.ID)
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "createdAt", Value: bson.D{{Key: "$lt", Value: at}}}},
			bson.D{{Key: "createdAt", Value: at}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}}},
		}})
	}
	return []bson.D{
		{{Key: "$match", Value: f}},
		{{Key: "$sort", Value: bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}}},
	}
}

// textStages: $text en español (primera etapa obligatoria) y orden por
// relevancia; cursor (score, _id).
func textStages(q domain.Query) []bson.D {
	f := append(bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: q.Text}}}}, baseFilter(q)...)
	stages := []bson.D{
		{{Key: "$match", Value: f}},
		{{Key: "$addFields", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}}}}},
	}
	if c := q.After; c != nil {
		stages = append(stages, bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "score", Value: bson.D{{Key: "$lt", Value: c.Key}}}},
			bson.D{{Key: "score", Value: c.Key}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: mustOID(c.ID)}}}},
		}}}}})
	}
	return append(stages, bson.D{{Key: "$sort", Value: bson.D{{Key: "score", Value: -1}, {Key: "_id", Value: -1}}}})
}

// nearStages: $geoNear (primera etapa) con los filtros en query, dentro del
// radio, por distancia ascendente; cursor (dist, _id). Con texto se filtra por
// palabras (regex escapada, sin importar acentos ni mayúsculas), porque $text
// y $geoNear no se combinan (plan §3).
func nearStages(q domain.Query) []bson.D {
	f := baseFilter(q)
	if q.Text != "" {
		f = append(f, bson.E{Key: "$and", Value: wordFilters(q.Text)})
	}
	stages := []bson.D{{{Key: "$geoNear", Value: bson.D{
		{Key: "near", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{q.Near.Lng, q.Near.Lat}}}},
		{Key: "key", Value: "location"},
		{Key: "distanceField", Value: "dist"},
		{Key: "maxDistance", Value: float64(q.RadiusKm) * 1000},
		{Key: "spherical", Value: true},
		{Key: "query", Value: f},
	}}}}
	if c := q.After; c != nil {
		stages = append(stages, bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "dist", Value: bson.D{{Key: "$gt", Value: c.Key}}}},
			bson.D{{Key: "dist", Value: c.Key}, {Key: "_id", Value: bson.D{{Key: "$gt", Value: mustOID(c.ID)}}}},
		}}}}})
	}
	return append(stages, bson.D{{Key: "$sort", Value: bson.D{{Key: "dist", Value: 1}, {Key: "_id", Value: 1}}}})
}

// wordFilters exige cada palabra (hasta maxTextWords) en el título o la
// descripción.
func wordFilters(text string) bson.A {
	words := strings.Fields(text)
	if len(words) > maxTextWords {
		words = words[:maxTextWords]
	}
	out := bson.A{}
	for _, w := range words {
		rx := bson.Regex{Pattern: AccentInsensitive(w), Options: "i"}
		out = append(out, bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "title", Value: rx}},
			bson.D{{Key: "description", Value: rx}},
		}}})
	}
	return out
}

// accentClass devuelve la clase de una vocal (o la ñ) con sus variantes
// acentuadas y mayúsculas.
func accentClass(r rune) (string, bool) {
	c, ok := map[rune]string{
		'a': "[aáàäâAÁÀÄÂ]", 'e': "[eéèëêEÉÈËÊ]", 'i': "[iíìïîIÍÌÏÎ]",
		'o': "[oóòöôOÓÒÖÔ]", 'u': "[uúùüûUÚÙÜÛ]", 'n': "[nñNÑ]",
	}[r]
	return c, ok
}

// foldAccent quita el acento de una vocal (o la tilde de la ñ) en minúsculas.
func foldAccent(r rune) rune {
	if f, ok := (map[rune]rune{
		'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
		'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i', 'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o',
		'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u', 'ñ': 'n',
	})[r]; ok {
		return f
	}
	return r
}

// AccentInsensitive convierte una palabra en una expresión regular escapada
// que coincide sin importar acentos («electrico» encuentra «eléctrico»).
func AccentInsensitive(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(word) {
		r = foldAccent(r)
		if class, ok := accentClass(r); ok {
			b.WriteString(class)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
}

// blockingBookingStatuses son los estados de reserva que bloquean la
// disponibilidad (RF-03). Una solicitud pendiente no bloquea.
func blockingBookingStatuses() bson.A { return bson.A{"accepted", "in_progress"} }

// availabilityStages excluyen objetos con una reserva aceptada o en curso que
// se cruce con el periodo (RF-03). Fechas YYYY-MM-DD: el orden de cadenas es
// el orden de fechas. Si bookings no existe aún, el $lookup devuelve vacío.
func availabilityStages(p domain.Period) []bson.D {
	return []bson.D{
		{{Key: "$addFields", Value: bson.D{{Key: "_itemId", Value: bson.D{{Key: "$toString", Value: "$_id"}}}}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: BookingsCollection},
			{Key: "localField", Value: "_itemId"},
			{Key: "foreignField", Value: "itemId"},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "status", Value: bson.D{{Key: "$in", Value: blockingBookingStatuses()}}},
					{Key: "startDate", Value: bson.D{{Key: "$lte", Value: p.To}}},
					{Key: "endDate", Value: bson.D{{Key: "$gte", Value: p.From}}},
				}}},
				bson.D{{Key: "$limit", Value: 1}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}},
			{Key: "as", Value: "_busy"},
		}}},
		{{Key: "$match", Value: bson.D{{Key: "_busy", Value: bson.D{{Key: "$size", Value: 0}}}}}},
	}
}

// ownerStages unen el perfil público del dueño y descartan cuentas dadas de
// baja o suspendidas (RF-02). Un ownerId mal formado no rompe la consulta.
func ownerStages(now time.Time) []bson.D {
	return []bson.D{
		{{Key: "$addFields", Value: bson.D{{Key: "_ownerOid", Value: bson.D{{Key: "$convert", Value: bson.D{
			{Key: "input", Value: "$ownerId"}, {Key: "to", Value: "objectId"},
			{Key: "onError", Value: nil}, {Key: "onNull", Value: nil},
		}}}}}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: UsersCollection},
			{Key: "localField", Value: "_ownerOid"},
			{Key: "foreignField", Value: "_id"},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{
				{Key: "displayName", Value: 1}, {Key: "status", Value: 1}, {Key: "suspendedUntil", Value: 1},
				{Key: "ratingSum", Value: 1}, {Key: "ratingCount", Value: 1},
			}}}}},
			{Key: "as", Value: "_owner"},
		}}},
		{{Key: "$unwind", Value: "$_owner"}},
		{{Key: "$match", Value: bson.D{
			{Key: "_owner.status", Value: "active"},
			{Key: "_owner.suspendedUntil", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$gt", Value: now.UTC()}}}}},
		}}},
	}
}

// projectStage deja solo los datos de la tarjeta (RF-06) y la clave de orden.
func projectStage(m domain.Mode) bson.D {
	var sortKey any
	switch m {
	case domain.ModeNear:
		sortKey = "$dist"
	case domain.ModeText:
		sortKey = "$score"
	default:
		sortKey = bson.D{{Key: "$toLong", Value: "$createdAt"}}
	}
	proj := bson.D{
		{Key: "title", Value: 1}, {Key: "category", Value: 1}, {Key: "modality", Value: 1},
		{Key: "pricePerDayCents", Value: 1}, {Key: "zone", Value: 1}, {Key: "location", Value: 1},
		{Key: "createdAt", Value: 1},
		{Key: "firstPhoto", Value: bson.D{{Key: "$first", Value: "$photos"}}},
		{Key: "photoCount", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$photos", bson.A{}}}}}}},
		{Key: "owner", Value: bson.D{
			{Key: "id", Value: "$ownerId"}, {Key: "displayName", Value: "$_owner.displayName"},
			{Key: "ratingSum", Value: "$_owner.ratingSum"}, {Key: "ratingCount", Value: "$_owner.ratingCount"},
		}},
		{Key: "sortKey", Value: sortKey},
	}
	if m == domain.ModeNear {
		proj = append(proj, bson.E{Key: "dist", Value: 1})
	}
	return bson.D{{Key: "$project", Value: proj}}
}

// mustOID convierte un id ya validado por el dominio (IsObjectIDHex).
func mustOID(hex string) bson.ObjectID {
	oid, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return bson.NilObjectID
	}
	return oid
}
