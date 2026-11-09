// Package mongo implementa el repositorio de reportes con MongoDB (spec 011,
// T010; plan §3). Es el único paquete del módulo que importa el driver.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
)

// ReportsCollection es la colección de reportes.
const ReportsCollection = "reports"

// Repo guarda los reportes. Cumple usecase.Repository.
type Repo struct{ col *drv.Collection }

// NewRepo crea el repositorio sobre db.
func NewRepo(db *drv.Database) *Repo { return &Repo{col: db.Collection(ReportsCollection)} }

// EnsureIndexes crea los índices del plan §3: el único
// {reporterId, targetType, targetId}, que impide reportar dos veces lo mismo, y
// {status, createdAt} para la cola de revisión.
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{
			Keys:    bson.D{{Key: "reporterId", Value: 1}, {Key: "targetType", Value: 1}, {Key: "targetId", Value: 1}},
			Options: options.Index().SetName("reporter_target_unique").SetUnique(true),
		},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}, Options: options.Index().SetName("status_created")},
	})
	if err != nil {
		return fmt.Errorf("reports: crear índices: %w", err)
	}
	return nil
}

type resolutionDoc struct {
	Action    string    `bson:"action"`
	Note      string    `bson:"note"`
	ByAdminID string    `bson:"byAdminId"`
	At        time.Time `bson:"at"`
}

// reportDoc es el documento de reports (plan §3).
type reportDoc struct {
	ID         bson.ObjectID  `bson:"_id"`
	ReporterID string         `bson:"reporterId"`
	TargetType string         `bson:"targetType"`
	TargetID   string         `bson:"targetId"`
	Reason     string         `bson:"reason"`
	Details    *string        `bson:"details"`
	Status     string         `bson:"status"`
	Resolution *resolutionDoc `bson:"resolution"`
	CreatedAt  time.Time      `bson:"createdAt"`
}

func (d reportDoc) toDomain() domain.Report {
	r := domain.Report{
		ID: d.ID.Hex(), ReporterID: d.ReporterID, TargetType: domain.TargetType(d.TargetType), TargetID: d.TargetID,
		Reason: domain.Reason(d.Reason), Details: d.Details, Status: domain.Status(d.Status), CreatedAt: d.CreatedAt.UTC(),
	}
	if d.Resolution != nil {
		r.Resolution = &domain.Resolution{
			Action: domain.Action(d.Resolution.Action), Note: d.Resolution.Note, ByAdminID: d.Resolution.ByAdminID, At: d.Resolution.At.UTC(),
		}
	}
	return r
}

// NewID genera el identificador de un reporte.
func (r *Repo) NewID() string { return bson.NewObjectID().Hex() }

// Create guarda el reporte. Si la misma persona ya reportó lo mismo devuelve
// el existente y created=false (RF-08); dos reportes iguales a la vez chocan
// con el índice único y uno gana.
func (r *Repo) Create(ctx context.Context, rep domain.Report) (domain.Report, bool, error) {
	oid, err := bson.ObjectIDFromHex(rep.ID)
	if err != nil {
		return domain.Report{}, false, fmt.Errorf("reports: id inválido: %w", err)
	}
	doc := reportDoc{
		ID: oid, ReporterID: rep.ReporterID, TargetType: string(rep.TargetType), TargetID: rep.TargetID,
		Reason: string(rep.Reason), Details: rep.Details, Status: string(domain.StatusOpen), CreatedAt: rep.CreatedAt.UTC(),
	}
	if _, err := r.col.InsertOne(ctx, doc); err != nil {
		if !drv.IsDuplicateKeyError(err) {
			return domain.Report{}, false, fmt.Errorf("reports: guardar reporte: %w", err)
		}
		var existing reportDoc
		f := bson.D{{Key: "reporterId", Value: rep.ReporterID}, {Key: "targetType", Value: string(rep.TargetType)}, {Key: "targetId", Value: rep.TargetID}}
		if err := r.col.FindOne(ctx, f).Decode(&existing); err != nil {
			return domain.Report{}, false, fmt.Errorf("reports: buscar reporte existente: %w", err)
		}
		return existing.toDomain(), false, nil
	}
	return doc.toDomain(), true, nil
}

// ByID busca un reporte; si no existe, domain.ErrReportNotFound.
func (r *Repo) ByID(ctx context.Context, id string) (domain.Report, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Report{}, domain.ErrReportNotFound
	}
	var d reportDoc
	err = r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.Report{}, domain.ErrReportNotFound
	}
	if err != nil {
		return domain.Report{}, fmt.Errorf("reports: buscar reporte: %w", err)
	}
	return d.toDomain(), nil
}

// List devuelve los reportes con estado status, los más antiguos primero (por
// _id), con _id mayor que after ("" = desde el inicio).
func (r *Repo) List(ctx context.Context, status domain.Status, after string, limit int) ([]domain.Report, error) {
	f := bson.D{{Key: "status", Value: string(status)}}
	if after != "" {
		oid, err := bson.ObjectIDFromHex(after)
		if err != nil {
			return nil, fmt.Errorf("reports: cursor inválido: %w", err)
		}
		f = append(f, bson.E{Key: "_id", Value: bson.D{{Key: "$gt", Value: oid}}})
	}
	cur, err := r.col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("reports: listar reportes: %w", err)
	}
	var docs []reportDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("reports: leer reportes: %w", err)
	}
	out := make([]domain.Report, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// Resolve cierra el reporte solo si sigue open (RF-10, RF-11): guarda el estado
// final y la decisión con quién la tomó. Si ya estaba resuelto devuelve
// domain.ErrAlreadyResolved; si no existe, domain.ErrReportNotFound.
func (r *Repo) Resolve(ctx context.Context, id string, status domain.Status, res domain.Resolution) (domain.Report, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Report{}, domain.ErrReportNotFound
	}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: string(status)},
		{Key: "resolution", Value: resolutionDoc{Action: string(res.Action), Note: res.Note, ByAdminID: res.ByAdminID, At: res.At.UTC()}},
	}}}
	var d reportDoc
	err = r.col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusOpen)}}, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	if err == nil {
		return d.toDomain(), nil
	}
	if !errors.Is(err, drv.ErrNoDocuments) {
		return domain.Report{}, fmt.Errorf("reports: resolver reporte: %w", err)
	}
	// No estaba open: ¿existe?
	if n, cerr := r.col.CountDocuments(ctx, bson.D{{Key: "_id", Value: oid}}); cerr != nil {
		return domain.Report{}, fmt.Errorf("reports: buscar reporte: %w", cerr)
	} else if n > 0 {
		return domain.Report{}, domain.ErrAlreadyResolved
	}
	return domain.Report{}, domain.ErrReportNotFound
}
