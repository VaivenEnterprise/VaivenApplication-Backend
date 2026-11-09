package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
)

// Cursor marca la posición de una página (plan P-01): el valor de orden del
// último resultado (Key) y su id, más el modo, para rechazar un cursor de otra
// búsqueda. Key es createdAt en ms (reciente), la relevancia (texto) o la
// distancia en metros (cercanía).
type Cursor struct {
	Mode Mode
	Key  float64
	ID   string
}

// MaxCursorLen es la longitud máxima del cursor codificado.
const MaxCursorLen = 256

type cursorJSON struct {
	M  string  `json:"m"`
	K  float64 `json:"k"`
	ID string  `json:"id"`
}

// ErrInvalidCursor indica un cursor que no se puede usar.
var ErrInvalidCursor = errors.New("search: cursor inválido")

// Encode devuelve el cursor opaco: JSON en base64url sin relleno. No lleva
// datos sensibles, así que no se firma (P-01).
func (c Cursor) Encode() string {
	b, _ := json.Marshal(cursorJSON{M: c.Mode.String(), K: c.Key, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor lee un cursor de Encode y rechaza cualquier otra cosa.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" || len(s) > MaxCursorLen {
		return Cursor{}, ErrInvalidCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var cj cursorJSON
	if err := json.Unmarshal(b, &cj); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	m, ok := parseMode(cj.M)
	if !ok || !IsObjectIDHex(cj.ID) || math.IsNaN(cj.K) || math.IsInf(cj.K, 0) || cj.K < 0 {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{Mode: m, Key: cj.K, ID: cj.ID}, nil
}

// IsObjectIDHex indica si s son 24 caracteres hexadecimales en minúsculas.
func IsObjectIDHex(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
