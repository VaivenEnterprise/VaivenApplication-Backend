package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/cache"
)

// Feature es la clave de esta función en el contador de uso (plan P-04).
const Feature = "assistant"

const (
	defaultTimeout  = 12 * time.Second // RF-16
	defaultCacheTTL = time.Hour        // TTL corto (Art. XIV.2)
	cachePrefix     = "ai:plan:"       // el adaptador antepone vaiven:<entorno>:
	dayLayout       = "2006-01-02"
	mexicoCity      = "America/Mexico_City"
	refusalSummary  = "No puedo ayudarte con eso. Cuéntame un proyecto práctico (hogar, jardín, herramientas, mudanza, deportes o eventos) y te digo qué necesitas y quién lo presta cerca."
)

// Errores de la parte B. ErrDailyLimit no es un código de dominio (Art. IV):
// la capa HTTP lo traduce a 429 rate_limited.
var (
	ErrDisabled   = shared.NewUnavailable("El asistente no está disponible por ahora.")
	ErrNoReply    = shared.NewUnavailable("El asistente no pudo responder. Intenta de nuevo en un momento.")
	ErrDailyLimit = errors.New("assistant: límite diario de mensajes alcanzado")
)

// Planner genera la respuesta del modelo (lo implementa el adaptador Gemini).
type Planner interface {
	Generate(ctx context.Context, req aidomain.Request) ([]byte, error)
}

// Conversations guarda las conversaciones (MongoDB).
type Conversations interface {
	Create(ctx context.Context, userID string, turns []aidomain.Turn, now time.Time) (string, error)
	Get(ctx context.Context, id, userID string) (domain.Conversation, error)
	Append(ctx context.Context, id, userID string, turns []aidomain.Turn, now time.Time) error
	Delete(ctx context.Context, id, userID string) error
}

// Usage es el contador de uso diario (MongoDB).
type Usage interface {
	Take(ctx context.Context, userID, feature, day string, limit int, now time.Time) (bool, error)
}

// Config son los parámetros de la función.
type Config struct {
	Enabled    bool           // ASSISTANT_ENABLED
	DailyLimit int            // ASSISTANT_DAILY_LIMIT
	Timeout    time.Duration  // 0 = 12 s
	CacheTTL   time.Duration  // 0 = 1 h
	Location   *time.Location // nil = America/Mexico_City
}

// Deps son las dependencias. Cache es opcional: sin ella se llama siempre al
// modelo (Redis nunca es fuente de verdad, Art. XIV.2).
type Deps struct {
	AI            Planner
	Finder        Finder
	Conversations Conversations
	Usage         Usage
	Moderation    moderation.Checker
	Cache         cache.Cache
	Now           func() time.Time
}

// SendInput es un mensaje de la persona.
type SendInput struct {
	ConversationID string
	Message        string
	Location       *GeoPoint // solo para buscar; nunca va al modelo ni se guarda
	Municipality   string
}

// Reply es la respuesta del asistente: el plan del modelo ya validado y, por
// cada necesidad, quién la tiene (de la búsqueda, nunca del modelo).
type Reply struct {
	ConversationID string
	Summary        string
	Needs          []NeedResult
	Steps          []string
	SafetyNotes    []string
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
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
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

// Enabled indica si la función está encendida (GET /meta, RF-16).
func (s *Service) Enabled() bool { return s.cfg.Enabled }

// Send procesa un mensaje (RF-09 a RF-18).
func (s *Service) Send(ctx context.Context, userID string, in SendInput) (Reply, error) {
	if !s.cfg.Enabled {
		return Reply{}, ErrDisabled
	}
	if !domain.ValidMessage(in.Message) {
		return Reply{}, shared.NewValidation("Revisa tu mensaje.",
			shared.Detail{Field: "message", Issue: "escribe de 2 a 500 caracteres"})
	}
	msg := domain.Sanitize(in.Message)
	if ok, reason, err := s.allowed(ctx, msg); err != nil {
		return Reply{}, err
	} else if !ok {
		return Reply{}, shared.NewValidation("Tu mensaje no se puede enviar.", shared.Detail{Field: "message", Issue: reason})
	}

	var history []aidomain.Turn
	if in.ConversationID != "" {
		conv, err := s.d.Conversations.Get(ctx, in.ConversationID, userID)
		if err != nil {
			return Reply{}, err
		}
		history = conv.Turns
	}

	now := s.d.Now()
	ok, err := s.d.Usage.Take(ctx, userID, Feature, now.In(s.cfg.Location).Format(dayLayout), s.cfg.DailyLimit, now)
	if err != nil {
		return Reply{}, fmt.Errorf("assistant: contador de uso: %w", err)
	}
	if !ok {
		return Reply{}, ErrDailyLimit
	}

	plan, err := s.plan(ctx, history, msg, in.ConversationID == "")
	if err != nil {
		return Reply{}, err
	}
	if plan, err = s.moderatePlan(ctx, plan); err != nil {
		return Reply{}, err
	}

	results, err := s.d.Finder.Find(ctx, FindRequest{
		ViewerID: userID, Needs: plan.Needs, Near: in.Location, Municipality: in.Municipality,
	})
	if err != nil {
		return Reply{}, ErrNoReply.Wrap(err)
	}

	turns := []aidomain.Turn{domain.UserTurn(msg), domain.ModelTurn(plan)}
	convID := in.ConversationID
	if convID == "" {
		if convID, err = s.d.Conversations.Create(ctx, userID, turns, now); err != nil {
			return Reply{}, fmt.Errorf("assistant: guardar conversación: %w", err)
		}
	} else if err := s.d.Conversations.Append(ctx, convID, userID, turns, now); err != nil {
		return Reply{}, err
	}

	return Reply{
		ConversationID: convID,
		Summary:        withMissing(plan.Summary, results),
		Needs:          results,
		Steps:          plan.Steps,
		SafetyNotes:    plan.SafetyNotes,
	}, nil
}

// Delete borra una conversación propia (RF-13).
func (s *Service) Delete(ctx context.Context, userID, conversationID string) error {
	return s.d.Conversations.Delete(ctx, conversationID, userID)
}

// plan obtiene la respuesta del modelo. El primer mensaje de una conversación
// se busca y se guarda en la caché (RF-15, CA-14); los siguientes no, porque
// dependen del historial.
func (s *Service) plan(ctx context.Context, history []aidomain.Turn, msg string, first bool) (aidomain.Plan, error) {
	key := cachePrefix + domain.CacheKey(msg)
	if first && s.d.Cache != nil {
		if raw, hit, err := s.d.Cache.Get(ctx, key); err == nil && hit {
			if p, err := aidomain.ParsePlan(raw); err == nil {
				return p, nil
			}
		}
	}
	cctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	// La ubicación no forma parte de la petición: BuildRequest no la recibe (CA-11).
	raw, err := s.d.AI.Generate(cctx, domain.BuildRequest(history, msg))
	if errors.Is(err, aidomain.ErrBlocked) {
		// El proveedor bloqueó el tema: negativa amable, sin cupo extra (RF-14).
		return aidomain.Plan{Summary: refusalSummary, Needs: []aidomain.Need{}, Steps: []string{}, SafetyNotes: []string{}}, nil
	}
	if err != nil {
		return aidomain.Plan{}, ErrNoReply.Wrap(err)
	}
	plan, err := aidomain.ParsePlan(raw)
	if err != nil {
		return aidomain.Plan{}, ErrNoReply.Wrap(err)
	}
	if first && s.d.Cache != nil {
		_ = s.d.Cache.Set(ctx, key, raw, s.cfg.CacheTTL) // si Redis falla, solo se pierde la caché
	}
	return plan, nil
}

// moderatePlan pasa todo el texto del modelo por el filtro (RF-17): si el
// resumen no se permite no hay respuesta; lo demás no permitido se quita.
func (s *Service) moderatePlan(ctx context.Context, p aidomain.Plan) (aidomain.Plan, error) {
	ok, _, err := s.allowed(ctx, p.Summary)
	if err != nil {
		return aidomain.Plan{}, err
	}
	if !ok {
		return aidomain.Plan{}, ErrNoReply
	}
	out := aidomain.Plan{Summary: p.Summary, Needs: []aidomain.Need{}}
	for _, n := range p.Needs {
		if ok, _, err := s.allowed(ctx, n.Name+" "+strings.Join(n.SearchTerms, " ")); err != nil {
			return aidomain.Plan{}, err
		} else if ok {
			out.Needs = append(out.Needs, n)
		}
	}
	if out.Steps, err = s.keepAllowed(ctx, p.Steps); err != nil {
		return aidomain.Plan{}, err
	}
	if out.SafetyNotes, err = s.keepAllowed(ctx, p.SafetyNotes); err != nil {
		return aidomain.Plan{}, err
	}
	return out, nil
}

func (s *Service) keepAllowed(ctx context.Context, texts []string) ([]string, error) {
	out := []string{}
	for _, t := range texts {
		ok, _, err := s.allowed(ctx, t)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Service) allowed(ctx context.Context, text string) (bool, string, error) {
	if text == "" {
		return true, "", nil
	}
	res, err := s.d.Moderation.Check(ctx, text)
	if err != nil {
		return false, "", fmt.Errorf("assistant: filtro de contenido: %w", err)
	}
	return res.Allowed, res.Reason, nil
}

// withMissing agrega al resumen qué necesidades no tienen coincidencias cerca
// (CA-08). Lo escribe el backend, no el modelo, y respeta el máximo de 600.
func withMissing(summary string, results []NeedResult) string {
	var missing []string
	for _, r := range results {
		if len(r.Matches) == 0 {
			missing = append(missing, strings.ToLower(r.Need.Name))
		}
	}
	if len(missing) == 0 {
		return summary
	}
	note := "Por ahora nadie presta cerca de ti: " + strings.Join(missing, ", ") + "."
	out := strings.TrimSpace(summary + " " + note)
	if utf8.RuneCountInString(out) > aidomain.MaxSummaryRunes {
		return summary
	}
	return out
}
