// Package system holds adapters for system facilities (clock, etc.).
package system

import "time"

// SystemClock implements ports.Clock using the wall clock in UTC.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }
