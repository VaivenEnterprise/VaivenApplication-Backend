// Package mongo implementa el repositorio de entrega y devolución con MongoDB
// (spec 009, T003; plan P-02 y P-09). Guarda dentro de los documentos de
// bookings: solo hace $set de sus campos con filtro por estado, nunca
// reemplaza el documento completo, y sube version para que un guardado
// concurrente de la spec 007 detecte el cambio en vez de pisarlo.
package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
)

// StageRepo guarda las etapas dentro de la colección bookings.
type StageRepo struct {
	bookings *drv.Collection
}

var _ usecase.Repository = (*StageRepo)(nil)

// NewStageRepo crea el repositorio sobre db.
func NewStageRepo(db *drv.Database) *StageRepo {
	return &StageRepo{bookings: db.Collection(bookingsmongo.BookingsCollection)}
}

func objectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, bookings.ErrNotFound
	}
	return oid, nil
}

// apply ejecuta un UpdateOne y traduce «no coincidió» a usecase.ErrNoMatch.
func (r *StageRepo) apply(ctx context.Context, what string, filter, update bson.D) error {
	res, err := r.bookings.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("handover: %s: %w", what, err)
	}
	if res.MatchedCount == 0 {
		return usecase.ErrNoMatch
	}
	return nil
}

// SaveEvidence reemplaza la etapa completa (evidencia nueva, sin QR) si la
// reserva sigue en el estado de la etapa y la etapa no está confirmada.
func (r *StageRepo) SaveEvidence(ctx context.Context, bookingID string, k domain.Kind, h bookings.Handover) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	field := string(k)
	at := *h.RecordedAt
	return r.apply(ctx, "guardar evidencia",
		bson.D{
			{Key: "_id", Value: oid},
			{Key: "status", Value: string(k.RequiredStatus())},
			{Key: field + ".confirmedAt", Value: nil},
		},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: field, Value: bookingsmongo.EncodeHandover(h)},
				{Key: "updatedAt", Value: at.UTC()},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
}

// SaveQR guarda el QR si la evidencia sigue en la versión del QR y la etapa no
// está confirmada. Reemplaza al QR anterior.
func (r *StageRepo) SaveQR(ctx context.Context, bookingID string, k domain.Kind, qr bookings.QR) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	field := string(k)
	return r.apply(ctx, "guardar QR",
		bson.D{
			{Key: "_id", Value: oid},
			{Key: "status", Value: string(k.RequiredStatus())},
			{Key: field + ".confirmedAt", Value: nil},
			{Key: field + ".evidenceVersion", Value: qr.EvidenceVersion},
		},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: field + ".qr", Value: bookingsmongo.EncodeQR(qr)}}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
}

// Confirm consume el QR y confirma la etapa y el cambio de estado en una sola
// operación atómica (plan P-02, RNF-02): el filtro exige el estado de la etapa,
// la etapa sin confirmar, el hash del token, el QR sin usar, vigente y de la
// versión de evidencia actual. De dos confirmaciones simultáneas con el mismo
// QR, MongoDB aplica una y la otra no coincide.
func (r *StageRepo) Confirm(ctx context.Context, bookingID string, c domain.Confirmation, tokenHash string) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	field := string(c.Kind)
	at := c.At.UTC()
	filter := bson.D{
		{Key: "_id", Value: oid},
		{Key: "status", Value: string(c.Kind.RequiredStatus())},
		{Key: field + ".confirmedAt", Value: nil},
		{Key: field + ".qr.tokenHash", Value: tokenHash},
		{Key: field + ".qr.usedAt", Value: nil},
		{Key: field + ".qr.expiresAt", Value: bson.D{{Key: "$gt", Value: at}}},
		{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$" + field + ".qr.evidenceVersion", "$" + field + ".evidenceVersion"}}}},
	}
	set := bson.D{
		{Key: field + ".qr.usedAt", Value: at},
		{Key: field + ".confirmedAt", Value: at},
		{Key: field + ".confirmedById", Value: c.ConfirmedBy},
		{Key: "status", Value: string(c.NewStatus)},
		{Key: "updatedAt", Value: at},
	}
	if c.LateAt != nil {
		set = append(set, bson.E{Key: "lateAt", Value: c.LateAt.UTC()})
	}
	if c.Adjustment != nil {
		set = append(set, bson.E{Key: "adjustments", Value: bson.A{bookingsmongo.EncodeAdjustment(*c.Adjustment)}})
	}
	entry := bson.D{
		{Key: "status", Value: string(c.History.Status)},
		{Key: "at", Value: c.History.At.UTC()},
		{Key: "by", Value: string(c.History.By)},
	}
	return r.apply(ctx, "confirmar etapa", filter, bson.D{
		{Key: "$set", Value: set},
		{Key: "$push", Value: bson.D{{Key: "statusHistory", Value: bson.D{
			{Key: "$each", Value: bson.A{entry}},
			{Key: "$slice", Value: -bookings.MaxHistory},
		}}}},
		{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
	})
}
