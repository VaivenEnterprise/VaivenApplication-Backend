---
name: go-clean-architecture
description: Reglas y plantillas de Clean Architecture en Go para el backend de VAIVÉN (domain, usecase, adapter, platform, cmd). Úsala al crear o modificar paquetes, casos de uso, handlers o la composición en main.
---

# Skill: Clean Architecture en Go

## Capas y dependencias (solo hacia adentro)
```
cmd/api  →  adapter/{http,mongo}  →  usecase  →  domain
                  ↑ platform (logger, cliente Mongo, reloj, ids)
```
| Capa | Contiene | Prohibido importar |
|---|---|---|
| `domain` | Entidades, objetos de valor, errores de dominio | cualquier paquete que no sea stdlib |
| `usecase` | Casos de uso + puertos (interfaces) | `net/http`, `chi`, `bson`, `mongo` |
| `adapter/http` | Router, handlers, middleware, DTO | `bson`, `mongo` |
| `adapter/mongo` | Repositorios, mapeo BSON, índices | `net/http`, `chi` |
| `platform` | Infra genérica reutilizable | reglas de negocio |
| `cmd/api` | Cableado de dependencias | lógica de negocio |

## Plantilla de error de dominio
```go
package domain

type Code string

const (
    CodeValidation Code = "validation_failed"
    CodeNotFound   Code = "not_found"
    CodeConflict   Code = "conflict"
    CodeUnauthorized Code = "unauthorized"
    CodeForbidden  Code = "forbidden"
)

type Error struct {
    Code    Code
    Message string // en español, apto para mostrar
    Details []Detail
    Err     error
}

type Detail struct{ Field, Issue string }

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Err }
```

## Plantilla de puerto y caso de uso
```go
package usecase

// El puerto lo declara quien lo consume.
type ThingRepository interface {
    FindByID(ctx context.Context, id string) (domain.Thing, error)
}

type GetThing struct{ repo ThingRepository }

func NewGetThing(repo ThingRepository) *GetThing { return &GetThing{repo: repo} }

func (uc *GetThing) Execute(ctx context.Context, id string) (domain.Thing, error) {
    t, err := uc.repo.FindByID(ctx, id)
    if err != nil {
        return domain.Thing{}, fmt.Errorf("get thing: %w", err)
    }
    return t, nil
}
```

## Plantilla de handler
```go
func (h *ThingHandler) Get(w http.ResponseWriter, r *http.Request) {
    thing, err := h.get.Execute(r.Context(), chi.URLParam(r, "id"))
    if err != nil {
        respond.Error(w, r, err) // traduce domain.Error → código HTTP + envoltorio
        return
    }
    respond.JSON(w, http.StatusOK, toThingDTO(thing))
}
```

## Composición (`cmd/api/main.go`)
Crea config → logger → cliente Mongo → repositorios → casos de uso → handlers → router → servidor. Sin `init()` ni globales.

## Lista de comprobación
- [ ] ¿Algún import de `domain`/`usecase` apunta a infraestructura? → mover.
- [ ] ¿El handler contiene reglas de negocio? → llevarlas a `usecase`.
- [ ] ¿Los errores se envuelven con `%w` y se traducen en un solo lugar?
- [ ] ¿Las dependencias se inyectan por constructor?
- [ ] ¿Hay prueba unitaria del caso de uso con un doble del puerto?
