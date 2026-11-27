// Package usecase contiene los casos de uso de la cuenta (spec 003): registro,
// inicio y cierre de sesión, renovación, perfil y baja. Declara los puertos que
// necesita (Art. III.3); los adaptadores de MongoDB y security los implementan.
package usecase

import (
	"context"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Users guarda y lee cuentas.
type Users interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
	ByEmail(ctx context.Context, email string) (domain.User, error)
	ByID(ctx context.Context, id string) (domain.User, error)
	// UpdateProfile aplica los cambios no nulos y devuelve la cuenta nueva.
	UpdateProfile(ctx context.Context, id string, p ProfileChanges, now time.Time) (domain.User, error)
	// LockActive es el cerrojo de la baja (spec 021, plan §4): marca la cuenta
	// activa como en uso dentro de la transacción y devuelve true; false si la
	// cuenta ya no está activa (la baja ya ocurrió) o no existe. Dos bajas
	// simultáneas chocan aquí y solo una sigue.
	LockActive(ctx context.Context, id string) (bool, error)
	// Deactivate da de baja la cuenta activa: deja solo los campos que conserva
	// la spec 021 (§5.2) y marca deletionPending. Si la cuenta no existe o ya no
	// está activa devuelve domain.ErrUserNotFound.
	Deactivate(ctx context.Context, id, anonymizedEmail string, now time.Time) error
	// UpdatePasswordHash reemplaza el hash de una cuenta activa (migración de
	// parámetros de argon2id).
	UpdatePasswordHash(ctx context.Context, id, passwordHash string, now time.Time) error
}

// ProfileChanges son los cambios de PATCH /me. Un campo nil no cambia;
// ClearPhone borra el teléfono.
type ProfileChanges struct {
	DisplayName *string
	Phone       *string
	ClearPhone  bool
}

// RefreshTokens guarda los refresh tokens (solo su hash).
type RefreshTokens interface {
	Create(ctx context.Context, t domain.RefreshToken) error
	ByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	// MarkUsed marca el token como usado de forma atómica; false si ya estaba
	// usado o revocado.
	MarkUsed(ctx context.Context, id string, at time.Time) (bool, error)
	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	// DeleteAllForUser borra todas las sesiones de la persona (baja, spec 021).
	DeleteAllForUser(ctx context.Context, userID string) error
}

// PasswordHasher calcula y verifica hashes de contraseña (argon2id).
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Verify devuelve false, nil si la contraseña no coincide.
	Verify(ctx context.Context, password, encoded string) (bool, error)
	// DecoyHash es un hash válido de un valor desconocido, con los parámetros
	// actuales, calculado al crear el hasher (no por petición). Login verifica
	// contra él cuando el correo no existe.
	DecoyHash() string
	// NeedsRehash indica si un hash guardado usa parámetros anteriores.
	NeedsRehash(encoded string) bool
}

// AccessTokens emite tokens de acceso (JWT).
type AccessTokens interface {
	// IssueSession firma un token de acceso ligado a la sesión (familia de
	// refresh) para poder revocarlo al cerrarla (spec 018, C-12).
	IssueSession(userID, role, sessionID string) (string, error)
	TTL() time.Duration
}

// Revocations revoca tokens de acceso antes de que venzan (spec 018, C-12).
// La implementación es Redis (internal/platform/redis); nil no revoca nada y
// el token sigue valiendo hasta vencer, como antes.
type Revocations interface {
	RevokeSession(ctx context.Context, sessionID string, ttl time.Duration) error
	RevokeUserBefore(ctx context.Context, userID string, cutoff time.Time, ttl time.Duration) error
}

// SecurityEvent es un evento de seguridad (spec 018, C-02). Subject es un
// seudónimo («user:<id>» o «unknown»): nunca correo, contraseña ni token.
type SecurityEvent struct {
	Name     string
	Result   string
	Severity string
	Subject  string
}

// SecurityEvents registra eventos de seguridad; nil no registra nada.
type SecurityEvents interface {
	Record(ctx context.Context, e SecurityEvent)
}

// RefreshGenerator genera un refresh token opaco y su hash.
type RefreshGenerator func() (token, hash string, err error)

// RefreshHasher calcula el hash con el que se guarda un refresh token.
type RefreshHasher func(token string) string

// ActiveBookingChecker indica si la persona tiene reservas activas (plan P-10).
// Hasta la spec 007 se usa NoActiveBookings.
type ActiveBookingChecker interface {
	HasActiveBookings(ctx context.Context, userID string) (bool, error)
}

// NoActiveBookings es la implementación vacía: nunca hay reservas activas.
type NoActiveBookings struct{}

// HasActiveBookings siempre devuelve false.
func (NoActiveBookings) HasActiveBookings(context.Context, string) (bool, error) {
	return false, nil
}

// TxRunner ejecuta fn en una sola transacción de la base de datos (spec 021,
// RF-02). fn puede ejecutarse más de una vez (errores transitorios), así que no
// debe dejar estado fuera de la transacción. Un error de fn la aborta y se
// devuelve sin cambios; el ctx que recibe fn es el de la transacción y es el que
// deben usar los repositorios.
type TxRunner interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// Tipos del contrato de la baja compartido con los módulos (spec 021). Viven en
// el paquete neutral internal/usecase/erasure para que ningún adaptador importe
// el usecase de auth (Art. III y XII); aquí son alias para este caso de uso.
type (
	// ImageRef identifica una imagen que hay que borrar en el proveedor.
	ImageRef = erasure.ImageRef
	// Notice es un aviso para una contraparte de la baja.
	Notice = erasure.Notice
	// EraseResult es lo que un Eraser deja para después del commit.
	EraseResult = erasure.EraseResult
	// Eraser lo implementa cada módulo sobre sus colecciones.
	Eraser = erasure.Eraser
)

// Job es un paso pendiente posterior al commit (bandeja de salida).
type Job struct {
	ID            string
	UserID        string
	Kind          domain.JobKind
	Image         *ImageRef // si Kind es JobImage
	Notice        *Notice   // si Kind es JobNotice
	Attempts      int
	NextAttemptAt time.Time
	// Retention marca un job programado a futuro por la retención de 12 meses
	// (D-4, p. ej. las evidencias de una incidencia, que se borran al purgarla):
	// no es un paso pendiente de la baja, así que no mantiene deletionPending.
	Retention bool
}

// Jobs es la bandeja de salida (outbox) de los pasos posteriores al commit.
type Jobs interface {
	// Enqueue guarda los jobs; se llama dentro de la transacción de la baja.
	Enqueue(ctx context.Context, jobs []Job, now time.Time) error
	// Claim reserva (con un arrendamiento) hasta limit jobs vencidos; dos
	// reclamos simultáneos no devuelven el mismo job.
	Claim(ctx context.Context, now time.Time, limit int) ([]Job, error)
	// Done borra el job terminado.
	Done(ctx context.Context, id string) error
	// Fail registra un intento fallido (ver JobFailure).
	Fail(ctx context.Context, id string, f JobFailure) error
	// Open cuenta los jobs aún por hacer de una cuenta. Los fallidos (agotaron
	// sus intentos) quedan en la bandeja para operación y los de retención aún no
	// vencidos (Job.Retention) esperan su fecha; ni unos ni otros cuentan.
	Open(ctx context.Context, userID string) (int64, error)
}

// JobFailure describe un intento fallido de un job.
type JobFailure struct {
	// At es el momento del intento.
	At time.Time
	// Next es cuándo reintentar (se ignora si Exhausted).
	Next time.Time
	// Exhausted marca el job como fallido: no se reintenta más y queda para
	// operación.
	Exhausted bool
	// Reason es un texto corto SIN datos personales (p. ej. «cloudinary 500»).
	Reason string
}

// PendingDeletions lee y limpia la marca deletionPending de las cuentas.
type PendingDeletions interface {
	// ListPending devuelve hasta limit cuentas con pasos pendientes.
	ListPending(ctx context.Context, limit int) ([]string, error)
	// ClearIfDone quita la marca si la cuenta aún la tiene y devuelve true si
	// la quitó. Quien llama ya comprobó que no quedan pasos (nunca escribe null).
	ClearIfDone(ctx context.Context, userID string, now time.Time) (bool, error)
}

// ImageDestroyer borra una imagen en el proveedor de imágenes (Cloudinary). Una
// imagen que ya no existe cuenta como borrada (idempotente).
type ImageDestroyer interface {
	Destroy(ctx context.Context, ref ImageRef) error
}

// NoticeSender envía el aviso de un job a su destinatario (notify.Notifier).
type NoticeSender interface {
	Send(ctx context.Context, n Notice) error
}
