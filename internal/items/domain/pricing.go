package domain

import (
	"math"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// SuggestPrice devuelve el precio sugerido por día en centavos para un valor
// declarado (POL-04, plan P-02):
//
//	max($10, ceilTo(round(declarado × 3 %), $5))
//
// Usa solo enteros (Art. V). No valida el rango del valor declarado: eso lo
// hace ValidDeclaredValue.
func SuggestPrice(declaredValueCents int64) int64 {
	if declaredValueCents <= 0 {
		return policy.POL04MinSuggestedCents
	}
	// round(x × 3 / 100) con enteros: (x × 3 + 50) / 100.
	raw := (declaredValueCents*policy.POL04SuggestedPercent + 50) / 100
	step := policy.POL04RoundStepCents
	rounded := (raw + step - 1) / step * step
	return max(rounded, policy.POL04MinSuggestedCents)
}

// ValidDeclaredValue indica si el valor declarado está en el rango de POL-04
// ($50 a $100,000).
func ValidDeclaredValue(cents int64) bool {
	return cents >= policy.POL04MinDeclaredValueCents && cents <= policy.POL04MaxDeclaredValueCents
}

// RoundCoord redondea una coordenada a 2 decimales (~1 km) antes de guardarla
// (RF-04, plan P-04). La ubicación exacta nunca se almacena en este módulo.
func RoundCoord(x float64) float64 {
	r := math.Round(x*100) / 100
	if r == 0 {
		return 0 // evita -0 en el JSON
	}
	return r
}
