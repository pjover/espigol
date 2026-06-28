package forecastid

import "testing"

func TestFormat_Decimal(t *testing.T) {
	cases := map[int]string{0: "CP26000", 1: "CP26001", 36: "CP26036", 999: "CP26999"}
	for seq, want := range cases {
		got, err := Format(2026, seq)
		if err != nil || got != want {
			t.Errorf("Format(2026,%d) = (%q,%v), want %q", seq, got, err, want)
		}
	}
}

func TestFormat_LetterOverflow(t *testing.T) {
	cases := map[int]string{1000: "CP26A00", 1099: "CP26A99", 1100: "CP26B00", 3599: "CP26Z99"}
	for seq, want := range cases {
		got, err := Format(2026, seq)
		if err != nil || got != want {
			t.Errorf("Format(2026,%d) = (%q,%v), want %q", seq, got, err, want)
		}
	}
}

func TestFormat_OutOfRange(t *testing.T) {
	if _, err := Format(2026, 3600); err == nil {
		t.Error("expected error for seq > 3599")
	}
	if _, err := Format(2026, -1); err == nil {
		t.Error("expected error for negative seq")
	}
}

func TestParseSeq_RoundTrip(t *testing.T) {
	for _, seq := range []int{0, 1, 36, 999, 1000, 1099, 1100, 3599} {
		id, _ := Format(2026, seq)
		y, gotSeq, err := ParseSeq(id)
		if err != nil || y != 2026 || gotSeq != seq {
			t.Errorf("ParseSeq(%q) = (%d,%d,%v), want (2026,%d)", id, y, gotSeq, err, seq)
		}
	}
}

func TestParseSeq_Invalid(t *testing.T) {
	for _, bad := range []string{"", "XX26001", "CP2601", "CP26@00", "CP260000"} {
		if _, _, err := ParseSeq(bad); err == nil {
			t.Errorf("ParseSeq(%q) should error", bad)
		}
	}
}
