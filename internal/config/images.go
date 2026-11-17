package config

// Images son los límites de imágenes (spec 004, plan §5; POL-13).
type Images struct {
	// MaxBytes es el tamaño máximo por imagen (5 MB por defecto).
	MaxBytes int64
	// SignaturesPerHour es el límite de firmas por persona y por hora (RF-04).
	SignaturesPerHour int
}

// loadImages lee IMAGE_MAX_BYTES e IMAGE_SIGNATURE_PER_HOUR.
func loadImages(r *reader) Images {
	return Images{
		MaxBytes:          int64(r.positiveInt("IMAGE_MAX_BYTES", 5*1024*1024)),
		SignaturesPerHour: r.positiveInt("IMAGE_SIGNATURE_PER_HOUR", 30),
	}
}
