package domain

import "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"

// Category es una categoría de POL-14 (RF-11).
type Category = policy.Category

// Categories devuelve las categorías de POL-14 en el orden en que se muestran.
func Categories() []Category { return policy.POL14Categories() }

// ValidCategory indica si key es una categoría de POL-14.
func ValidCategory(key string) bool { return policy.POL14Valid(key) }
