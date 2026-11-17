package domain

import (
	"errors"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Errores de la cuenta. Son *shared.Error con mensaje en es-MX; la capa HTTP
// los traduce con el envoltorio del Art. IV. Se comparan con Is (por código y
// mensaje), así que también se reconocen si llevan una causa con Wrap.
var (
	// ErrEmailTaken: el correo ya tiene cuenta (RF-02).
	ErrEmailTaken = shared.NewConflict("Ya existe una cuenta con ese correo.")
	// ErrInvalidCredentials: mensaje genérico de login (RF-04); no revela si
	// el correo existe.
	ErrInvalidCredentials = shared.NewUnauthorized("El correo o la contraseña no son correctos.")
	// ErrInvalidSession: token ausente, inválido, vencido o reutilizado.
	ErrInvalidSession = shared.NewUnauthorized("Tu sesión no es válida o venció. Inicia sesión de nuevo.")
	// ErrUserNotFound: no existe la cuenta.
	ErrUserNotFound = shared.NewNotFound("No encontramos a esa persona.")
	// ErrWrongPassword: la contraseña de confirmación de la baja no coincide.
	ErrWrongPassword = shared.NewForbidden("La contraseña no es correcta.")
	// ErrActiveBookings: no se puede dar de baja con reservas activas (CA-05).
	ErrActiveBookings = shared.NewConflict("Tienes reservas activas. Termínalas antes de dar de baja tu cuenta.")
	// ErrSuspended: la cuenta está suspendida y no puede crear (RF-13).
	ErrSuspended = shared.NewForbidden("Tu cuenta está suspendida por ahora y no puede realizar esta acción.")
	// ErrForbiddenRole: la ruta exige otro rol (CA-07).
	ErrForbiddenRole = shared.NewForbidden("No tienes permiso para realizar esta acción.")
)

// Is indica si err es (o envuelve) el error de dominio target, comparando
// código y mensaje.
func Is(err error, target *shared.Error) bool {
	var de *shared.Error
	return errors.As(err, &de) && de.Code == target.Code && de.Message == target.Message
}
