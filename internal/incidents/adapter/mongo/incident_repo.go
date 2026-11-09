// Package mongo implementa el repositorio de incidencias con MongoDB (spec 010,
// T006; plan §3). Es el único paquete del módulo que importa el driver. No lee
// ni escribe bookings ni users: eso lo hacen sus propios módulos.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/usecase"
)

// IncidentsCollection es la colección de incidencias.
const IncidentsCollection = "incidents"

// Repo guarda las incidencias. Cumple usecase.Repository.
type Repo struct{ col *drv.Collection }

// NewRepo crea el repositorio sobre db.
func NewRepo(db *drv.Database) *Repo { return &Repo{col: db.Collection(IncidentsCollection)} }

// EnsureIndexes crea los índices de incidents (plan §3).
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "bookingId", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetName("booking_status")},
		{Keys: bson.D{{Key: "reporterId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetName("reporter_createdAt")},
		{Keys: bson.D{{Key: "reportedId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetName("reported_createdAt")},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}, Options: options.Index().SetName("status_createdAt")},
	})
	if err != nil {
		return fmt.Errorf("incidents: crear índices: %w", err)
	}
	return nil
}

type imageDoc struct {
	PublicID string `bson:"publicId"`
	Width    int    `bson:"width"`
	Height   int    `bson:"height"`
	Format   string `bson:"format"`
	Bytes    int64  `bson:"bytes"`
	Delivery string `bson:"delivery"`
}

type noteDoc struct {
	Text             string    `bson:"text"`
	At               time.Time `bson:"at"`
	ByAdminID        string    `bson:"byAdminId"`
	VisibleToParties bool      `bson:"visibleToParties"`
}

type finalDoc struct {
	Text      string    `bson:"text"`
	Outcome   string    `bson:"outcome"`
	ByAdminID string    `bson:"byAdminId"`
	At        time.Time `bson:"at"`
}

// incidentDoc es el documento de incidents (plan §3). Las notas (≤ 10) y la
// evidencia (≤ 5) están incrustadas porque su cardinalidad está acotada.
type incidentDoc struct {
	ID          bson.ObjectID `bson:"_id"`
	BookingID   string        `bson:"bookingId"`
	ReporterID  string        `bson:"reporterId"`
	ReportedID  string        `bson:"reportedId"`
	Reason      string        `bson:"reason"`
	Description string        `bson:"description"`
	Evidence    []imageDoc    `bson:"evidence"`
	Status      string        `bson:"status"`
	Notes       []noteDoc     `bson:"notes"`
	Final       *finalDoc     `bson:"finalResponse"`
	CreatedAt   time.Time     `bson:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt"`
	ClosedAt    *time.Time    `bson:"closedAt"`
	Version     int64         `bson:"version"`
}

func toDoc(i domain.Incident) (incidentDoc, error) {
	oid, err := bson.ObjectIDFromHex(i.ID)
	if err != nil {
		return incidentDoc{}, fmt.Errorf("incidents: id inválido: %w", err)
	}
	d := incidentDoc{
		ID: oid, BookingID: i.BookingID, ReporterID: i.ReporterID, ReportedID: i.ReportedID,
		Reason: string(i.Reason), Description: i.Description, Status: string(i.Status),
		Evidence: make([]imageDoc, 0, len(i.Evidence)), Notes: make([]noteDoc, 0, len(i.Notes)),
		CreatedAt: i.CreatedAt.UTC(), UpdatedAt: i.UpdatedAt.UTC(), Version: i.Version,
	}
	for _, e := range i.Evidence {
		d.Evidence = append(d.Evidence, imageDoc{PublicID: e.PublicID, Width: e.Width, Height: e.Height, Format: e.Format, Bytes: e.Bytes, Delivery: string(e.Delivery)})
	}
	for _, n := range i.Notes {
		d.Notes = append(d.Notes, noteDoc{Text: n.Text, At: n.At.UTC(), ByAdminID: n.ByAdminID, VisibleToParties: n.VisibleToParties})
	}
	if i.Final != nil {
		d.Final = finalToDoc(*i.Final)
	}
	if i.ClosedAt != nil {
		c := i.ClosedAt.UTC()
		d.ClosedAt = &c
	}
	return d, nil
}

func finalToDoc(f domain.FinalResponse) *finalDoc {
	return &finalDoc{Text: f.Text, Outcome: string(f.Outcome), ByAdminID: f.ByAdminID, At: f.At.UTC()}
}

func (d incidentDoc) toDomain() domain.Incident {
	i := domain.Incident{
		ID: d.ID.Hex(), BookingID: d.BookingID, ReporterID: d.ReporterID, ReportedID: d.ReportedID,
		Reason: domain.Reason(d.Reason), Description: d.Description, Status: domain.Status(d.Status),
		Evidence: make([]images.Image, 0, len(d.Evidence)), Notes: make([]domain.Note, 0, len(d.Notes)),
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(), Version: d.Version,
	}
	for _, e := range d.Evidence {
		i.Evidence = append(i.Evidence, images.Image{PublicID: e.PublicID, Width: e.Width, Height: e.Height, Format: e.Format, Bytes: e.Bytes, Delivery: images.Delivery(e.Delivery)})
	}
	for _, n := range d.Notes {
		i.Notes = append(i.Notes, domain.Note{Text: n.Text, At: n.At.UTC(), ByAdminID: n.ByAdminID, VisibleToParties: n.VisibleToParties})
	}
	if d.Final != nil {
		i.Final = &domain.FinalResponse{Text: d.Final.Text, Outcome: domain.Outcome(d.Final.Outcome), ByAdminID: d.Final.ByAdminID, At: d.Final.At.UTC()}
	}
	if d.ClosedAt != nil {
		c := d.ClosedAt.UTC()
		i.ClosedAt = &c
	}
	return i
}

// NewID genera el identificador de una incidencia.
func (r *Repo) NewID() string { return bson.NewObjectID().Hex() }

// Create inserta la incidencia.
func (r *Repo) Create(ctx context.Context, i domain.Incident) (domain.Incident, error) {
	d, err := toDoc(i)
	if err != nil {
		return domain.Incident{}, err
	}
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		return domain.Incident{}, fmt.Errorf("incidents: guardar incidencia: %w", err)
	}
	return d.toDomain(), nil
}

// ByID busca una incidencia; si no existe (o el id está mal formado),
// domain.ErrNotFound.
func (r *Repo) ByID(ctx context.Context, id string) (domain.Incident, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Incident{}, domain.ErrNotFound
	}
	var d incidentDoc
	if err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.Incident{}, domain.ErrNotFound
		}
		return domain.Incident{}, fmt.Errorf("incidents: buscar incidencia: %w", err)
	}
	return d.toDomain(), nil
}

// Exists indica si existe la incidencia id.
func (r *Repo) Exists(ctx context.Context, id string) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, nil
	}
	n, err := r.col.CountDocuments(ctx, bson.D{{Key: "_id", Value: oid}}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("incidents: buscar incidencia: %w", err)
	}
	return n > 0, nil
}

// CountOpen cuenta las incidencias abiertas de la reserva.
func (r *Repo) CountOpen(ctx context.Context, bookingID string) (int, error) {
	n, err := r.col.CountDocuments(ctx, bson.D{{Key: "bookingId", Value: bookingID}, {Key: "status", Value: string(domain.StatusOpen)}})
	if err != nil {
		return 0, fmt.Errorf("incidents: contar abiertas: %w", err)
	}
	return int(n), nil
}

// ListByParty devuelve las incidencias en que userID es quien reporta o la
// parte reportada, por createdAt descendente y, a igual fecha, id descendente.
// status "" = todas.
func (r *Repo) ListByParty(ctx context.Context, userID string, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error) {
	and := bson.A{bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "reporterId", Value: userID}},
		bson.D{{Key: "reportedId", Value: userID}},
	}}}}
	if status != "" {
		and = append(and, bson.D{{Key: "status", Value: string(status)}})
	}
	if after != nil {
		c, err := cursorFilter(after, "$lt")
		if err != nil {
			return nil, err
		}
		and = append(and, c)
	}
	return r.find(ctx, bson.D{{Key: "$and", Value: and}}, bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}, limit)
}

// ListAll devuelve las incidencias para administración por createdAt
// ascendente (la más antigua primero) y, a igual fecha, id ascendente.
// status "" = todas.
func (r *Repo) ListAll(ctx context.Context, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error) {
	and := bson.A{}
	if status != "" {
		and = append(and, bson.D{{Key: "status", Value: string(status)}})
	}
	if after != nil {
		c, err := cursorFilter(after, "$gt")
		if err != nil {
			return nil, err
		}
		and = append(and, c)
	}
	filter := bson.D{}
	if len(and) > 0 {
		filter = bson.D{{Key: "$and", Value: and}}
	}
	return r.find(ctx, filter, bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}, limit)
}

// cursorFilter arma la condición «después del cursor»: op es $lt (orden
// descendente) o $gt (ascendente).
func cursorFilter(c *domain.Cursor, op string) (bson.D, error) {
	oid, err := bson.ObjectIDFromHex(c.ID)
	if err != nil {
		return nil, fmt.Errorf("incidents: cursor inválido: %w", err)
	}
	at := c.CreatedAt.UTC()
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "createdAt", Value: bson.D{{Key: op, Value: at}}}},
		bson.D{{Key: "createdAt", Value: at}, {Key: "_id", Value: bson.D{{Key: op, Value: oid}}}},
	}}}, nil
}

func (r *Repo) find(ctx context.Context, filter, sort bson.D, limit int) ([]domain.Incident, error) {
	cur, err := r.col.Find(ctx, filter, options.Find().SetSort(sort).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("incidents: listar incidencias: %w", err)
	}
	var docs []incidentDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("incidents: leer incidencias: %w", err)
	}
	out := make([]domain.Incident, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// AddNote agrega la nota solo si la incidencia está abierta y tiene menos de
// MaxNotes notas, en una sola operación atómica; si no, usecase.ErrNoMatch.
func (r *Repo) AddNote(ctx context.Context, id string, n domain.Note) (domain.Incident, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Incident{}, domain.ErrNotFound
	}
	filter := bson.D{
		{Key: "_id", Value: oid},
		{Key: "status", Value: string(domain.StatusOpen)},
		// notes.<MaxNotes-1> existe solo si ya hay MaxNotes notas.
		{Key: fmt.Sprintf("notes.%d", domain.MaxNotes-1), Value: bson.D{{Key: "$exists", Value: false}}},
	}
	update := bson.D{
		{Key: "$push", Value: bson.D{{Key: "notes", Value: noteDoc{Text: n.Text, At: n.At.UTC(), ByAdminID: n.ByAdminID, VisibleToParties: n.VisibleToParties}}}},
		{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: n.At.UTC()}}},
		{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}},
	}
	return r.updateOne(ctx, filter, update)
}

// Close cierra la incidencia solo si está abierta, en una sola operación
// atómica; si no, usecase.ErrNoMatch.
func (r *Repo) Close(ctx context.Context, id string, f domain.FinalResponse) (domain.Incident, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Incident{}, domain.ErrNotFound
	}
	at := f.At.UTC()
	filter := bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusOpen)}}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "status", Value: string(domain.StatusClosed)},
			{Key: "finalResponse", Value: finalToDoc(f)},
			{Key: "closedAt", Value: at},
			{Key: "updatedAt", Value: at},
		}},
		{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}},
	}
	return r.updateOne(ctx, filter, update)
}

func (r *Repo) updateOne(ctx context.Context, filter, update bson.D) (domain.Incident, error) {
	var d incidentDoc
	err := r.col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.Incident{}, usecase.ErrNoMatch
	}
	if err != nil {
		return domain.Incident{}, fmt.Errorf("incidents: actualizar incidencia: %w", err)
	}
	return d.toDomain(), nil
}
