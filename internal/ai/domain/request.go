package domain

import (
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Roles de un turno de conversación.
const (
	RoleUser  = "user"
	RoleModel = "model"
)

// Errores del proveedor que los casos de uso traducen a service_unavailable.
var (
	// ErrUnavailable: el proveedor no respondió a tiempo, falló o no devolvió
	// contenido (spec 014, RF-06 y RF-16).
	ErrUnavailable = errors.New("ai: proveedor no disponible")
	// ErrBlocked: el proveedor bloqueó la petición o la respuesta por sus
	// filtros de seguridad.
	ErrBlocked = errors.New("ai: el proveedor bloqueó el contenido")
)

// Turn es un mensaje previo de la conversación (RF-13).
type Turn struct {
	Role string // RoleUser o RoleModel
	Text string
}

// Request es lo que un caso de uso pide al proveedor. Es genérica a propósito:
// cada caso de uso declara su propio puerto con el método Generate (Art. III.3)
// y el adaptador del proveedor lo implementa una sola vez (plan P-01).
type Request struct {
	// System son las instrucciones fijas del caso de uso (nunca texto de la
	// persona; plan P-08).
	System string
	// Turns son los mensajes, del más antiguo al más reciente; el último es el
	// de la persona. El texto de la persona viaja siempre como dato.
	Turns []Turn
	// ImageURL es una miniatura construida por el servidor (parte A); vacía en
	// la parte B. El adaptador la descarga y la envía al proveedor.
	ImageURL string
	// Schema es la forma JSON que debe tener la respuesta.
	Schema *Schema
}

// Schema describe la salida JSON esperada (subconjunto de OpenAPI que aceptan
// los proveedores con salida estructurada).
type Schema struct {
	Type        string             `json:"type"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
}

// Tipos del esquema.
const (
	TypeObject  = "OBJECT"
	TypeArray   = "ARRAY"
	TypeString  = "STRING"
	TypeBoolean = "BOOLEAN"
)

func str(desc string) *Schema { return &Schema{Type: TypeString, Description: desc} }

func strList(desc string) *Schema {
	return &Schema{Type: TypeArray, Description: desc, Items: &Schema{Type: TypeString}}
}

// categoryEnum son las claves de POL-14, tomadas del código (nunca a mano).
func categoryEnum() []string {
	cats := policy.POL14Categories()
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = c.Key
	}
	return out
}

// SuggestionSchema es la salida de la parte A.
func SuggestionSchema() *Schema {
	return &Schema{
		Type: TypeObject,
		Properties: map[string]*Schema{
			"title":       str("Título corto en español de México, hasta 80 caracteres."),
			"category":    {Type: TypeString, Enum: categoryEnum()},
			"description": str("Descripción breve en español de México, hasta 1000 caracteres."),
		},
		Required: []string{"title", "category"},
	}
}

// PlanSchema es la salida de la parte B. No tiene ningún campo para objetos,
// personas ni ubicaciones: eso lo decide la búsqueda (RF-11).
func PlanSchema() *Schema {
	return &Schema{
		Type: TypeObject,
		Properties: map[string]*Schema{
			"summary": str("Resumen en español de México, hasta 600 caracteres."),
			"needs": {
				Type:        TypeArray,
				Description: "Objetos que se necesitan, como máximo 8.",
				Items: &Schema{
					Type: TypeObject,
					Properties: map[string]*Schema{
						"name":        str("Nombre del objeto, hasta 60 caracteres."),
						"category":    {Type: TypeString, Enum: categoryEnum()},
						"searchTerms": strList("De 1 a 3 palabras para buscarlo."),
						"optional":    {Type: TypeBoolean},
					},
					Required: []string{"name", "category", "searchTerms", "optional"},
				},
			},
			"steps":       strList("Pasos en orden, como máximo 10, cada uno hasta 200 caracteres."),
			"safetyNotes": strList("Avisos de seguridad, como máximo 3."),
		},
		Required: []string{"summary", "needs", "steps", "safetyNotes"},
	}
}
