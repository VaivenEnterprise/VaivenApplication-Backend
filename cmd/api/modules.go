package main

import (
	"log/slog"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// moduleDeps son las dependencias compartidas por los módulos. Los puertos
// compartidos (Art. XII.4) empiezan con su implementación vacía.
type moduleDeps struct {
	Log        *slog.Logger
	Notifier   notify.Notifier
	Moderation moderation.Checker
}

// modules devuelve los módulos de negocio que se montan bajo /api/v1 (spec 003,
// RF-11; Art. XII.3). Cada spec agrega su módulo con UNA línea en la lista y no
// edita las de los demás. deps reúne lo que los módulos comparten.
func modules(_ moduleDeps) []apihttp.Module {
	return []apihttp.Module{
		// Una línea por módulo, en orden de spec:
	}
}
