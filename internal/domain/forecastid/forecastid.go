// Package forecastid formats and parses expense-forecast ids of the form
// CP + 2-digit year + 3-char sequence. Sequence 0..999 is decimal (%03d);
// 1000..3599 uses a leading letter for the hundreds block: 1000 -> "A00",
// 1099 -> "A99", 1100 -> "B00", ... 3599 -> "Z99".
package forecastid

import (
	"fmt"
	"strconv"
)

const maxSeq = 3599 // 999 decimal + 26*100 letter-block slots - 1

// Format renders (year, seq) into a CPYYnnn id.
func Format(year, seq int) (string, error) {
	if seq < 0 || seq > maxSeq {
		return "", fmt.Errorf("forecast sequence out of range [0,%d]: %d", maxSeq, seq)
	}
	yy := year % 100
	if seq <= 999 {
		return fmt.Sprintf("CP%02d%03d", yy, seq), nil
	}
	m := seq - 1000
	letter := byte('A' + m/100)
	return fmt.Sprintf("CP%02d%c%02d", yy, letter, m%100), nil
}

// ParseSeq is the inverse of Format. year is reconstructed as 2000+YY.
func ParseSeq(id string) (year, seq int, err error) {
	if len(id) != 7 || id[:2] != "CP" {
		return 0, 0, fmt.Errorf("invalid forecast id: %q", id)
	}
	yy, err := strconv.Atoi(id[2:4])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid forecast id year: %q", id)
	}
	year = 2000 + yy
	tail := id[4:7]
	if tail[0] >= '0' && tail[0] <= '9' {
		n, err := strconv.Atoi(tail)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
		}
		return year, n, nil
	}
	if tail[0] < 'A' || tail[0] > 'Z' {
		return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
	}
	rest, err := strconv.Atoi(tail[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
	}
	return year, 1000 + int(tail[0]-'A')*100 + rest, nil
}
