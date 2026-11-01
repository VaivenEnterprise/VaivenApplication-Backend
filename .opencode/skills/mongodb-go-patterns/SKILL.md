---
name: mongodb-go-patterns
description: Patrones de MongoDB con el driver oficial v2 de Go para VAIVÉN - conexión, repositorios, índices, validación de esquema, transacciones y pruebas con testcontainers. Úsala al tocar persistencia.
---

# Skill: MongoDB con Go (driver v2)

Imports base (driver v2): `go.mongodb.org/mongo-driver/v2/mongo`, `.../v2/mongo/options` y `.../v2/bson` (en v2 `bson.ObjectID` reemplaza a `primitive.ObjectID`; no existe `bson/primitive`). Ante duda de firma, verifica con `go doc`.

## Diseño de documentos
- **Incrustar** si se lee junto y la cardinalidad está acotada; **referenciar** si crece sin límite.
- **Instantánea** del dato que un acuerdo debe conservar (p. ej. precio aceptado).
- Colecciones `snake_case` plural; campos `camelCase`.
- Dinero: `amountCents int64`. Fechas: `time.Time` en UTC.
- Siempre `createdAt`, `updatedAt`; `version int` si hay concurrencia.

## Conexión (platform)
```go
func Connect(ctx context.Context, uri string) (*mongo.Client, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    client, err := mongo.Connect(options.Client().ApplyURI(uri))
    if err != nil {
        return nil, fmt.Errorf("mongo connect: %w", err)
    }
    if err := client.Ping(ctx, nil); err != nil {
        return nil, fmt.Errorf("mongo ping: %w", err)
    }
    return client, nil
}
```

## Modelo de persistencia separado del dominio
```go
// adapter/mongo — el dominio no conoce estos tags.
type thingDoc struct {
    ID        bson.ObjectID `bson:"_id,omitempty"`
    Name      string        `bson:"name"`
    CreatedAt time.Time     `bson:"createdAt"`
    UpdatedAt time.Time     `bson:"updatedAt"`
    Version   int           `bson:"version"`
}
func (d thingDoc) toDomain() domain.Thing { /* ... */ }
```

## Repositorio
```go
type ThingRepo struct{ col *mongo.Collection }

func NewThingRepo(db *mongo.Database) *ThingRepo { return &ThingRepo{col: db.Collection("things")} }

func (r *ThingRepo) FindByID(ctx context.Context, id string) (domain.Thing, error) {
    oid, err := bson.ObjectIDFromHex(id)
    if err != nil {
        return domain.Thing{}, &domain.Error{Code: domain.CodeNotFound, Message: "No se encontró el recurso."}
    }
    var doc thingDoc
    err = r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
    if errors.Is(err, mongo.ErrNoDocuments) {
        return domain.Thing{}, &domain.Error{Code: domain.CodeNotFound, Message: "No se encontró el recurso."}
    }
    if err != nil {
        return domain.Thing{}, fmt.Errorf("find thing: %w", err)
    }
    return doc.toDomain(), nil
}
```
Traduce `mongo.ErrNoDocuments` y errores de clave duplicada (`mongo.IsDuplicateKeyError`) a errores de dominio.

## Índices (idempotentes, al arrancar)
```go
func (r *ThingRepo) EnsureIndexes(ctx context.Context) error {
    _, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
        {Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uniq_name")},
    })
    return err
}
```
Cada índice se justifica con la consulta que sirve. Los repositorios se registran en un registro común que `main` recorre.

## Validación de esquema
Aplicar `$jsonSchema` con `collMod`/`create` en colecciones críticas (`bsonType`, `required`, rangos). La app valida primero; el esquema es la red de seguridad.

## Transacciones (solo si la spec las exige)
```go
sess, _ := client.StartSession()
defer sess.EndSession(ctx)
_, err := sess.WithTransaction(ctx, func(sc context.Context) (any, error) { /* operaciones con sc */ return nil, nil })
```
Requiere replica set. Prefiere atomicidad de un solo documento y operadores atómicos (`$set`, `$inc`) con filtro por `version`.

## Pruebas de integración
Usa `testcontainers-go/modules/mongodb` con `mongodb.Run(ctx, "mongo:7")`; obtén la URI con `ConnectionString`. Una base por prueba (nombre único). Nunca mocks de Mongo para repositorios.

## Lista de comprobación
- [ ] ¿Sin arreglos de crecimiento ilimitado?
- [ ] ¿Índice para cada consulta frecuente y para cada unicidad?
- [ ] ¿Sin `float`/`Decimal128` para dinero?
- [ ] ¿Errores del driver traducidos a errores de dominio?
- [ ] ¿Prueba de integración con Mongo real?
