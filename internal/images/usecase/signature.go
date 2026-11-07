package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Service implementa los casos de uso de imágenes.
type Service struct {
	d Deps
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Signer == nil || d.Resources == nil || d.URLs == nil || strings.Trim(d.BaseFolder, "/") == "" {
		return nil, errors.New("images: faltan dependencias del servicio")
	}
	d.BaseFolder = strings.Trim(d.BaseFolder, "/")
	if d.MaxBytes <= 0 {
		d.MaxBytes = domain.DefaultMaxBytes
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewID == nil {
		d.NewID = randomID
	}
	return &Service{d: d}, nil
}

// Signature emite la firma para que userID suba una imagen con el propósito
// indicado (RF-01 a RF-03). La firma cubre allowed_formats, public_id,
// timestamp y type, así que el cliente no puede cambiar la carpeta, el tipo
// de entrega ni los formatos.
func (s *Service) Signature(_ context.Context, userID string, purpose domain.Purpose) (domain.UploadSignature, error) {
	if !purpose.Valid() {
		return domain.UploadSignature{}, domain.ErrInvalidPurpose
	}
	if userID == "" {
		return domain.UploadSignature{}, shared.NewUnauthorized("Inicia sesión para subir imágenes.")
	}
	id, err := s.d.NewID()
	if err != nil {
		return domain.UploadSignature{}, fmt.Errorf("images: generar id: %w", err)
	}
	sig := domain.UploadSignature{
		UploadURL:      s.d.Signer.UploadURL(),
		APIKey:         s.d.Signer.APIKey(),
		Timestamp:      s.d.Now().Unix(),
		PublicID:       domain.NewPublicID(s.d.BaseFolder, purpose, userID, id),
		Type:           purpose.Delivery().CloudinaryType(),
		AllowedFormats: domain.AllowedFormats,
	}
	sig.Signature = s.d.Signer.SignUpload(map[string]string{
		"allowed_formats": sig.AllowedFormats,
		"public_id":       sig.PublicID,
		"timestamp":       strconv.FormatInt(sig.Timestamp, 10),
		"type":            sig.Type,
	})
	return sig, nil
}

// randomID devuelve 16 bytes aleatorios en hexadecimal.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
