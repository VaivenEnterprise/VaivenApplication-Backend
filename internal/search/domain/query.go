// Package domain contiene las reglas de la búsqueda de objetos (spec 006,
// T002): validación de parámetros (RF-08, plan P-02), modos de orden, tarjetas
// y cursor. Solo biblioteca estándar (Art. III).
package domain

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Mode es el orden de los resultados (plan §3).
type Mode int

// Modos de búsqueda.
const (
	// ModeRecent: sin texto ni ubicación; por fecha de publicación descendente.
	ModeRecent Mode = iota
	// ModeText: con texto y sin ubicación; por relevancia.
	ModeText
	// ModeNear: con ubicación; por distancia ascendente.
	ModeNear
)

func (m Mode) String() string {
	switch m {
	case ModeText:
		return "text"
	case ModeNear:
		return "near"
	default:
		return "recent"
	}
}

func parseMode(s string) (Mode, bool) {
	switch s {
	case "recent":
		return ModeRecent, true
	case "text":
		return ModeText, true
	case "near":
		return ModeNear, true
	}
	return 0, false
}

// Point es una coordenada.
type Point struct {
	Lat float64
	Lng float64
}

// Period es un periodo de fechas de calendario YYYY-MM-DD (inclusivo) en
// America/Mexico_City (plan P-02).
type Period struct {
	From string
	To   string
}

// Límites de los parámetros (RF-01).
const (
	TextMin          = 2
	TextMax          = 80
	MunicipalityMin  = 2
	MunicipalityMax  = 60
	RadiusMinKm      = 1
	RadiusMaxKm      = 50
	DefaultRadiusKm  = 25 // plan P-04
	DefaultLimit     = 20
	MaxLimit         = 50
	dateLayout       = "2006-01-02"
	messageInvalid   = "Revisa los filtros de la búsqueda."
	issueInvalidOpt  = "no es una opción válida"
	issueIntegerLim  = "debe ser un entero entre 1 y 50"
	issueDateFormat  = "debe tener el formato AAAA-MM-DD"
	issuePairLatLng  = "lat y lng van juntos"
	issuePairFromTo  = "from y to van juntos"
	issueRadiusNoPos = "solo se usa con lat y lng"
)

// RawQuery son los parámetros tal como llegan en la URL.
type RawQuery struct {
	Q, Category, Modality, Municipality string
	From, To                            string
	Lat, Lng, RadiusKm                  string
	Limit, Cursor                       string
}

// Query es una búsqueda validada.
type Query struct {
	// ViewerID es quien busca: sus objetos no aparecen (RF-02).
	ViewerID     string
	Text         string
	Category     string
	Modality     string
	Municipality string
	Period       *Period
	Near         *Point
	RadiusKm     int
	Limit        int
	After        *Cursor
}

// Mode devuelve el modo de orden de la consulta.
func (q Query) Mode() Mode {
	switch {
	case q.Near != nil:
		return ModeNear
	case q.Text != "":
		return ModeText
	default:
		return ModeRecent
	}
}

// Parse valida los parámetros (RF-01, RF-08). today es la fecha de hoy en
// America/Mexico_City (YYYY-MM-DD). Devuelve validation_failed con un detalle
// por parámetro.
func Parse(raw RawQuery, viewerID, today string) (Query, error) {
	q := Query{ViewerID: viewerID, Limit: DefaultLimit}
	var d []shared.Detail
	add := func(f, issue string) { d = append(d, shared.Detail{Field: f, Issue: issue}) }

	if t := strings.TrimSpace(raw.Q); t != "" {
		if n := utf8.RuneCountInString(t); n < TextMin || n > TextMax {
			add("q", "debe tener entre 2 y 80 caracteres")
		}
		q.Text = t
	}
	if raw.Category != "" {
		if !policy.POL14Valid(raw.Category) {
			add("category", issueInvalidOpt)
		}
		q.Category = raw.Category
	}
	if raw.Modality != "" {
		if raw.Modality != "loan" && raw.Modality != "rent" {
			add("modality", issueInvalidOpt)
		}
		q.Modality = raw.Modality
	}
	if m := strings.TrimSpace(raw.Municipality); m != "" {
		if n := utf8.RuneCountInString(m); n < MunicipalityMin || n > MunicipalityMax {
			add("municipality", "debe tener entre 2 y 60 caracteres")
		}
		q.Municipality = m
	}
	q.Period = parsePeriod(raw.From, raw.To, today, add)
	q.Near, q.RadiusKm = parseNear(raw.Lat, raw.Lng, raw.RadiusKm, add)
	if raw.Limit != "" {
		n, err := strconv.Atoi(raw.Limit)
		if err != nil || n < 1 || n > MaxLimit {
			add("limit", issueIntegerLim)
		} else {
			q.Limit = n
		}
	}
	if raw.Cursor != "" {
		c, err := DecodeCursor(raw.Cursor)
		switch {
		case err != nil:
			add("cursor", "no es válido")
		case len(d) == 0 && c.Mode != q.Mode():
			add("cursor", "es de otra búsqueda; repite los mismos filtros")
		default:
			q.After = &c
		}
	}
	if len(d) > 0 {
		return Query{}, shared.NewValidation(messageInvalid, d...)
	}
	return q, nil
}

func parsePeriod(from, to, today string, add func(string, string)) *Period {
	if from == "" && to == "" {
		return nil
	}
	if from == "" || to == "" {
		missing := "from"
		if from != "" {
			missing = "to"
		}
		add(missing, issuePairFromTo)
		return nil
	}
	f, errF := time.Parse(dateLayout, from)
	t, errT := time.Parse(dateLayout, to)
	if errF != nil {
		add("from", issueDateFormat)
	}
	if errT != nil {
		add("to", issueDateFormat)
	}
	if errF != nil || errT != nil {
		return nil
	}
	ok := true
	if now, err := time.Parse(dateLayout, today); err == nil && f.Before(now) {
		add("from", "no puede ser anterior a hoy")
		ok = false
	}
	days := int(t.Sub(f).Hours()/24) + 1
	switch {
	case days < policy.POL02MinDays:
		add("to", "no puede ser anterior a from")
		ok = false
	case days > policy.POL02MaxDays:
		add("to", "el periodo dura como máximo 7 días")
		ok = false
	}
	if !ok {
		return nil
	}
	return &Period{From: f.Format(dateLayout), To: t.Format(dateLayout)}
}

func parseNear(lat, lng, radius string, add func(string, string)) (*Point, int) {
	if lat == "" && lng == "" {
		if radius != "" {
			add("radiusKm", issueRadiusNoPos)
		}
		return nil, 0
	}
	if lat == "" || lng == "" {
		missing := "lat"
		if lat != "" {
			missing = "lng"
		}
		add(missing, issuePairLatLng) // CA-06
		return nil, 0
	}
	la, errLa := strconv.ParseFloat(lat, 64)
	ln, errLn := strconv.ParseFloat(lng, 64)
	ok := true
	if errLa != nil || math.IsNaN(la) || la < -90 || la > 90 {
		add("lat", "debe ser un número entre -90 y 90")
		ok = false
	}
	if errLn != nil || math.IsNaN(ln) || ln < -180 || ln > 180 {
		add("lng", "debe ser un número entre -180 y 180")
		ok = false
	}
	r := DefaultRadiusKm
	if radius != "" {
		n, err := strconv.Atoi(radius)
		if err != nil || n < RadiusMinKm || n > RadiusMaxKm {
			add("radiusKm", issueIntegerLim)
			ok = false
		} else {
			r = n
		}
	}
	if !ok {
		return nil, 0
	}
	return &Point{Lat: la, Lng: ln}, r
}
