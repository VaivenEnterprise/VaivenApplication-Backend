//go:build integration

package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	aimongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/adapter/mongo"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	assistantmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	chatdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	incidentsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	notificationsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	ratingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
	recodomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	reportsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
	sanctionsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
)

// CA-02 (Fase 1) sobre MongoDB real: tras la siembra completa y un recorrido de
// escrituras de todos los módulos con documentos opcionales vacíos,
// `verify-no-nulls` sale con 0. Todos los validadores se aplican en modo
// ESTRICTO (T030: sin null, acción error): cualquier null habría fallado la
// escritura con el código 121 además de fallar la verificación.
func TestIntegrationZeroNullsAfterSeedAndWrites(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_zero_nulls")
	ctx := context.Background()
	cfg := seedTestConfig()
	open := func(ctx context.Context, _ config.Config, _ bool) (seedDeps, error) {
		return seedDepsOn(ctx, cfg, db, localImages{baseFolder: cfg.Cloudinary.BaseFolder}, localPhotos{baseFolder: cfg.Cloudinary.BaseFolder})
	}
	if _, err := adaptermongo.EnforceValidators(ctx, db, allValidatorSpecs(), true); err != nil {
		t.Fatalf("validadores: %v", err)
	}
	path := writeCreds(t, nil)
	var out, errb bytes.Buffer
	if code := runSeed(nil, &out, &errb, cfg, func(string) string { return path }, open); code != 0 {
		t.Fatalf("seed: código %d\n%s\n%s", code, out.String(), errb.String())
	}
	if code, o := verifyOn(t, db); code != verifyNullsClean {
		t.Fatalf("tras la siembra: verify = %d, want 0\n%s", code, o)
	}

	// Personas y una reserva de la siembra.
	var ids []string
	cur, err := db.Collection("users").Find(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	var us []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &us); err != nil || len(us) < 4 {
		t.Fatalf("users = %d (%v)", len(us), err)
	}
	for _, u := range us {
		ids = append(ids, u.ID.Hex())
	}
	var bk struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := db.Collection("bookings").FindOne(ctx, bson.D{}).Decode(&bk); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)

	// auth: perfil sin teléfono y baja.
	userRepo := authmongo.NewUserRepo(db)
	if _, err := userRepo.UpdateProfile(ctx, ids[1], authusecase.ProfileChanges{ClearPhone: true}, now); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if err := userRepo.Deactivate(ctx, ids[3], "deleted+"+ids[3]+"@vaiven.invalid", now); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if err := authmongo.NewRefreshRepo(db).Create(ctx, authdomain.RefreshToken{UserID: ids[0], FamilyID: "f", TokenHash: "h", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// chat: un mensaje (conversación con lastMessage) sin clientId.
	convs, err := db.Collection("conversations").Find(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	var cs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := convs.All(ctx, &cs); err != nil || len(cs) == 0 {
		t.Fatalf("conversaciones: %v", err)
	}
	msg := chatdomain.Message{ID: bson.NewObjectID().Hex(), ConversationID: cs[0].ID.Hex(), SenderID: ids[0], Body: "hola", CreatedAt: now}
	if _, _, err := chatmongo.NewSender(db).Send(ctx, msg, ids[1], chatdomain.LastMessage{Preview: "hola", At: now, SenderID: ids[0]}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// ratings sin comentario.
	rr := ratingsmongo.NewRepo(db, userRepo)
	if _, err := rr.Create(ctx, ratingsdomain.Rating{ID: rr.NewID(), BookingID: bk.ID.Hex(), RaterID: ids[0], RateeID: ids[1], RatedRole: "owner", Score: 5, CreatedAt: now}); err != nil {
		t.Fatalf("rating: %v", err)
	}

	// moderación: reporte sin detalle, incidencia abierta y sanción sin fin ni incidencia.
	rep := reportsmongo.NewRepo(db)
	if _, _, err := rep.Create(ctx, reportsdomain.Report{ID: rep.NewID(), ReporterID: ids[0], TargetType: reportsdomain.TargetUser, TargetID: ids[1], Reason: reportsdomain.ReasonSpam, CreatedAt: now}); err != nil {
		t.Fatalf("report: %v", err)
	}
	inc := incidentsmongo.NewRepo(db)
	if _, err := inc.Create(ctx, incidentsdomain.Incident{ID: inc.NewID(), BookingID: bk.ID.Hex(), ReporterID: ids[0], ReportedID: ids[1], Reason: incidentsdomain.ReasonNoShow, Description: "no llegó", Status: incidentsdomain.StatusOpen, CreatedAt: now, UpdatedAt: now, Version: 1}); err != nil {
		t.Fatalf("incident: %v", err)
	}
	san := sanctionsmongo.NewRepo(db)
	if _, err := san.Create(ctx, sanctionsdomain.Sanction{ID: san.NewID(), UserID: ids[1], Type: sanctionsdomain.TypeWarning, Reason: "aviso", CreatedByAdminID: ids[0], CreatedAt: now}); err != nil {
		t.Fatalf("sanction: %v", err)
	}

	// notificaciones: aviso sin leer (sin data) y token de push.
	if err := notificationsmongo.NewNotificationRepo(db).Save(ctx, notify.Record{UserID: ids[1], Type: "x", Title: "t", Body: "b", CreatedAt: now}); err != nil {
		t.Fatalf("notification: %v", err)
	}
	if err := notificationsmongo.NewTokenRepo(db).Upsert(ctx, notificationsdomain.PushToken{Token: "tok", UserID: ids[1], Platform: "android", CreatedAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("push token: %v", err)
	}

	// ai, asistente y reco.
	if _, err := aimongo.NewUsageRepo(db).Take(ctx, ids[0], "assistant", "2026-11-26", 5, now); err != nil {
		t.Fatalf("ai usage: %v", err)
	}
	if _, err := assistantmongo.NewConversationRepo(db).Create(ctx, ids[0], []aidomain.Turn{{Role: "user", Text: "hola"}}, now); err != nil {
		t.Fatalf("assistant: %v", err)
	}
	meta := recomongo.NewMetaRepo(db)
	if ok, err := meta.TryLock(ctx, "owner", now); err != nil || !ok {
		t.Fatalf("TryLock = %v, %v", ok, err)
	}
	if err := meta.SetReset(ctx, ids[0], now); err != nil {
		t.Fatalf("SetReset: %v", err)
	}
	if err := recomongo.NewModelRepo(db).Save(ctx, "b1", map[string][]recodomain.Neighbor{"i1": nil}, map[string][]string{"all": nil}, now); err != nil {
		t.Fatalf("model: %v", err)
	}

	if code, o := verifyOn(t, db); code != verifyNullsClean {
		t.Fatalf("tras el recorrido de escrituras: verify = %d, want 0\n%s", code, o)
	}
}
