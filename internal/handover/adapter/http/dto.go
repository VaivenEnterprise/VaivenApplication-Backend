package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// --- peticiones ---

type photoRefJSON struct {
	PublicID *string `json:"publicId" validate:"required,nonempty"`
}

// evidenceRequest se valida en la frontera con validator/v10 (Art. VI.2; spec
// 016, T005); los límites (3–500, 1–5 fotos) los aplica el dominio.
type evidenceRequest struct {
	ConditionNotes *string        `json:"conditionNotes" validate:"required"`
	Photos         []photoRefJSON `json:"photos" validate:"required,dive"`
}

// input convierte una petición ya validada (Module.decode).
func (r evidenceRequest) input() (string, []images.Ref) {
	refs := make([]images.Ref, 0, len(r.Photos))
	for _, p := range r.Photos {
		refs = append(refs, images.Ref{PublicID: *p.PublicID})
	}
	return *r.ConditionNotes, refs
}

type confirmRequest struct {
	Token *string `json:"token"`
}

// decode lee un único objeto JSON.
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(dst)
	switch {
	case err != nil:
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return shared.NewValidation("El cuerpo de la petición es demasiado grande.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return shared.NewValidation(bookings.MessageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"})
		}
		if errors.Is(err, io.EOF) {
			return shared.NewValidation("El cuerpo de la petición está vacío.")
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	case dec.More():
		return shared.NewValidation("El cuerpo de la petición debe ser un solo objeto JSON.")
	}
	return nil
}

// --- respuestas ---

type imageResponse struct {
	PublicID string `json:"publicId"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Format   string `json:"format"`
	Bytes    int64  `json:"bytes"`
	Delivery string `json:"delivery"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl"`
}

type qrStateResponse struct {
	State     string     `json:"state"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// stageResponse es Stage (api/openapi.yaml).
type stageResponse struct {
	RecordedAt      *time.Time      `json:"recordedAt"`
	RecordedByID    *string         `json:"recordedById"`
	ConditionNotes  string          `json:"conditionNotes"`
	Photos          []imageResponse `json:"photos"`
	EvidenceVersion int             `json:"evidenceVersion"`
	QR              qrStateResponse `json:"qr"`
	ConfirmedAt     *time.Time      `json:"confirmedAt"`
	ConfirmedByID   *string         `json:"confirmedById"`
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func toStage(v usecase.StageView) stageResponse {
	s := v.Stage
	out := stageResponse{
		RecordedAt: utc(s.RecordedAt), RecordedByID: s.RecordedByID, ConditionNotes: s.ConditionNotes,
		Photos: make([]imageResponse, 0, len(s.Photos)), EvidenceVersion: s.EvidenceVersion,
		QR:          qrStateResponse{State: string(v.QR), ExpiresAt: utc(v.QRExpiresAt)},
		ConfirmedAt: utc(s.ConfirmedAt), ConfirmedByID: s.ConfirmedByID,
	}
	for i, p := range s.Photos {
		img := imageResponse{
			PublicID: p.PublicID, Width: p.Width, Height: p.Height, Format: p.Format, Bytes: p.Bytes, Delivery: string(p.Delivery),
		}
		if i < len(v.URLs) {
			img.URL, img.ThumbURL = v.URLs[i].URL, v.URLs[i].ThumbURL
		}
		out.Photos = append(out.Photos, img)
	}
	return out
}

type qrResponse struct {
	Token     string    `json:"token"`
	QRPayload string    `json:"qrPayload"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func toQR(q usecase.QRIssued) qrResponse {
	return qrResponse{Token: q.Token, QRPayload: q.Payload, ExpiresAt: q.ExpiresAt.UTC()}
}
