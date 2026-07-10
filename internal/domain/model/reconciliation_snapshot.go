package model

import (
	"fmt"
	"time"
)

type ReconciliationSnapshot struct {
	year         int
	generatedAt  time.Time
	snapshotJSON string
	pdf          []byte
}

func NewReconciliationSnapshot(year int, at time.Time, snapshotJSON string, pdf []byte) (ReconciliationSnapshot, error) {
	if year < 0 {
		return ReconciliationSnapshot{}, fmt.Errorf("reconciliation snapshot: year must be non-negative, got %d", year)
	}
	if snapshotJSON == "" {
		return ReconciliationSnapshot{}, fmt.Errorf("reconciliation snapshot: snapshotJSON must not be empty")
	}
	return ReconciliationSnapshot{year: year, generatedAt: at, snapshotJSON: snapshotJSON, pdf: pdf}, nil
}

func (r ReconciliationSnapshot) Year() int              { return r.year }
func (r ReconciliationSnapshot) GeneratedAt() time.Time { return r.generatedAt }
func (r ReconciliationSnapshot) SnapshotJSON() string   { return r.snapshotJSON }
func (r ReconciliationSnapshot) Pdf() []byte            { return r.pdf }
