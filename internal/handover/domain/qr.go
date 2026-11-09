package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/url"
	"strings"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// QRTTL es la vigencia del QR (POL-06).
const QRTTL = time.Duration(policy.POL06QRValidityMinutes) * time.Minute

// tokenBytes son los bytes aleatorios del token (plan P-01).
const tokenBytes = 32

// QRState es el estado del QR que se muestra en la etapa; lo define el modelo
// de datos de la reserva y aquí se reexporta.
type QRState = bookings.QRState

// Estados del QR.
const (
	QRNone    = bookings.QRNone
	QRActive  = bookings.QRActive
	QRExpired = bookings.QRExpired
	QRUsed    = bookings.QRUsed
)

// NewToken genera un token de 32 bytes aleatorios en base64url sin relleno.
// r nil usa crypto/rand. El token solo se devuelve a quien lo genera; nunca se
// guarda ni se registra en logs (RNF-01).
func NewToken(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken devuelve el SHA-256 del token en hexadecimal: lo único que se guarda.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewQR arma el QR de la etapa para el token, ligado a la versión de evidencia
// vigente (RF-02). Generar otro reemplaza al anterior.
func NewQR(h *bookings.Handover, token string, at time.Time) bookings.QR {
	return bookings.QR{
		TokenHash:       HashToken(token),
		ExpiresAt:       at.Add(QRTTL).UTC(),
		EvidenceVersion: h.EvidenceVersion,
	}
}

// Payload es el texto que codifica el QR: vaiven://qr?b=<reserva>&s=<etapa>&t=<token>.
func Payload(bookingID string, k Kind, token string) string {
	q := url.Values{}
	q.Set("b", bookingID)
	q.Set("s", string(k))
	q.Set("t", token)
	return "vaiven://qr?" + q.Encode()
}

// ParsePayload lee un qrPayload y devuelve reserva, etapa y token; ok es false
// si no es un QR de Vaiven bien formado.
func ParsePayload(s string) (bookingID string, k Kind, token string, ok bool) {
	rest, found := strings.CutPrefix(s, "vaiven://qr?")
	if !found {
		return "", "", "", false
	}
	q, err := url.ParseQuery(rest)
	if err != nil {
		return "", "", "", false
	}
	k = Kind(q.Get("s"))
	bookingID, token = q.Get("b"), q.Get("t")
	if bookingID == "" || token == "" || !k.Valid() {
		return "", "", "", false
	}
	return bookingID, k, token, true
}

// StateOf devuelve el estado del QR de la etapa en el instante at.
func StateOf(h *bookings.Handover, at time.Time) QRState { return h.QRStateAt(at) }

// VerifyQR comprueba que token es el QR vigente de la etapa. El hash se
// compara en tiempo constante (plan P-01). El orden de los errores no
// distingue un token ajeno de uno inexistente: ambos son ErrQRInvalid.
func VerifyQR(h *bookings.Handover, token string, at time.Time) error {
	if h == nil || h.QR == nil {
		return ErrQRInvalid
	}
	got, want := HashToken(token), h.QR.TokenHash
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return ErrQRInvalid
	}
	switch StateOf(h, at) {
	case QRUsed:
		return ErrQRUsed
	case QRExpired:
		return ErrQRExpired
	case QRActive:
		return nil
	}
	return ErrQRInvalid
}
