package validate

import (
	"errors"
	"reflect"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

type photo struct {
	PublicID *string `json:"publicId" validate:"required,nonempty"`
}

type sample struct {
	ItemID *string  `json:"itemId" validate:"required,nonempty"`
	Total  *int64   `json:"acceptedTotalCents" validate:"required,nonnegative"`
	Ratio  float64  `json:"ratio" validate:"nonnegative"`
	Kind   string   `json:"kind" validate:"omitempty,oneof=a b"`
	Code   string   `json:"code" validate:"omitempty,len=3"`
	Hex    string   `json:"hex" validate:"omitempty,hexadecimal"`
	Odd    string   `json:"odd" validate:"omitempty,alpha"`
	Skip   string   `json:"-" validate:"omitempty,len=2"`
	Bool   *bool    `json:"flag" validate:"omitempty,nonnegative"` // tipo no numérico: no aplica
	Photos []photo  `json:"photos" validate:"required,dive"`
	Note   *meeting `json:"meetingPoint"`
}

type meeting struct {
	Description *string `json:"description" validate:"required"`
}

func ptr[T any](v T) *T { return &v }

func details(t *testing.T, err error) []shared.Detail {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation || de.Message != "Revisa los datos." {
		t.Fatalf("error = %v", err)
	}
	return de.Details
}

func TestValid(t *testing.T) {
	v := New()
	ok := sample{ItemID: ptr("x"), Total: ptr(int64(0)), Kind: "a", Code: "abc", Hex: "0f", Odd: "abc", Bool: ptr(true),
		Photos: []photo{{PublicID: ptr("p")}}, Note: &meeting{Description: ptr("aquí")}}
	if err := v.Struct(ok, "Revisa los datos."); err != nil {
		t.Fatalf("válido: %v", err)
	}
	if err := v.Struct(&ok, "Revisa los datos."); err != nil {
		t.Fatalf("puntero válido: %v", err)
	}
}

func TestDetailsInFieldOrderWithJSONNames(t *testing.T) {
	v := New()
	bad := sample{ItemID: ptr("  "), Total: ptr(int64(-1)), Ratio: -0.5, Kind: "z", Code: "ab", Hex: "xyz", Odd: "a1", Bool: ptr(false),
		Photos: []photo{{PublicID: ptr("p")}, {}}, Note: &meeting{}}
	got := details(t, v.Struct(bad, "Revisa los datos."))
	want := []shared.Detail{
		{Field: "itemId", Issue: "es obligatorio"},
		{Field: "acceptedTotalCents", Issue: "no puede ser negativo"},
		{Field: "ratio", Issue: "no puede ser negativo"},
		{Field: "kind", Issue: "valor no permitido"},
		{Field: "code", Issue: "fuera del rango permitido"},
		{Field: "hex", Issue: "formato inválido"},
		{Field: "odd", Issue: IssueInvalid},
		{Field: "photos[1].publicId", Issue: "es obligatorio"},
		{Field: "meetingPoint.description", Issue: "es obligatorio"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detalles:\n got %+v\nwant %+v", got, want)
	}
}

func TestMissingRequired(t *testing.T) {
	got := details(t, New().Struct(sample{}, "Revisa los datos."))
	want := []shared.Detail{
		{Field: "itemId", Issue: "es obligatorio"},
		{Field: "acceptedTotalCents", Issue: "es obligatorio"},
		{Field: "photos", Issue: "es obligatorio"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("faltantes = %+v", got)
	}
}

func TestNonStructIsGenericValidation(t *testing.T) {
	var de *shared.Error
	err := New().Struct(42, "Revisa los datos.")
	if !errors.As(err, &de) || de.Code != shared.CodeValidation || len(de.Details) != 0 {
		t.Errorf("no struct = %v", err)
	}
}

func TestFieldPath(t *testing.T) {
	for in, want := range map[string]string{"r.a": "a", "r.a.b": "a.b", "r.p[0].x": "p[0].x", "solo": "solo"} {
		if got := fieldPath(in); got != want {
			t.Errorf("fieldPath(%q) = %q", in, got)
		}
	}
}
