package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// --- peticiones ---

type createRequest struct {
	ItemID             *string `json:"itemId"`
	StartDate          *string `json:"startDate"`
	EndDate            *string `json:"endDate"`
	PaymentMode        *string `json:"paymentMode"`
	AcceptedTotalCents *int64  `json:"acceptedTotalCents"`
}

func (r createRequest) input() (usecase.CreateInput, error) {
	var d []shared.Detail
	req := func(field string, ok bool) {
		if !ok {
			d = append(d, shared.Detail{Field: field, Issue: "es obligatorio"})
		}
	}
	req("itemId", r.ItemID != nil && *r.ItemID != "")
	req("startDate", r.StartDate != nil)
	req("endDate", r.EndDate != nil)
	req("paymentMode", r.PaymentMode != nil)
	req("acceptedTotalCents", r.AcceptedTotalCents != nil)
	if r.AcceptedTotalCents != nil && *r.AcceptedTotalCents < 0 {
		d = append(d, shared.Detail{Field: "acceptedTotalCents", Issue: "no puede ser negativo"})
	}
	if len(d) > 0 {
		return usecase.CreateInput{}, shared.NewValidation(domain.MessageInvalidData, d...)
	}
	return usecase.CreateInput{
		QuoteInput: usecase.QuoteInput{
			ItemID: *r.ItemID, Start: *r.StartDate, End: *r.EndDate, PaymentMode: domain.PaymentMode(*r.PaymentMode),
		},
		AcceptedTotalCents: *r.AcceptedTotalCents,
	}, nil
}

type meetingPointJSON struct {
	Description *string `json:"description"`
}

type acceptRequest struct {
	MeetingPoint *meetingPointJSON `json:"meetingPoint"`
}

type reasonRequest struct {
	Reason *string `json:"reason"`
}

// decode lee un único objeto JSON. Si optional y el cuerpo está vacío, deja
// dst sin cambios.
func decode(r *http.Request, dst any, optional bool) error {
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(dst)
	switch {
	case optional && errors.Is(err, io.EOF):
		return nil
	case err != nil:
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return shared.NewValidation("El cuerpo de la petición es demasiado grande.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"})
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	case dec.More():
		return shared.NewValidation("El cuerpo de la petición debe ser un solo objeto JSON.")
	}
	return nil
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func parseListQuery(r *http.Request) (usecase.ListQuery, error) {
	q := r.URL.Query()
	lq := usecase.ListQuery{Role: domain.Role(q.Get("role"))}
	if s := q.Get("status"); s != "" {
		st := domain.Status(s)
		lq.Status = &st
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > usecase.MaxPageSize {
			return lq, invalid("limit", "debe ser un entero entre 1 y 50")
		}
		lq.Limit = n
	}
	if s := q.Get("cursor"); s != "" {
		c, err := decodeCursor(s)
		if err != nil {
			return lq, invalid("cursor", "no es válido")
		}
		lq.After = &c
	}
	return lq, nil
}

// El cursor es opaco: base64url("<createdAt en ms>.<id>").
func encodeCursor(c *domain.Cursor) *string {
	if c == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.CreatedAt.UnixMilli(), 10) + "." + c.ID))
	return &s
}

func decodeCursor(s string) (domain.Cursor, error) {
	if len(s) > 128 {
		return domain.Cursor{}, errors.New("cursor demasiado largo")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return domain.Cursor{}, err
	}
	ms, id, ok := strings.Cut(string(b), ".")
	n, errN := strconv.ParseInt(ms, 10, 64)
	if !ok || errN != nil || !isObjectIDHex(id) {
		return domain.Cursor{}, errors.New("cursor inválido")
	}
	return domain.Cursor{CreatedAt: time.UnixMilli(n).UTC(), ID: id}, nil
}

func isObjectIDHex(s string) bool {
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

// --- respuestas ---

type paymentResponse struct {
	Stage       string `json:"stage"`
	AmountCents int64  `json:"amountCents"`
}

type policyResponse struct {
	RequestExpiresInHours  int    `json:"requestExpiresInHours"`
	LateCancellationNotice string `json:"lateCancellationNotice"`
	NoShowNotice           string `json:"noShowNotice"`
}

// quoteResponse es Quote (api/openapi.yaml).
type quoteResponse struct {
	ItemID           string            `json:"itemId"`
	StartDate        string            `json:"startDate"`
	EndDate          string            `json:"endDate"`
	Days             int               `json:"days"`
	Modality         string            `json:"modality"`
	PricePerDayCents int64             `json:"pricePerDayCents"`
	TotalCents       int64             `json:"totalCents"`
	Currency         string            `json:"currency"`
	PaymentMode      string            `json:"paymentMode"`
	Payments         []paymentResponse `json:"payments"`
	Policy           policyResponse    `json:"policy"`
}

func toQuote(q domain.Quote) quoteResponse {
	out := quoteResponse{
		ItemID: q.ItemID, StartDate: q.Start.String(), EndDate: q.End.String(), Days: q.Days,
		Modality: string(q.Modality), PricePerDayCents: q.PricePerDayCents, TotalCents: q.TotalCents,
		Currency: domain.Currency, PaymentMode: string(q.PaymentMode), Payments: make([]paymentResponse, 0, len(q.Payments)),
		Policy: policyResponse{
			RequestExpiresInHours:  policy.POL03RequestExpiryHours,
			LateCancellationNotice: policy.POL09LateCancellationNotice,
			NoShowNotice:           policy.POL09NoShowNotice,
		},
	}
	for _, p := range q.Payments {
		out.Payments = append(out.Payments, paymentResponse{Stage: string(p.Stage), AmountCents: p.AmountCents})
	}
	return out
}

type publicUserResponse struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	MemberSince   time.Time `json:"memberSince"`
}

func toPublic(p users.PublicUser) publicUserResponse {
	return publicUserResponse{ID: p.ID, DisplayName: p.DisplayName, RatingAverage: p.RatingAverage, RatingCount: p.RatingCount, MemberSince: p.MemberSince.UTC()}
}

type bookingItemResponse struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	ThumbURL *string `json:"thumbUrl"`
	Modality string  `json:"modality"`
}

type priceResponse struct {
	Modality         string `json:"modality"`
	PricePerDayCents int64  `json:"pricePerDayCents"`
	TotalCents       int64  `json:"totalCents"`
	Currency         string `json:"currency"`
}

type bookingPaymentResponse struct {
	Stage       string     `json:"stage"`
	AmountCents int64      `json:"amountCents"`
	Status      string     `json:"status"`
	ReportedAt  *time.Time `json:"reportedAt"`
	// Note es la nota opcional del reporte de pago (spec 009).
	Note        *string    `json:"note"`
	ConfirmedAt *time.Time `json:"confirmedAt"`
}

// imageResponse, qrStateResponse y stageResponse son Image y Stage
// (api/openapi.yaml): la etapa de entrega o devolución de la spec 009. Nunca
// llevan el hash del token (RNF-01).
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

// adjustmentResponse es Adjustment: un ajuste registrado, nunca cobrado.
type adjustmentResponse struct {
	Type        string `json:"type"`
	LateDays    int    `json:"lateDays"`
	AmountCents int64  `json:"amountCents"`
	Status      string `json:"status"`
}

func toStage(v *usecase.HandoverView) *stageResponse {
	if v == nil {
		return nil
	}
	s := v.Stage
	out := &stageResponse{
		RecordedAt: utcPtr(s.RecordedAt), RecordedByID: s.RecordedByID, ConditionNotes: s.ConditionNotes,
		Photos: make([]imageResponse, 0, len(s.Photos)), EvidenceVersion: s.EvidenceVersion,
		QR:          qrStateResponse{State: string(v.QR), ExpiresAt: utcPtr(v.QRExpiresAt)},
		ConfirmedAt: utcPtr(s.ConfirmedAt), ConfirmedByID: s.ConfirmedByID,
	}
	for i, p := range s.Photos {
		img := imageResponse{PublicID: p.PublicID, Width: p.Width, Height: p.Height, Format: p.Format, Bytes: p.Bytes, Delivery: string(p.Delivery)}
		if i < len(v.URLs) {
			img.URL, img.ThumbURL = v.URLs[i].URL, v.URLs[i].ThumbURL
		}
		out.Photos = append(out.Photos, img)
	}
	return out
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

type meetingPointResponse struct {
	Description string `json:"description"`
}

// bookingResponse es Booking (api/openapi.yaml). delivery, return, isLate,
// adjustments y affectedBy los completa la spec 009.
type bookingResponse struct {
	ID             string                   `json:"id"`
	Item           bookingItemResponse      `json:"item"`
	Owner          publicUserResponse       `json:"owner"`
	Requester      publicUserResponse       `json:"requester"`
	StartDate      string                   `json:"startDate"`
	EndDate        string                   `json:"endDate"`
	Days           int                      `json:"days"`
	UsageRules     *string                  `json:"usageRules"`
	PriceSnapshot  priceResponse            `json:"priceSnapshot"`
	PaymentMode    string                   `json:"paymentMode"`
	Payments       []bookingPaymentResponse `json:"payments"`
	Status         string                   `json:"status"`
	ExpiresAt      *time.Time               `json:"expiresAt"`
	MeetingPoint   *meetingPointResponse    `json:"meetingPoint"`
	CancelReason   *string                  `json:"cancelReason"`
	CancelledBy    *string                  `json:"cancelledBy"`
	RejectReason   *string                  `json:"rejectReason"`
	ConversationID *string                  `json:"conversationId"`
	Delivery       *stageResponse           `json:"delivery"`
	Return         *stageResponse           `json:"return"`
	IsLate         bool                     `json:"isLate"`
	Adjustments    []adjustmentResponse     `json:"adjustments"`
	AffectedBy     *string                  `json:"affectedBy"`
	ViewerRole     string                   `json:"viewerRole"`
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
	Version        int64                    `json:"version"`
}

func toBooking(v usecase.BookingView) bookingResponse {
	b := v.Booking
	out := bookingResponse{
		ID:        b.ID,
		Item:      bookingItemResponse{ID: b.ItemID, Title: b.Item.Title, ThumbURL: v.ThumbURL, Modality: string(b.Item.Modality)},
		Owner:     toPublic(v.Owner),
		Requester: toPublic(v.Requester),
		StartDate: b.Start.String(), EndDate: b.End.String(), Days: b.Days,
		UsageRules: b.Item.UsageRules,
		PriceSnapshot: priceResponse{
			Modality: string(b.Price.Modality), PricePerDayCents: b.Price.PricePerDayCents,
			TotalCents: b.Price.TotalCents, Currency: b.Price.Currency,
		},
		PaymentMode:    string(b.PaymentMode),
		Payments:       make([]bookingPaymentResponse, 0, len(b.Payments)),
		Status:         string(b.Status),
		ExpiresAt:      b.ExpiresAt,
		CancelReason:   b.CancelReason,
		RejectReason:   b.RejectReason,
		ConversationID: b.ConversationID,
		Delivery:       toStage(v.Delivery),
		Return:         toStage(v.Return),
		IsLate:         b.IsLate,
		Adjustments:    make([]adjustmentResponse, 0, len(b.Adjustments)),
		AffectedBy:     b.AffectedBy,
		ViewerRole:     string(v.ViewerRole),
		CreatedAt:      b.CreatedAt.UTC(),
		UpdatedAt:      b.UpdatedAt.UTC(),
		Version:        b.Version,
	}
	if b.CancelledBy != nil {
		s := string(*b.CancelledBy)
		out.CancelledBy = &s
	}
	if v.ShowMeetingPoint && b.MeetingPoint != nil {
		out.MeetingPoint = &meetingPointResponse{Description: *b.MeetingPoint}
	}
	for _, p := range b.Payments {
		out.Payments = append(out.Payments, bookingPaymentResponse{
			Stage: string(p.Stage), AmountCents: p.AmountCents, Status: string(p.Status),
			ReportedAt: p.ReportedAt, Note: p.ReportedNote, ConfirmedAt: p.ConfirmedAt,
		})
	}
	for _, a := range b.Adjustments {
		out.Adjustments = append(out.Adjustments, adjustmentResponse{Type: a.Type, LateDays: a.LateDays, AmountCents: a.AmountCents, Status: a.Status})
	}
	return out
}

type pageResponse struct {
	Items      []bookingResponse `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}
