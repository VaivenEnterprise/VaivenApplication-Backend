// Package usecase sugiere título y categoría a partir de una foto propia
// (spec 014, parte A; RF-01 a RF-07). La IA solo propone: quien publica
// acepta, edita o ignora la sugerencia.
//
// Puertos (Art. III.3): los declara este paquete y los implementan los
// adaptadores (Gemini, images.Service, el contador de MongoDB).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
)

// Feature es la clave de esta función en el contador de uso (plan P-04).
const Feature = "suggest"

const (
	defaultTimeout = 25 * time.Second // RF-06 (cambio 2026-11-26, T018): configurable con SUGGEST_TIMEOUT
	maxPublicIDLen = 255
	dayLayout      = "2006-01-02"
	mexicoCity     = "America/Mexico_City"
)

// Instrucciones fijas (plan P-08). No llevan nada que escriba la persona.
const systemPrompt = `Eres el asistente de VAIVÉN, una app mexicana de préstamo y renta de objetos entre personas.
Recibes la foto de un objeto que alguien quiere publicar. Devuelve SOLO el JSON pedido:
- title: título corto y claro en español de México (máximo 80 caracteres), sin precios, marcas inventadas, datos de contacto ni datos personales.
- category: una de las claves permitidas.
- description: una o dos frases neutrales en español de México sobre el objeto.
Si la foto no muestra un objeto prestable, usa la categoría "otros" y un título genérico.`

const userInstruction = "Sugiere el título, la categoría y una descripción breve para publicar este objeto."

// Errores de la parte A. ErrDailyLimit no es un código de dominio (Art. IV):
// la capa HTTP lo traduce a 429 rate_limited.
var (
	ErrDisabled   = shared.NewUnavailable("Las sugerencias con IA no están disponibles por ahora.")
	ErrNoImage    = shared.NewNotFound("No encontramos esa foto.")
	ErrNoProposal = shared.NewUnavailable("No pudimos sugerir algo para esta foto. Escribe el título tú mismo.")
	ErrDailyLimit = errors.New("suggest: límite diario de sugerencias alcanzado")
)

// Describer genera la respuesta del modelo (lo implementa el adaptador Gemini).
type Describer interface {
	Generate(ctx context.Context, req aidomain.Request) ([]byte, error)
}

// Images es la parte de images.Service que se usa aquí.
type Images interface {
	Resolve(ctx context.Context, userID string, purpose imagesdomain.Purpose, field string, refs []imagesdomain.Ref) ([]imagesdomain.Image, error)
	URLs(img imagesdomain.Image) (imagesdomain.URLs, error)
}

// Usage es el contador de uso diario (MongoDB).
type Usage interface {
	Take(ctx context.Context, userID, feature, day string, limit int, now time.Time) (bool, error)
}

// Config son los parámetros de la función.
type Config struct {
	Enabled    bool           // AI_ENABLED
	DailyLimit int            // AI_DAILY_LIMIT
	BaseFolder string         // CLOUDINARY_BASE_FOLDER (vaiven/<entorno>)
	Timeout    time.Duration  // SUGGEST_TIMEOUT; 0 = 25 s
	Location   *time.Location // nil = America/Mexico_City
}

// Deps son las dependencias.
type Deps struct {
	AI         Describer
	Images     Images
	Usage      Usage
	Moderation moderation.Checker
	Now        func() time.Time // nil = time.Now
}

// Service es el caso de uso.
type Service struct {
	cfg Config
	d   Deps
}

// New crea el caso de uso.
func New(cfg Config, d Deps) *Service {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Location == nil {
		if loc, err := time.LoadLocation(mexicoCity); err == nil {
			cfg.Location = loc
		} else {
			cfg.Location = time.UTC
		}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Moderation == nil {
		d.Moderation = moderation.AllowAll{}
	}
	return &Service{cfg: cfg, d: d}
}

// Enabled indica si la función está encendida (GET /meta, RF-06).
func (s *Service) Enabled() bool { return s.cfg.Enabled }

// Suggest propone título, categoría y descripción para la foto publicID de
// userID. El orden importa: la foto se verifica antes de gastar cupo y de
// llamar al proveedor (CA-02).
func (s *Service) Suggest(ctx context.Context, userID, publicID string) (aidomain.Suggestion, error) {
	if !s.cfg.Enabled {
		return aidomain.Suggestion{}, ErrDisabled
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" || len(publicID) > maxPublicIDLen {
		return aidomain.Suggestion{}, shared.NewValidation("Revisa la foto.",
			shared.Detail{Field: "imagePublicId", Issue: "es obligatorio (máximo 255 caracteres)"})
	}
	// Foto ajena, de evidencia o de otra carpeta: 404 sin revelar si existe.
	if !imagesdomain.OwnedBy(publicID, s.cfg.BaseFolder, imagesdomain.PurposeItemPhoto, userID) {
		return aidomain.Suggestion{}, ErrNoImage
	}
	imgs, err := s.d.Images.Resolve(ctx, userID, imagesdomain.PurposeItemPhoto, "imagePublicId",
		[]imagesdomain.Ref{{PublicID: publicID}})
	if err != nil {
		var de *shared.Error
		if errors.As(err, &de) && de.Code == shared.CodeUnavailable {
			return aidomain.Suggestion{}, de
		}
		if errors.As(err, &de) && de.Code == shared.CodeValidation {
			return aidomain.Suggestion{}, ErrNoImage
		}
		return aidomain.Suggestion{}, fmt.Errorf("suggest: verificar la foto: %w", err)
	}
	if len(imgs) != 1 {
		return aidomain.Suggestion{}, ErrNoImage
	}

	now := s.d.Now()
	ok, err := s.d.Usage.Take(ctx, userID, Feature, now.In(s.cfg.Location).Format(dayLayout), s.cfg.DailyLimit, now)
	if err != nil {
		return aidomain.Suggestion{}, fmt.Errorf("suggest: contador de uso: %w", err)
	}
	if !ok {
		return aidomain.Suggestion{}, ErrDailyLimit
	}

	urls, err := s.d.Images.URLs(imgs[0])
	if err != nil {
		return aidomain.Suggestion{}, fmt.Errorf("suggest: miniatura: %w", err)
	}
	thumb := urls.ThumbURL
	if thumb == "" {
		thumb = urls.URL
	}

	cctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	raw, err := s.d.AI.Generate(cctx, aidomain.Request{
		System:   systemPrompt,
		Turns:    []aidomain.Turn{{Role: aidomain.RoleUser, Text: userInstruction}},
		ImageURL: thumb,
		Schema:   aidomain.SuggestionSchema(),
	})
	if err != nil {
		// Caída, tiempo de espera o bloqueo del proveedor: la app sigue sin
		// sugerencia (RF-06). La causa queda para los registros internos.
		return aidomain.Suggestion{}, ErrNoProposal.Wrap(err)
	}
	sug, err := aidomain.ParseSuggestion(raw)
	if err != nil {
		return aidomain.Suggestion{}, ErrNoProposal.Wrap(aidomain.WithCause(err, aidomain.CauseMalformedJSON, 0))
	}
	return s.moderate(ctx, sug)
}

// moderate aplica el filtro de contenido (spec 011): un título no permitido
// anula la sugerencia (CA-03); una descripción no permitida solo se quita.
func (s *Service) moderate(ctx context.Context, sug aidomain.Suggestion) (aidomain.Suggestion, error) {
	res, err := s.d.Moderation.Check(ctx, sug.Title)
	if err != nil {
		return aidomain.Suggestion{}, fmt.Errorf("suggest: filtro de contenido: %w", err)
	}
	if !res.Allowed {
		return aidomain.Suggestion{}, ErrNoProposal
	}
	if sug.Description != "" {
		res, err := s.d.Moderation.Check(ctx, sug.Description)
		if err != nil {
			return aidomain.Suggestion{}, fmt.Errorf("suggest: filtro de contenido: %w", err)
		}
		if !res.Allowed {
			sug.Description = ""
		}
	}
	return sug, nil
}
