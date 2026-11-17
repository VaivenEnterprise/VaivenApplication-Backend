// Package mongo implementa los repositorios de avisos y tokens de push con
// MongoDB (spec 011, T003; plan §3). Es el único paquete del módulo que
// importa el driver.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// NotificationsCollection es la colección de avisos.
const NotificationsCollection = "notifications"

// RetentionSeconds son los 90 días que se conservan los avisos (RNF-01).
const RetentionSeconds = int32(domain.RetentionDays * 24 * 60 * 60)

// NotificationRepo guarda los avisos. Cumple notify.Store y
// usecase.Repository.
type NotificationRepo struct{ col *drv.Collection }

var _ notify.Store = (*NotificationRepo)(nil)

// NewNotificationRepo crea el repositorio sobre db.
func NewNotificationRepo(db *drv.Database) *NotificationRepo {
	return &NotificationRepo{col: db.Collection(NotificationsCollection)}
}

// NotificationIndexes declara los índices de NotificationsCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func NotificationIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(NotificationsCollection, []drv.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("user_id")},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "readAt", Value: 1}}, Options: options.Index().SetName("user_read")},
		{Keys: bson.D{{Key: "createdAt", Value: 1}}, Options: options.Index().SetName("created_ttl").SetExpireAfterSeconds(RetentionSeconds)},
	}...)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *NotificationRepo) Indexes() []adaptermongo.IndexSpec { return NotificationIndexes() }

// EnsureIndexes crea los índices del plan §3: {userId, _id desc} para listar,
// {userId, readAt} para contar los no leídos y el TTL de 90 días sobre
// createdAt (RNF-01).
func (r *NotificationRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("notifications: crear índices: %w", err)
	}
	return nil
}

// notificationDoc es el documento de notifications (plan §3).
type notificationDoc struct {
	ID        bson.ObjectID     `bson:"_id"`
	UserID    string            `bson:"userId"`
	Type      string            `bson:"type"`
	Title     string            `bson:"title"`
	Body      string            `bson:"body"`
	Data      map[string]string `bson:"data"`
	ReadAt    *time.Time        `bson:"readAt"`
	CreatedAt time.Time         `bson:"createdAt"`
}

func (d notificationDoc) toDomain() domain.Notification {
	data := d.Data
	if data == nil {
		data = map[string]string{}
	}
	n := domain.Notification{ID: d.ID.Hex(), UserID: d.UserID, Type: d.Type, Title: d.Title, Body: d.Body, Data: data, CreatedAt: d.CreatedAt.UTC()}
	if d.ReadAt != nil {
		t := d.ReadAt.UTC()
		n.ReadAt = &t
	}
	return n
}

// Save inserta el aviso sin leer (notify.Store).
func (r *NotificationRepo) Save(ctx context.Context, rec notify.Record) error {
	data := rec.Data
	if data == nil {
		data = map[string]string{}
	}
	doc := notificationDoc{
		ID: bson.NewObjectID(), UserID: rec.UserID, Type: rec.Type, Title: rec.Title, Body: rec.Body,
		Data: data, CreatedAt: rec.CreatedAt.UTC(),
	}
	if _, err := r.col.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("notifications: guardar aviso: %w", err)
	}
	return nil
}

// List devuelve los avisos de userID, los más recientes primero (por _id), con
// _id menor que after ("" = desde el inicio).
func (r *NotificationRepo) List(ctx context.Context, userID, after string, limit int) ([]domain.Notification, error) {
	f := bson.D{{Key: "userId", Value: userID}}
	if after != "" {
		oid, err := bson.ObjectIDFromHex(after)
		if err != nil {
			return nil, fmt.Errorf("notifications: cursor inválido: %w", err)
		}
		f = append(f, bson.E{Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}})
	}
	cur, err := r.col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("notifications: listar avisos: %w", err)
	}
	var docs []notificationDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("notifications: leer avisos: %w", err)
	}
	out := make([]domain.Notification, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// CountUnread cuenta los avisos de userID con readAt nulo.
func (r *NotificationRepo) CountUnread(ctx context.Context, userID string) (int64, error) {
	n, err := r.col.CountDocuments(ctx, bson.D{{Key: "userId", Value: userID}, {Key: "readAt", Value: nil}})
	if err != nil {
		return 0, fmt.Errorf("notifications: contar no leídos: %w", err)
	}
	return n, nil
}

// MarkRead marca como leídos los avisos ids de userID que sigan sin leer. Los
// ids ajenos o inexistentes simplemente no coinciden.
func (r *NotificationRepo) MarkRead(ctx context.Context, userID string, ids []string, at time.Time) error {
	oids := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		oid, err := bson.ObjectIDFromHex(id)
		if err != nil {
			return fmt.Errorf("notifications: id inválido: %w", err)
		}
		oids = append(oids, oid)
	}
	f := bson.D{{Key: "userId", Value: userID}, {Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}, {Key: "readAt", Value: nil}}
	if _, err := r.col.UpdateMany(ctx, f, bson.D{{Key: "$set", Value: bson.D{{Key: "readAt", Value: at.UTC()}}}}); err != nil {
		return fmt.Errorf("notifications: marcar leídos: %w", err)
	}
	return nil
}

// MarkAllRead marca como leídos todos los avisos sin leer de userID.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID string, at time.Time) error {
	f := bson.D{{Key: "userId", Value: userID}, {Key: "readAt", Value: nil}}
	if _, err := r.col.UpdateMany(ctx, f, bson.D{{Key: "$set", Value: bson.D{{Key: "readAt", Value: at.UTC()}}}}); err != nil {
		return fmt.Errorf("notifications: marcar todos leídos: %w", err)
	}
	return nil
}
