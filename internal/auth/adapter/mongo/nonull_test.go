package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// nullPaths devuelve las rutas con un valor BSON null en cualquier nivel
// (spec 021, RF-10). Los arreglos se anotan con "[]".
func nullPaths(t *testing.T, raw bson.Raw, prefix string) []string {
	t.Helper()
	elems, err := raw.Elements()
	if err != nil {
		t.Fatalf("BSON inválido: %v", err)
	}
	var out []string
	for _, el := range elems {
		out = append(out, nullPathsOfValue(t, el.Value(), prefix+"."+el.Key())...)
	}
	return out
}

func nullPathsOfValue(t *testing.T, v bson.RawValue, path string) []string {
	t.Helper()
	switch v.Type {
	case bson.TypeNull:
		return []string{path}
	case bson.TypeEmbeddedDocument:
		return nullPaths(t, v.Document(), path)
	case bson.TypeArray:
		vals, err := v.Array().Values()
		if err != nil {
			t.Fatalf("arreglo inválido: %v", err)
		}
		var out []string
		for _, it := range vals {
			out = append(out, nullPathsOfValue(t, it, path+"[]")...)
		}
		return out
	}
	return nil
}

func TestUserDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	now := time.Now().UTC()
	d, err := toDoc(domain.User{
		Email: "a@b.mx", PasswordHash: "h", DisplayName: "Ana", Role: domain.RoleUser,
		Status: domain.StatusActive, AcceptedTermsAt: now, CreatedAt: now, UpdatedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := nullPaths(t, raw, ""); len(got) != 0 {
		t.Errorf("la cuenta nueva guarda null en %v", got)
	}
	for _, absent := range []string{"phone", "suspendedUntil", "deactivatedAt"} {
		if _, err := bson.Raw(raw).LookupErr(absent); err == nil {
			t.Errorf("el campo opcional %q debe estar ausente", absent)
		}
	}
}

func TestUserDocWithOptionalFieldsKeepsThem(t *testing.T) {
	now := time.Now().UTC()
	phone := "+525512345678"
	d, err := toDoc(domain.User{Phone: &phone, SuspendedUntil: &now, DeactivatedAt: &now, AcceptedTermsAt: now})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := bson.Marshal(d)
	for _, k := range []string{"phone", "suspendedUntil", "deactivatedAt"} {
		if _, err := bson.Raw(raw).LookupErr(k); err != nil {
			t.Errorf("falta %q: %v", k, err)
		}
	}
}

// Los lectores aceptan el documento viejo (con null) y el nuevo (sin campo).
func TestUserDocReadsOldNullAndAbsentFields(t *testing.T) {
	cases := map[string]bson.D{
		"viejo con null":  {{Key: "email", Value: "a@b.mx"}, {Key: "phone", Value: nil}, {Key: "suspendedUntil", Value: nil}, {Key: "deactivatedAt", Value: nil}},
		"nuevo sin campo": {{Key: "email", Value: "a@b.mx"}},
	}
	for name, doc := range cases {
		raw, err := bson.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var d userDoc
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		u := d.toDomain()
		if u.Phone != nil || u.SuspendedUntil != nil || u.DeactivatedAt != nil {
			t.Errorf("%s: opcionales = %v %v %v, want nil", name, u.Phone, u.SuspendedUntil, u.DeactivatedAt)
		}
	}
}

func TestRefreshDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	raw, err := bson.Marshal(refreshDoc{UserID: "u", FamilyID: "f", TokenHash: "h", ExpiresAt: time.Now(), CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if got := nullPaths(t, raw, ""); len(got) != 0 {
		t.Errorf("el refresh token nuevo guarda null en %v", got)
	}
	for _, doc := range []bson.D{
		{{Key: "usedAt", Value: nil}, {Key: "revokedAt", Value: nil}},
		{},
	} {
		raw, _ := bson.Marshal(doc)
		var d refreshDoc
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		if tok := d.toDomain(); tok.UsedAt != nil || tok.RevokedAt != nil {
			t.Errorf("opcionales = %v %v, want nil", tok.UsedAt, tok.RevokedAt)
		}
	}
}
