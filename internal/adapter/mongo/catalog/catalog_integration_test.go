//go:build integration

package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// base es una fecha futura: los TTL (refresh_tokens, notifications) no borran
// los documentos de prueba mientras corre.
var base = time.Now().Add(365 * 24 * time.Hour).UTC().Truncate(time.Second)

// seedDoc es un documento de prueba con todos los campos que indexan las
// colecciones, con valores repetidos (selectividad realista) y únicos donde
// hay índices únicos. No representa datos reales.
func seedDoc(i int) bson.D {
	var readAt any = base.Add(time.Duration(i) * time.Minute)
	if i%4 == 0 {
		readAt = nil
	}
	return bson.D{
		{Key: "userId", Value: fmt.Sprintf("u%d", i%6)},
		{Key: "ownerId", Value: fmt.Sprintf("o%d", i%6)},
		{Key: "requesterId", Value: fmt.Sprintf("r%d", i%6)},
		{Key: "itemId", Value: fmt.Sprintf("i%d", i%6)},
		{Key: "familyId", Value: fmt.Sprintf("f%d", i%6)},
		{Key: "status", Value: []string{"a", "b", "c"}[i%3]},
		{Key: "category", Value: fmt.Sprintf("cat%d", i%4)},
		{Key: "modality", Value: fmt.Sprintf("m%d", i%2)},
		{Key: "zone", Value: bson.D{{Key: "municipality", Value: fmt.Sprintf("mun%d", i%5)}}},
		{Key: "createdAt", Value: base.Add(time.Duration(i) * time.Minute)},
		{Key: "expiresAt", Value: base.Add(time.Duration(i) * time.Hour)},
		{Key: "lateAt", Value: base.Add(time.Duration(59-i) * time.Hour)},
		{Key: "startDate", Value: base.Add(time.Duration(i) * 24 * time.Hour)},
		{Key: "endDate", Value: base.Add(time.Duration(i+2) * 24 * time.Hour)},
		{Key: "participantIds", Value: bson.A{fmt.Sprintf("p%d", i%6), fmt.Sprintf("p%d", (i+1)%6)}},
		{Key: "lastMessageAt", Value: base.Add(time.Duration(i) * time.Minute)},
		{Key: "conversationId", Value: fmt.Sprintf("c%d", i%6)},
		{Key: "seq", Value: i},
		{Key: "senderId", Value: fmt.Sprintf("s%d", i%2)},
		{Key: "clientId", Value: fmt.Sprintf("cl%d", i)},
		{Key: "bookingId", Value: fmt.Sprintf("b%d", i)},
		{Key: "raterId", Value: fmt.Sprintf("ra%d", i)},
		{Key: "rateeId", Value: fmt.Sprintf("re%d", i%6)},
		{Key: "reporterId", Value: fmt.Sprintf("rp%d", i%6)},
		{Key: "reportedId", Value: fmt.Sprintf("rd%d", i%6)},
		{Key: "targetType", Value: "item"},
		{Key: "targetId", Value: fmt.Sprintf("t%d", i)},
		{Key: "tokenHash", Value: fmt.Sprintf("h%d", i)},
		{Key: "token", Value: fmt.Sprintf("tok%d", i)},
		{Key: "email", Value: fmt.Sprintf("e%d@example.com", i)},
		{Key: "readAt", Value: readAt},
	}
}

// queryCase es la consulta representativa de un índice de consulta (CA-10a).
type queryCase struct {
	filter bson.D
	sort   bson.D
}

// queries tiene una consulta por cada índice de propósito forQuery.
var queries = map[string]queryCase{
	"refresh_tokens.familyId": {filter: bson.D{{Key: "familyId", Value: "f0"}}},
	"refresh_tokens.userId":   {filter: bson.D{{Key: "userId", Value: "u0"}}},

	"bookings.item_status_dates": {filter: bson.D{{Key: "itemId", Value: "i0"},
		{Key: "status", Value: bson.D{{Key: "$in", Value: bson.A{"a", "b"}}}},
		{Key: "startDate", Value: bson.D{{Key: "$lte", Value: base.Add(30 * 24 * time.Hour)}}},
		{Key: "endDate", Value: bson.D{{Key: "$gte", Value: base}}}}},
	"bookings.requester_status_createdAt": {filter: bson.D{{Key: "requesterId", Value: "r0"}, {Key: "status", Value: "a"}},
		sort: bson.D{{Key: "createdAt", Value: -1}}},
	"bookings.owner_status_createdAt": {filter: bson.D{{Key: "ownerId", Value: "o0"}, {Key: "status", Value: "a"}},
		sort: bson.D{{Key: "createdAt", Value: -1}}},
	"bookings.status_expiresAt": {filter: bson.D{{Key: "status", Value: "a"},
		{Key: "expiresAt", Value: bson.D{{Key: "$lte", Value: base.Add(10 * time.Hour)}}}}},
	"bookings.status_lateAt": {filter: bson.D{{Key: "status", Value: "a"},
		{Key: "lateAt", Value: bson.D{{Key: "$lte", Value: base.Add(10 * time.Hour)}}}}},

	"items.owner_status_createdAt": {filter: bson.D{{Key: "ownerId", Value: "o0"}, {Key: "status", Value: "a"}},
		sort: bson.D{{Key: "createdAt", Value: -1}}},
	"items.status_category_createdAt": {filter: bson.D{{Key: "status", Value: "a"}, {Key: "category", Value: "cat0"}},
		sort: bson.D{{Key: "createdAt", Value: -1}}},
	"items.search_filters": {filter: bson.D{{Key: "status", Value: "a"}, {Key: "category", Value: "cat0"},
		{Key: "modality", Value: "m0"}, {Key: "zone.municipality", Value: "mun0"}}, sort: bson.D{{Key: "createdAt", Value: -1}}},
	"items.search_recent": {filter: bson.D{{Key: "status", Value: "a"}},
		sort: bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}},

	"conversations.participant_lastMessageAt": {filter: bson.D{{Key: "participantIds", Value: "p0"}},
		sort: bson.D{{Key: "lastMessageAt", Value: -1}, {Key: "_id", Value: -1}}},
	"messages.conversation_seq": {filter: bson.D{{Key: "conversationId", Value: "c0"}},
		sort: bson.D{{Key: "seq", Value: -1}, {Key: "_id", Value: -1}}},
	"ratings.ratee_id": {filter: bson.D{{Key: "rateeId", Value: "re0"}}, sort: bson.D{{Key: "_id", Value: -1}}},

	"incidents.booking_status":     {filter: bson.D{{Key: "bookingId", Value: "b0"}, {Key: "status", Value: "a"}}},
	"incidents.reporter_createdAt": {filter: bson.D{{Key: "reporterId", Value: "rp0"}}, sort: bson.D{{Key: "createdAt", Value: -1}}},
	"incidents.reported_createdAt": {filter: bson.D{{Key: "reportedId", Value: "rd0"}}, sort: bson.D{{Key: "createdAt", Value: -1}}},
	"incidents.status_createdAt":   {filter: bson.D{{Key: "status", Value: "a"}}, sort: bson.D{{Key: "createdAt", Value: 1}}},

	"sanctions.user_createdAt": {filter: bson.D{{Key: "userId", Value: "u0"}}, sort: bson.D{{Key: "createdAt", Value: -1}}},

	"notifications.user_id":   {filter: bson.D{{Key: "userId", Value: "u0"}}, sort: bson.D{{Key: "_id", Value: -1}}},
	"notifications.user_read": {filter: bson.D{{Key: "userId", Value: "u0"}, {Key: "readAt", Value: nil}}},
	"push_tokens.user":        {filter: bson.D{{Key: "userId", Value: "u0"}}},

	"reports.status_created": {filter: bson.D{{Key: "status", Value: "a"}}, sort: bson.D{{Key: "createdAt", Value: 1}}},
}

// TestQueriesCoverEveryQueryIndex: cada índice de consulta tiene su consulta
// y no hay consultas de índices que no son de consulta.
func TestQueriesCoverEveryQueryIndex(t *testing.T) {
	for id, p := range purpose {
		_, ok := queries[id]
		if p.kind == forQuery && !ok {
			t.Errorf("%s: índice de consulta sin consulta representativa", id)
		}
		if p.kind != forQuery && ok {
			t.Errorf("%s: tiene consulta pero su propósito es %s", id, p.kind)
		}
	}
	for id := range queries {
		if _, ok := purpose[id]; !ok {
			t.Errorf("%s: consulta de un índice sin propósito", id)
		}
	}
}

// Prueba 13 (CA-10a): con los índices del catálogo y datos de prueba, el
// planificador elige cada índice de consulta para su consulta representativa.
// Para TTL, únicos, texto y 2dsphere no se exige IXSCAN.
func TestIntegrationQueryIndexesAreUsed(t *testing.T) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker o Podman: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("vaiven_explain")

	byCol := map[string][]adaptermongo.IndexSpec{}
	for _, s := range All() {
		byCol[s.Collection] = append(byCol[s.Collection], s)
	}
	for col, specs := range byCol {
		if _, err := db.Collection(col).Indexes().CreateMany(ctx, adaptermongo.Models(specs)); err != nil {
			t.Fatalf("crear índices de %s: %v", col, err)
		}
		docs := make([]any, 60)
		for i := range docs {
			docs[i] = seedDoc(i)
		}
		if _, err := db.Collection(col).InsertMany(ctx, docs); err != nil {
			t.Fatalf("sembrar %s: %v", col, err)
		}
	}

	for id, q := range queries {
		col, name := splitID(id)
		used, err := winningIndexes(ctx, db, col, q)
		if err != nil {
			t.Errorf("%s: explain: %v", id, err)
			continue
		}
		if !used[name] {
			t.Errorf("%s: el plan ganador usa %v, no %s", id, keys(used), name)
		}
	}
}

func splitID(id string) (string, string) {
	for i := 0; i < len(id); i++ {
		if id[i] == '.' {
			return id[:i], id[i+1:]
		}
	}
	return id, ""
}

// winningIndexes devuelve los índices (indexName) del plan ganador.
func winningIndexes(ctx context.Context, db *drv.Database, col string, q queryCase) (map[string]bool, error) {
	find := bson.D{{Key: "find", Value: col}, {Key: "filter", Value: q.filter}}
	if len(q.sort) > 0 {
		find = append(find, bson.E{Key: "sort", Value: q.sort})
	}
	var res bson.D
	if err := db.RunCommand(ctx, bson.D{{Key: "explain", Value: find}, {Key: "verbosity", Value: "queryPlanner"}}).Decode(&res); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	collectIndexNames(field(field(res, "queryPlanner"), "winningPlan"), out)
	return out, nil
}

// field devuelve el campo key de un documento (bson.D o bson.M).
func field(v any, key string) any {
	switch d := v.(type) {
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	case bson.M:
		return d[key]
	}
	return nil
}

func collectIndexNames(v any, out map[string]bool) {
	switch x := v.(type) {
	case bson.M:
		if n, ok := x["indexName"].(string); ok {
			out[n] = true
		}
		for _, e := range x {
			collectIndexNames(e, out)
		}
	case bson.D:
		for _, e := range x {
			if e.Key == "indexName" {
				if n, ok := e.Value.(string); ok {
					out[n] = true
				}
			}
			collectIndexNames(e.Value, out)
		}
	case bson.A:
		for _, e := range x {
			collectIndexNames(e, out)
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
