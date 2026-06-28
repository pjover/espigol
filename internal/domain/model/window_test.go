package model

import (
	"testing"
	"time"
)

func TestNewSubmissionWindow_AndWith(t *testing.T) {
	deadline := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	w, err := NewSubmissionWindow(2026, WindowDraft, nil, nil, deadline, MoneyOf(30000), MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	if w.Year() != 2026 || w.State() != WindowDraft {
		t.Errorf("accessors wrong: %+v", w)
	}
	opened := w.WithState(WindowOpen)
	if opened.State() != WindowOpen || w.State() != WindowDraft {
		t.Error("WithState should not mutate original")
	}
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if got := w.WithOpenedAt(now).OpenedAt(); got == nil || !got.Equal(now) {
		t.Errorf("WithOpenedAt = %v, want %v", got, now)
	}
}

func TestNewSubmissionWindow_RejectsBadYear(t *testing.T) {
	if _, err := NewSubmissionWindow(1800, WindowDraft, nil, nil, time.Now(), ZeroMoney(), ZeroMoney()); err == nil {
		t.Fatal("expected error for year < 1900")
	}
}
