package erasure

import (
	"testing"
	"time"
)

func TestPurgeAtIsTwelveCalendarMonths(t *testing.T) {
	cases := []struct{ at, want string }{
		{"2026-11-26T10:00:00Z", "2027-11-26T10:00:00Z"},
		{"2027-01-31T23:59:59Z", "2028-01-31T23:59:59Z"},
		{"2027-03-01T00:00:00Z", "2028-03-01T00:00:00Z"}, // 2028 es bisiesto: 366 días, no 365
		{"2026-11-26T20:00:00-06:00", "2027-11-27T02:00:00Z"},
	}
	for _, c := range cases {
		at, err := time.Parse(time.RFC3339, c.at)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := time.Parse(time.RFC3339, c.want)
		if got := PurgeAt(at); !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("PurgeAt(%s) = %s; quiero %s", c.at, got, c.want)
		}
	}
}
