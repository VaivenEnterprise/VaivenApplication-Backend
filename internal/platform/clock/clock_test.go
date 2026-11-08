package clock

import (
	"testing"
	"time"
)

func TestSystemNowIsUTC(t *testing.T) {
	var c Clock = System{}
	before := time.Now()
	got := c.Now()
	if got.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", got.Location())
	}
	if got.Before(before.Add(-time.Second)) || got.After(time.Now().Add(time.Second)) {
		t.Fatalf("Now() = %v, fuera de rango", got)
	}
}

func TestFixedNow(t *testing.T) {
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var c Clock = Fixed{T: want}
	if got := c.Now(); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}
