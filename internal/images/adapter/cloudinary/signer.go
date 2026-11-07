// Package cloudinary es el adaptador de Cloudinary de VAIVÉN, escrito sin SDK
// con la biblioteca estándar (plan P-01): firma de subidas, URLs de entrega y
// consultas de administración. El API secret nunca sale del servidor: no se
// devuelve al cliente, no se registra y no aparece en las URLs firmadas.
package cloudinary

import (
	"crypto/sha1" //nolint:gosec // Cloudinary exige SHA-1 para sus firmas (plan §3).
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// errMissingConfig indica que falta un dato de configuración de Cloudinary.
var errMissingConfig = errors.New("falta configuración")

// Sign calcula la firma de Cloudinary (plan §3): ordena las claves
// alfabéticamente, omite las de valor vacío, une «k=v» con «&», concatena el
// secreto y devuelve el SHA-1 en hexadecimal.
func Sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+params[k])
	}
	sum := sha1.Sum([]byte(strings.Join(pairs, "&") + secret)) //nolint:gosec // ver import.
	return hex.EncodeToString(sum[:])
}

// Signer firma subidas directas a Cloudinary con el secreto de la cuenta.
type Signer struct {
	cloudName string
	apiKey    string
	secret    string
	baseURL   string
}

// NewSigner crea un Signer. Devuelve error si falta cloudName, apiKey o secret;
// el error nunca incluye el secreto.
func NewSigner(cloudName, apiKey, secret string) (*Signer, error) {
	if err := checkConfig(cloudName, apiKey, secret); err != nil {
		return nil, err
	}
	return &Signer{
		cloudName: cloudName,
		apiKey:    apiKey,
		secret:    secret,
		baseURL:   "https://api.cloudinary.com/v1_1/" + cloudName,
	}, nil
}

// checkConfig valida los datos de la cuenta sin mencionar sus valores.
func checkConfig(cloudName, apiKey, secret string) error {
	var missing []string
	if cloudName == "" {
		missing = append(missing, "cloud name")
	}
	if apiKey == "" {
		missing = append(missing, "api key")
	}
	if secret == "" {
		missing = append(missing, "api secret")
	}
	if len(missing) > 0 {
		return fmt.Errorf("cloudinary: %s: %w", strings.Join(missing, ", "), errMissingConfig)
	}
	return nil
}

// UploadURL devuelve el endpoint de subida de imágenes de la cuenta.
func (s *Signer) UploadURL() string { return s.baseURL + "/image/upload" }

// APIKey devuelve la API key pública de la cuenta (no es secreta).
func (s *Signer) APIKey() string { return s.apiKey }

// SignUpload firma los parámetros de una subida con el secreto de la cuenta.
func (s *Signer) SignUpload(params map[string]string) string {
	return Sign(params, s.secret)
}

// String oculta el secreto al imprimir el Signer.
func (s *Signer) String() string {
	return fmt.Sprintf("cloudinary.Signer{cloud: %q, apiKey: %q, secret: [REDACTED]}", s.cloudName, s.apiKey)
}

// GoString oculta el secreto al imprimir el Signer con %#v.
func (s *Signer) GoString() string { return s.String() }
