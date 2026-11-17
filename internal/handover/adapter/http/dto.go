package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// --- peticiones ---

type photoRefJSON struct {
	PublicID *string `json:"publicId"`
}

type evidenceRequest struct {
	ConditionNotes *string        `json:"conditionNotes"`
	Photos         []photoRefJSON `json:"photos"`
}

// input valida la forma del cuerpo; los límites (3–500, 1–5 fotos) los aplica
// el dominio.
func (r evidenceRequest) input() (string, []images.Ref, error) {
	var d []shared.Detail
	if r.ConditionNotes == nil {
		d = append(d, shared.Detail{Field: "conditionNotes", Issue: "es obligatorio"})
	}
	if r.Photos == nil {
		d = append(d, shared.Detail{Field: "photos", Issue: "es obligatorio"})
	}
	refs := make([]images.Ref, 0, len(r.Photos))
	for i, p := range r.Photos {
		if p.PublicID == nil || *p.PublicID == "" {
			d = append(d, shared.Detail{Field: "photos[" + strconv.Itoa(i) + "].publicId", Issue: "es obligatorio"})
			continue
		}
		refs = append(refs, images.Ref{PublicID: *p.PublicID})
	}
	if len(d) > 0 {
		return "", nil, shared.NewValidation(bookings.MessageInvalidData, d...)
	}
	return *r.ConditionNotes, refs, nil
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
