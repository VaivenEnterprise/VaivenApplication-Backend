// Package clock abstrae la hora actual para que los casos de uso y adaptadores
// puedan probarse con un reloj fijo.
package clock

import "time"

// Clock devuelve la hora actual.
type Clock interface {
	Now() time.Time
}

// System es el reloj real del sistema, en UTC.
type System struct{}

// Now devuelve la hora actual en UTC.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed es un reloj de pruebas que siempre devuelve T.
type Fixed struct{ T time.Time }

// Now devuelve T.
func (f Fixed) Now() time.Time { return f.T }
