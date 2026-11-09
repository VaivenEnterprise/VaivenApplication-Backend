package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// DeletedName es el nombre público de una cuenta dada de baja (spec 003).
const DeletedName = "Cuenta eliminada"

// Record es el aviso que se guarda para una persona (RF-01).
type Record struct {
	UserID    string
	Type      string
	Title     string
	Body      string
	Data      map[string]string
	CreatedAt time.Time
}

// Store guarda avisos (lo implementa notifications/adapter/mongo).
type Store interface {
	Save(ctx context.Context, r Record) error
}

// Push es el contenido de una notificación push (RF-05, RF-06). Data lleva solo
// ids; nunca datos personales (Art. VI.5).
type Push struct {
	Type  string
	Title string
	Body  string
	Data  map[string]string
}

// Pusher encola un push para enviarlo en segundo plano. No debe bloquear: si
// no hay espacio devuelve false y el push se descarta (plan P-01).
type Pusher interface {
	Enqueue(userID string, p Push) bool
}

// ServiceDeps son las dependencias del Service. Pusher puede ser nil
// (PUSH_ENABLED=false): solo se guardan los avisos.
type ServiceDeps struct {
	Store  Store
	Users  users.Directory
	Pusher Pusher
	Log    *slog.Logger
	Now    func() time.Time // nil = time.Now
}

// Service es el Notifier real (spec 011, RF-01): guarda el aviso con el título
// y el cuerpo de la plantilla del tipo y encola el push. Cumple Notifier.
type Service struct{ d ServiceDeps }

var _ Notifier = (*Service)(nil)

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d ServiceDeps) (*Service, error) {
	if d.Store == nil || d.Users == nil || d.Log == nil {
		return nil, errors.New("notify: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

// Notify guarda un aviso para userID y encola su push. El tipo se normaliza a
// los 18 del RF-02 (Normalize); uno desconocido no se guarda y se registra, sin
// error, porque un aviso de más no debe deshacer el negocio. Si la cuenta no
// existe o fue dada de baja no se guarda ni se envía nada. El título y el cuerpo
// del evento, si vienen, se respetan; si no, se usan los de la plantilla.
func (s *Service) Notify(ctx context.Context, userID string, e Event) error {
	typ, ok := Normalize(e.Type)
	if !ok {
		s.d.Log.WarnContext(ctx, "notify: tipo de aviso desconocido, no se guarda", "type", e.Type)
		return nil
	}
	profiles, err := s.d.Users.PublicUsers(ctx, []string{userID})
	if err != nil {
		return fmt.Errorf("notify: leer cuenta: %w", err)
	}
	if p, found := profiles[userID]; !found || p.DisplayName == DeletedName {
		return nil
	}
	tpl, _ := TemplateFor(typ)
	title, body := e.Title, e.Body
	if title == "" {
		title = tpl.Title
	}
	if body == "" {
		body = tpl.Body
	}
	data := cleanData(e.Data)
	rec := Record{UserID: userID, Type: typ, Title: title, Body: body, Data: data, CreatedAt: s.d.Now().UTC().Truncate(time.Millisecond)}
	if err := s.d.Store.Save(ctx, rec); err != nil {
		return fmt.Errorf("notify: guardar aviso: %w", err)
	}
	if s.d.Pusher != nil {
		if !s.d.Pusher.Enqueue(userID, Push{Type: typ, Title: title, Body: body, Data: data}) {
			s.d.Log.WarnContext(ctx, "notify: cola de push llena, se descarta el push", "type", typ)
		}
	}
	return nil
}

// allowedData son las únicas claves de data que viajan en el aviso y en el push
// (RF-06): ids para abrir la pantalla correcta.
var allowedData = map[string]bool{"bookingId": true, "itemId": true, "conversationId": true, "incidentId": true}

// cleanData deja solo las claves permitidas con valor; nunca nil.
func cleanData(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if allowedData[k] && v != "" {
			out[k] = v
		}
	}
	return out
}
