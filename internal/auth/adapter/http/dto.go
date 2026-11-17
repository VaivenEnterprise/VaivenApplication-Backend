package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Peticiones (api/paths/auth.yaml). La frontera valida forma y tamaño con
// validator/v10 (Art. VI.2); las reglas de negocio las vuelve a validar el
// dominio.
type registerRequest struct {
	Email       string `json:"email" validate:"required,max=254"`
	Password    string `json:"password" validate:"required,max=72"`
	DisplayName string `json:"displayName" validate:"required,max=80"`
	AcceptTerms bool   `json:"acceptTerms"`
	IsAdult     bool   `json:"isAdult"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,max=254"`
	Password string `json:"password" validate:"required,max=72"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required,max=128"`
}

type deleteMeRequest struct {
	Password string `json:"password" validate:"required,max=72"`
}

// updateMeRequest distingue "phone" ausente (no cambia) de null (lo borra).
type updateMeRequest struct {
	DisplayName *string         `json:"displayName" validate:"omitempty,max=80"`
	Phone       json.RawMessage `json:"phone"`
}

func (r updateMeRequest) changes() (usecase.ProfileChanges, error) {
	c := usecase.ProfileChanges{DisplayName: r.DisplayName}
	switch raw := bytes.TrimSpace(r.Phone); {
	case len(raw) == 0:
	case bytes.Equal(raw, []byte("null")):
		c.ClearPhone = true
	default:
		var phone string
		if err := json.Unmarshal(raw, &phone); err != nil {
			return c, shared.NewValidation(messageInvalidData, shared.Detail{Field: "phone", Issue: "debe ser texto o null"})
		}
		c.Phone = &phone
	}
	return c, nil
}

// Respuestas (api/openapi.yaml: User, PublicUser, Tokens, AuthResult).
type userResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"displayName"`
	Phone         *string   `json:"phone"`
	Role          string    `json:"role"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	CreatedAt     time.Time `json:"createdAt"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:            u.ID,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		Phone:         u.Phone,
		Role:          string(u.Role),
		RatingAverage: u.RatingAverage(),
		RatingCount:   u.RatingCount,
		CreatedAt:     u.CreatedAt.UTC(),
	}
}

type publicUserResponse struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	MemberSince   time.Time `json:"memberSince"`
}

func toPublicUserResponse(p domain.PublicUser) publicUserResponse {
	return publicUserResponse{
		ID:            p.ID,
		DisplayName:   p.DisplayName,
		RatingAverage: p.RatingAverage,
		RatingCount:   p.RatingCount,
		MemberSince:   p.MemberSince.UTC(),
	}
}

type tokensResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

func toTokensResponse(t usecase.Tokens) tokensResponse {
	return tokensResponse{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresIn:    int64(t.ExpiresIn / time.Second),
	}
}

type authResponse struct {
	User   userResponse   `json:"user"`
	Tokens tokensResponse `json:"tokens"`
}

type refreshResponse struct {
	Tokens tokensResponse `json:"tokens"`
}

// messageInvalidData es el mensaje general de validation_failed.
const messageInvalidData = "Revisa los datos marcados."

// decoder lee y valida cuerpos JSON.
type decoder struct {
	v *validator.Validate
}

func newDecoder() decoder {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string { return jsonName(f.Tag.Get("json")) })
	return decoder{v: v}
}

// decode lee un único objeto JSON de body en dst y lo valida. Los errores son
// validation_failed con detalles por campo (nombres del JSON, en es-MX).
func (d decoder) decode(body io.Reader, dst any) error {
	dec := json.NewDecoder(body)
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return shared.NewValidation("El cuerpo de la petición es demasiado grande.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return shared.NewValidation(messageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"})
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	}
	if dec.More() {
		return shared.NewValidation("El cuerpo de la petición debe ser un solo objeto JSON.")
	}
	if err := d.v.Struct(dst); err != nil {
		var verrs validator.ValidationErrors
		if !errors.As(err, &verrs) {
			return shared.NewValidation(messageInvalidData)
		}
		details := make([]shared.Detail, 0, len(verrs))
		for _, fe := range verrs {
			details = append(details, shared.Detail{Field: fe.Field(), Issue: issueFor(fe.Tag())})
		}
		return shared.NewValidation(messageInvalidData, details...)
	}
	return nil
}

func issueFor(tag string) string {
	switch tag {
	case "required":
		return "es obligatorio"
	case "max":
		return "es demasiado largo"
	default:
		return "valor inválido"
	}
}

// jsonName devuelve el nombre del campo en el JSON a partir de su etiqueta.
func jsonName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	return name
}
