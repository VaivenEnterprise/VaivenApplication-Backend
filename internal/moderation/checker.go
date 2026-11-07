// Package moderation declara el puerto compartido de moderación de texto (spec
// 003, RF-12; Art. XII.4). Hasta la spec 011 se usa AllowAll, que permite todo;
// la 011 lo reemplaza sin cambiar a quien lo consume.
package moderation

import "context"

// Result es el veredicto sobre un texto. Si Allowed es false, Reason explica
// el motivo en es-MX y se puede mostrar a la persona.
type Result struct {
	Allowed bool
	Reason  string
}

// Checker revisa un texto escrito por una persona antes de publicarlo.
type Checker interface {
	Check(ctx context.Context, text string) (Result, error)
}

// AllowAll es la implementación vacía: permite cualquier texto.
type AllowAll struct{}

// Check siempre permite el texto.
func (AllowAll) Check(context.Context, string) (Result, error) {
	return Result{Allowed: true}, nil
}
