package respond

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func TestInvalidBody(t *testing.T) {
	var de *domain.Error
	tooBig := fmt.Errorf("decode: %w", &http.MaxBytesError{Limit: 1 << 20})
	if err := InvalidBody(tooBig); !errors.As(err, &de) || de.Message != MessageBodyTooLarge || de.Code != domain.CodeValidation {
		t.Errorf("demasiado grande = %v", err)
	}
	for _, e := range []error{errors.New("unexpected EOF"), nil} {
		if err := InvalidBody(e); !errors.As(err, &de) || de.Message != MessageInvalidJSON {
			t.Errorf("JSON inválido = %v", err)
		}
	}
}
