package domain

import "testing"

func TestPopularKeys(t *testing.T) {
	if PopularMunicipality("  San   PEDRO ") != "m:san pedro" || PopularCategory("hogar") != "c:hogar" || PopularAll != "all" {
		t.Error("claves de populares")
	}
}
