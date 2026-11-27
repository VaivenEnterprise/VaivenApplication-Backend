// Package mongo implementa los repositorios de la cuenta con MongoDB (spec
// 003, plan §3; Art. II). Es el único paquete del módulo auth que importa el
// driver.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// UsersCollection es el nombre de la colección de cuentas.
const UsersCollection = "users"

// userDoc es el documento BSON de users; los campos coinciden con el JSON
// (camelCase, Art. 6.1 de la constitución).
type userDoc struct {
	ID              bson.ObjectID `bson:"_id,omitempty"`
	Email           string        `bson:"email"`
	PasswordHash    string        `bson:"passwordHash"`
	DisplayName     string        `bson:"displayName"`
	Phone           *string       `bson:"phone,omitempty"`
	Role            string        `bson:"role"`
	Status          string        `bson:"status"`
	AcceptedTermsAt time.Time     `bson:"acceptedTermsAt"`
	RatingSum       int64         `bson:"ratingSum"`
	RatingCount     int64         `bson:"ratingCount"`
	SuspendedUntil  *time.Time    `bson:"suspendedUntil,omitempty"`
	CreatedAt       time.Time     `bson:"createdAt"`
	UpdatedAt       time.Time     `bson:"updatedAt"`
	Version         int64         `bson:"version"`
	DeactivatedAt   *time.Time    `bson:"deactivatedAt,omitempty"`
}

func toDoc(u domain.User) (userDoc, error) {
	d := userDoc{
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		DisplayName:     u.DisplayName,
		Phone:           u.Phone,
		Role:            string(u.Role),
		Status:          string(u.Status),
		AcceptedTermsAt: u.AcceptedTermsAt,
		RatingSum:       u.RatingSum,
		RatingCount:     u.RatingCount,
		SuspendedUntil:  u.SuspendedUntil,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
		Version:         u.Version,
		DeactivatedAt:   u.DeactivatedAt,
	}
	if u.ID != "" {
		id, err := bson.ObjectIDFromHex(u.ID)
		if err != nil {
			return d, fmt.Errorf("users: id inválido: %w", err)
		}
		d.ID = id
	}
	return d, nil
}

func (d userDoc) toDomain() domain.User {
	return domain.User{
		ID:              d.ID.Hex(),
		Email:           d.Email,
		PasswordHash:    d.PasswordHash,
		DisplayName:     d.DisplayName,
		Phone:           d.Phone,
		Role:            domain.Role(d.Role),
		Status:          domain.Status(d.Status),
		AcceptedTermsAt: d.AcceptedTermsAt.UTC(),
		RatingSum:       d.RatingSum,
		RatingCount:     d.RatingCount,
		SuspendedUntil:  utcPtr(d.SuspendedUntil),
		CreatedAt:       d.CreatedAt.UTC(),
		UpdatedAt:       d.UpdatedAt.UTC(),
		Version:         d.Version,
		DeactivatedAt:   utcPtr(d.DeactivatedAt),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// UserRepo guarda las cuentas en la colección users.
type UserRepo struct {
	col *drv.Collection
}

// NewUserRepo crea el repositorio sobre db.
func NewUserRepo(db *drv.Database) *UserRepo {
	return &UserRepo{col: db.Collection(UsersCollection)}
}

// UserIndexes declara los índices de UsersCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func UserIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(UsersCollection,
		drv.IndexModel{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetName("email_unique").SetUnique(true),
		},
		// Bajas con pasos pendientes (spec 021): parcial, solo existe en las
		// pocas cuentas marcadas con deletionPending: true.
		drv.IndexModel{
			Keys: bson.D{{Key: "deletionPending", Value: 1}},
			Options: options.Index().SetName("deletionPending_partial").
				SetPartialFilterExpression(bson.D{{Key: "deletionPending", Value: true}}),
		},
	)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *UserRepo) Indexes() []adaptermongo.IndexSpec { return UserIndexes() }

// EnsureIndexes crea el índice único de correo (RF-02). Es idempotente.
func (r *UserRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("users: crear índices: %w", err)
	}
	return nil
}

// Create inserta la cuenta y devuelve la cuenta con su ID. Un correo repetido
// devuelve domain.ErrEmailTaken.
func (r *UserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	u.ID = ""
	d, err := toDoc(u)
	if err != nil {
		return domain.User{}, err
	}
	d.ID = bson.NewObjectID()
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		if drv.IsDuplicateKeyError(err) {
			return domain.User{}, domain.ErrEmailTaken.Wrap(err)
		}
		return domain.User{}, fmt.Errorf("users: insertar: %w", err)
	}
	return d.toDomain(), nil
}

// ByEmail busca por correo normalizado; si no existe devuelve
// domain.ErrUserNotFound.
func (r *UserRepo) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

// ByID busca por ID; un ID mal formado también es domain.ErrUserNotFound.
func (r *UserRepo) ByID(ctx context.Context, id string) (domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.findOne(ctx, bson.D{{Key: "_id", Value: oid}})
}

func (r *UserRepo) findOne(ctx context.Context, filter bson.D) (domain.User, error) {
	var d userDoc
	if err := r.col.FindOne(ctx, filter).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("users: buscar: %w", err)
	}
	return d.toDomain(), nil
}

// UpdateProfile aplica los cambios a una cuenta activa y devuelve la cuenta
// actualizada; si no existe o se dio de baja devuelve domain.ErrUserNotFound.
func (r *UserRepo) UpdateProfile(ctx context.Context, id string, p usecase.ProfileChanges, now time.Time) (domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	set := bson.D{{Key: "updatedAt", Value: now}}
	if p.DisplayName != nil {
		set = append(set, bson.E{Key: "displayName", Value: *p.DisplayName})
	}
	update := bson.D{{Key: "$set", Value: set}, {Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}}}
	if p.ClearPhone {
		// Quitar un valor es $unset, nunca $set: null (spec 021, RF-10).
		update = append(update, bson.E{Key: "$unset", Value: bson.D{{Key: "phone", Value: ""}}})
	} else if p.Phone != nil {
		set = append(set, bson.E{Key: "phone", Value: *p.Phone})
		update[0].Value = set
	}
	var d userDoc
	err = r.col.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusActive)}},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("users: actualizar perfil: %w", err)
	}
	return d.toDomain(), nil
}

// UpdatePasswordHash reemplaza el hash de una cuenta activa; si no existe o se
// dio de baja devuelve domain.ErrUserNotFound.
func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, passwordHash string, now time.Time) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrUserNotFound
	}
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusActive)}},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "passwordHash", Value: passwordHash}, {Key: "updatedAt", Value: now}}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
	if err != nil {
		return fmt.Errorf("users: actualizar hash: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

// LockActive es el cerrojo de la baja (spec 021): $inc de version sobre la
// cuenta activa. Dentro de una transacción, dos bajas simultáneas chocan aquí
// (WriteConflict) y la segunda ve la cuenta ya dada de baja. false si no existe
// o ya no está activa.
func (r *UserRepo) LockActive(ctx context.Context, id string) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, nil
	}
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusActive)}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}}})
	if err != nil {
		return false, fmt.Errorf("users: cerrojo de la baja: %w", err)
	}
	return res.MatchedCount == 1, nil
}

// Deactivate da de baja la cuenta activa (spec 021, RF-02, §5.2 de la spec): deja
// solo los campos que se conservan. Reemplaza correo y nombre, fija estado y
// fecha de baja, marca deletionPending (hay pasos posteriores al commit) y
// QUITA con $unset passwordHash, phone, suspendedUntil, role, ratingSum y
// ratingCount; ningún campo queda en null. acceptedTermsAt se conserva (D-6).
// El correo anónimo es único, así que el índice se conserva y el original
// queda libre. Si la cuenta no existe o ya no está activa devuelve
// domain.ErrUserNotFound (el cerrojo status: active hace la baja atómica).
func (r *UserRepo) Deactivate(ctx context.Context, id, anonymizedEmail string, now time.Time) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrUserNotFound
	}
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusActive)}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "email", Value: anonymizedEmail},
				{Key: "displayName", Value: domain.DeletedDisplayName},
				{Key: "status", Value: string(domain.StatusDeactivated)},
				{Key: "deactivatedAt", Value: now},
				{Key: "updatedAt", Value: now},
				{Key: "deletionPending", Value: true},
			}},
			{Key: "$unset", Value: bson.D{
				{Key: "passwordHash", Value: ""}, {Key: "phone", Value: ""}, {Key: "suspendedUntil", Value: ""},
				{Key: "role", Value: ""}, {Key: "ratingSum", Value: ""}, {Key: "ratingCount", Value: ""},
			}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
	if err != nil {
		return fmt.Errorf("users: baja: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

// ListPending devuelve hasta limit cuentas con pasos pendientes de la baja
// (deletionPending: true), de la más antigua a la más reciente.
func (r *UserRepo) ListPending(ctx context.Context, limit int) ([]string, error) {
	cur, err := r.col.Find(ctx, bson.D{{Key: "deletionPending", Value: true}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("users: bajas pendientes: %w", err)
	}
	var docs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("users: bajas pendientes: %w", err)
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.ID.Hex())
	}
	return out, nil
}

// ClearIfDone quita la marca deletionPending con $unset (nunca la deja en
// null). Devuelve true si la cuenta la tenía. Quien llama ya comprobó que no
// quedan pasos pendientes.
func (r *UserRepo) ClearIfDone(ctx context.Context, id string, now time.Time) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, nil
	}
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "deletionPending", Value: true}},
		bson.D{
			{Key: "$unset", Value: bson.D{{Key: "deletionPending", Value: ""}}},
			{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: now}}},
		})
	if err != nil {
		return false, fmt.Errorf("users: quitar marca de baja: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// PublicUsers implementa users.Directory: perfiles públicos por lote. Los ids
// mal formados o inexistentes no aparecen en el resultado.
func (r *UserRepo) PublicUsers(ctx context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := make(map[string]users.PublicUser, len(ids))
	oids := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return out, nil
	}
	cur, err := r.col.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}},
		options.Find().SetProjection(bson.D{
			{Key: "displayName", Value: 1}, {Key: "status", Value: 1}, {Key: "ratingSum", Value: 1},
			{Key: "ratingCount", Value: 1}, {Key: "createdAt", Value: 1},
		}))
	if err != nil {
		return nil, fmt.Errorf("users: perfiles públicos: %w", err)
	}
	var docs []userDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("users: perfiles públicos: %w", err)
	}
	for _, d := range docs {
		p := d.toDomain().Public()
		out[p.ID] = users.PublicUser{
			ID: p.ID, DisplayName: p.DisplayName, RatingAverage: p.RatingAverage,
			RatingCount: p.RatingCount, MemberSince: p.MemberSince,
		}
	}
	return out, nil
}

// Suspend implementa users.Admin: fija suspendedUntil (RF-13). Solo sobre
// cuentas activas: sobre una cuenta ya dada de baja no hace nada y no es error
// (recrearía un campo que la baja quitó, spec 021 plan §6).
func (r *UserRepo) Suspend(ctx context.Context, userID string, until time.Time) error {
	return r.updateActive(ctx, userID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "suspendedUntil", Value: until.UTC()},
		{Key: "updatedAt", Value: time.Now().UTC()},
	}}})
}

// AddRating implementa users.Admin: suma una calificación de 1 a 5. Sobre una
// cuenta dada de baja no hace nada y no es error: la estrella ya quedó en
// ratings y no se acumula en un perfil que no se muestra.
func (r *UserRepo) AddRating(ctx context.Context, userID string, score int) error {
	if score < 1 || score > 5 {
		return fmt.Errorf("users: calificación fuera de rango: %d", score)
	}
	return r.updateActive(ctx, userID, bson.D{{Key: "$inc", Value: bson.D{
		{Key: "ratingSum", Value: int64(score)},
		{Key: "ratingCount", Value: int64(1)},
	}}})
}

// updateActive aplica update solo si la cuenta está activa. Si no coincide,
// distingue entre una cuenta que no existe (domain.ErrUserNotFound) y una que
// existe pero no está activa (sin error y sin cambios).
func (r *UserRepo) updateActive(ctx context.Context, id string, update bson.D) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrUserNotFound
	}
	res, err := r.col.UpdateOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: string(domain.StatusActive)}}, update)
	if err != nil {
		return fmt.Errorf("users: actualizar: %w", err)
	}
	if res.MatchedCount > 0 {
		return nil
	}
	n, err := r.col.CountDocuments(ctx, bson.D{{Key: "_id", Value: oid}}, options.Count().SetLimit(1))
	if err != nil {
		return fmt.Errorf("users: actualizar: %w", err)
	}
	if n == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

// Comprobaciones en compilación de los puertos que implementa UserRepo.
var (
	_ usecase.Users            = (*UserRepo)(nil)
	_ users.Directory          = (*UserRepo)(nil)
	_ users.Admin              = (*UserRepo)(nil)
	_ usecase.PendingDeletions = (*UserRepo)(nil)
)
