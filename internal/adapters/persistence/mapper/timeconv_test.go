package mapper

import (
	"database/sql"
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

func TestFormatNullableTimestamp_Nil(t *testing.T) {
	ns := FormatNullableTimestamp(nil)
	if ns.Valid {
		t.Errorf("FormatNullableTimestamp(nil) = {%q, valid=true}, want invalid NullString", ns.String)
	}
}

func TestParseNullableTimestamp_InvalidNullString(t *testing.T) {
	pt, err := ParseNullableTimestamp(sql.NullString{})
	if err != nil {
		t.Fatalf("ParseNullableTimestamp(invalid): unexpected error: %v", err)
	}
	if pt != nil {
		t.Errorf("ParseNullableTimestamp(invalid) = %v, want nil", pt)
	}
}

func TestNullableTimestampRoundTrip(t *testing.T) {
	in := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	ns := FormatNullableTimestamp(&in)
	if !ns.Valid {
		t.Fatal("FormatNullableTimestamp(&t) returned invalid NullString")
	}
	out, err := ParseNullableTimestamp(ns)
	if err != nil {
		t.Fatalf("ParseNullableTimestamp: %v", err)
	}
	if out == nil {
		t.Fatal("ParseNullableTimestamp returned nil for a valid NullString")
	}
	if !out.Equal(in) {
		t.Errorf("round trip: got %v, want %v", *out, in)
	}
}
