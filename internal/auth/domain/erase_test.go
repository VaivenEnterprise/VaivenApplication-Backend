package domain

import (
	"testing"
	"time"
)

func TestJobBackoff(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{-3, time.Minute}, {0, time.Minute}, {1, time.Minute}, {2, 2 * time.Minute}, {3, 4 * time.Minute},
		{9, 256 * time.Minute}, {10, 6 * time.Hour}, {20, 6 * time.Hour}, {1000, 6 * time.Hour},
	}
	for _, c := range cases {
		if got := JobBackoff(c.attempts); got != c.want {
			t.Errorf("JobBackoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestJobExhaustedAndKinds(t *testing.T) {
	if JobExhausted(MaxJobAttempts-1) || !JobExhausted(MaxJobAttempts) || !JobExhausted(MaxJobAttempts+1) {
		t.Error("JobExhausted no respeta MaxJobAttempts")
	}
	if !JobImage.Valid() || !JobNotice.Valid() || JobKind("otro").Valid() || JobKind("").Valid() {
		t.Error("JobKind.Valid")
	}
}
