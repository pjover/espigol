package mapper

import (
	"testing"
	"time"
)

func TestTimestampRoundTrip(t *testing.T) {
	in := time.Date(2026, 3, 1, 18, 36, 37, 0, time.UTC)
	out, err := ParseTimestamp(FormatTimestamp(in))
	if err != nil || !out.Equal(in) {
		t.Fatalf("round trip: got (%v,%v), want %v", out, err, in)
	}
}

func TestDateRoundTrip(t *testing.T) {
	in := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	s := FormatDate(in)
	if s != "2026-03-01" {
		t.Errorf("FormatDate = %q, want 2026-03-01", s)
	}
	out, err := ParseDate(s)
	if err != nil || !out.Equal(in) {
		t.Fatalf("date round trip: got (%v,%v), want %v", out, err, in)
	}
}
