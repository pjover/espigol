package model

import (
	"fmt"
	"time"
)

type Report struct {
	id           int
	year         int
	generatedAt  time.Time
	snapshotJSON string
	pdf          []byte
	supersededAt *time.Time
}

func NewReport(id, year int, generatedAt time.Time, snapshotJSON string, pdf []byte,
	supersededAt *time.Time) (Report, error) {
	if snapshotJSON == "" {
		return Report{}, fmt.Errorf("report snapshotJSON must not be empty")
	}
	return Report{id, year, generatedAt, snapshotJSON, pdf, supersededAt}, nil
}

func (r Report) ID() int                { return r.id }
func (r Report) Year() int              { return r.year }
func (r Report) GeneratedAt() time.Time { return r.generatedAt }
func (r Report) SnapshotJSON() string   { return r.snapshotJSON }
func (r Report) Pdf() []byte            { return r.pdf }
func (r Report) SupersededAt() *time.Time { return r.supersededAt }

func (r Report) WithSupersededAt(t time.Time) Report { r.supersededAt = &t; return r }
