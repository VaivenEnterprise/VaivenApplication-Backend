// Package policy reúne como constantes con nombre las políticas del servicio
// de ROADMAP §8 que usa el backend (spec 005, T002; plan P-01; Art. XII). Un
// cambio de política es un cambio en este único archivo. Solo depende de la
// biblioteca estándar, así que cualquier capa (incluido domain) la puede usar.
//
// Montos en centavos de MXN (Art. V).
package policy

// POL-04 — Precio sugerido: 3 % del valor declarado por día, mínimo $10,
// redondeado hacia arriba a múltiplos de $5. Valor declarado entre $50 y
// $100,000.
const (
	// POL04SuggestedPercent es el porcentaje diario del valor declarado.
	POL04SuggestedPercent = 3
	// POL04MinSuggestedCents es la sugerencia mínima ($10).
	POL04MinSuggestedCents int64 = 1_000
	// POL04RoundStepCents es el múltiplo al que se redondea hacia arriba ($5).
	POL04RoundStepCents int64 = 500
	// POL04MinDeclaredValueCents es el valor declarado mínimo ($50).
	POL04MinDeclaredValueCents int64 = 5_000
	// POL04MaxDeclaredValueCents es el valor declarado máximo ($100,000).
	POL04MaxDeclaredValueCents int64 = 10_000_000
)

// POL-01 — Cobro por día; el préstamo cuesta $0. La renta cuesta al menos $10
// por día (spec 005, RF-02).
const (
	// POL01LoanPriceCents es la tarifa diaria de un préstamo.
	POL01LoanPriceCents int64 = 0
	// POL01MinRentPriceCents es la tarifa diaria mínima de una renta ($10).
	POL01MinRentPriceCents int64 = 1_000
)

// POL-02 — Duración de un préstamo o renta: de 1 a 7 días de calendario
// (fechas inclusivas). La búsqueda la usa para acotar el periodo (spec 006).
const (
	POL02MinDays = 1
	POL02MaxDays = 7
)

// POL03RequestExpiryHours es el vencimiento de una solicitud (POL-03): 48 h o
// al llegar la fecha de inicio, lo primero (spec 007).
const POL03RequestExpiryHours = 48

// POL-05 — Modalidades de pago de una renta: todo en la entrega o 50/50
// (entrega = ceil(total/2), devolución = floor(total/2)). El cálculo vive en
// bookings/domain.SplitPayments.
const (
	POL05FullOnDelivery = "full_on_delivery"
	POL05Split5050      = "split_50_50"
)

// POL-09 — Cancelación tardía y ausencia: sin monto automático; se informa
// antes de solicitar (spec 007, RF-01).
const (
	POL09LateCancellationNotice = "Cancelar tarde no tiene cargo automático, pero puede registrarse una incidencia."
	POL09NoShowNotice           = "Si no te presentas a la entrega o la devolución puede registrarse una incidencia y una advertencia o suspensión."
)

// POL-11 — Chat: texto de hasta 1000 caracteres y 30 mensajes por minuto por
// persona; sin borrado en esta versión (spec 008).
const (
	POL11MaxMessageChars   = 1000
	POL11MessagesPerMinute = 30
)

// POL-13 — Fotos: de 1 a 5 por objeto.
const (
	POL13MinItemPhotos = 1
	POL13MaxItemPhotos = 5
)

// Category es una categoría de POL-14: Key se guarda y viaja en la API; Label
// se muestra (es-MX).
type Category struct {
	Key   string
	Label string
}

// POL-14 — Claves de las categorías.
const (
	POL14HerramientasManuales   = "herramientas_manuales"
	POL14HerramientasElectricas = "herramientas_electricas"
	POL14ConstruccionJardineria = "construccion_jardineria"
	POL14Movilidad              = "movilidad"
	POL14Deportes               = "deportes"
	POL14Hogar                  = "hogar"
	POL14Otros                  = "otros"
)

// POL14Categories devuelve las categorías en el orden en que se muestran. Es
// una función (no una variable) para que nadie las cambie en tiempo de
// ejecución (constitución §5: sin globales mutables).
func POL14Categories() []Category {
	return []Category{
		{POL14HerramientasManuales, "Herramientas manuales"},
		{POL14HerramientasElectricas, "Herramientas eléctricas"},
		{POL14ConstruccionJardineria, "Construcción y jardinería"},
		{POL14Movilidad, "Patines, patinetas, bicicletas y similares"},
		{POL14Deportes, "Deportes y aire libre"},
		{POL14Hogar, "Hogar"},
		{POL14Otros, "Otros"},
	}
}

// POL14Valid indica si key es una categoría de POL-14.
func POL14Valid(key string) bool {
	for _, c := range POL14Categories() {
		if c.Key == key {
			return true
		}
	}
	return false
}
