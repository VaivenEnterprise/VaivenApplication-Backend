package http

import (
	"errors"
	"reflect"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/validate"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Spec 016, T005: los DTO de entrada se validan en la frontera con
// validator/v10 (Art. VI.2) y el contrato de error no cambia: 400
// validation_failed con un detalle por campo y su nombre JSON.
func missingFields(t *testing.T, dst any) []string {
	t.Helper()
	var de *shared.Error
	if err := validate.New().Struct(dst, "m"); !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("Struct(%T) = %v", dst, err)
	}
	out := make([]string, 0, len(de.Details))
	for _, d := range de.Details {
		out = append(out, d.Field+": "+d.Issue)
	}
	return out
}

func TestRequestDTOValidation(t *testing.T) {
	neg := int64(-1)
	for name, tc := range map[string]struct {
		dst  any
		want []string
	}{
		"create vacío":            {&createRequest{}, []string{"itemId: es obligatorio", "startDate: es obligatorio", "endDate: es obligatorio", "paymentMode: es obligatorio", "acceptedTotalCents: es obligatorio"}},
		"create negativo y vacío": {&createRequest{ItemID: ptr(" "), StartDate: ptr("a"), EndDate: ptr("b"), PaymentMode: ptr("c"), AcceptedTotalCents: &neg}, []string{"itemId: es obligatorio", "acceptedTotalCents: no puede ser negativo"}},
		"accept sin descripción":  {&acceptRequest{MeetingPoint: &meetingPointJSON{}}, []string{"meetingPoint.description: es obligatorio"}},
		"meetingPoint vacío":      {&meetingPointJSON{}, []string{"description: es obligatorio"}},
	} {
		if got := missingFields(t, tc.dst); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: detalles = %q, quiero %q", name, got, tc.want)
		}
	}
}
