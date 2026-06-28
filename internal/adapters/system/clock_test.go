package system

import (
	"testing"
	"time"
)

func TestSystemClock_NowIsUTCAndRecent(t *testing.T) {
	c := SystemClock{}
	now := c.Now()
	if now.Location() != time.UTC {
		t.Errorf("Now() location = %v, want UTC", now.Location())
	}
	if time.Since(now) > time.Minute {
		t.Errorf("Now() = %v is not recent", now)
	}
}
