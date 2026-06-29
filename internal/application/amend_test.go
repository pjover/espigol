package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	appreport "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// advancingClock returns a time that advances by one minute on each call.
type advancingClock struct{ t time.Time }

func (c *advancingClock) Now() time.Time {
	now := c.t
	c.t = c.t.Add(time.Minute)
	return now
}

func TestAmend_SupersedesAndKeepsClosed(t *testing.T) {
	clock := &advancingClock{t: time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC)}
	conn, err := db.Open(filepath.Join(t.TempDir(), "svc_amend.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewWindowService(persistence.NewTxManager(conn), appreport.NoopRenderer{}, clock)
	seedOpenYearWithForecasts(t, conn)
	ctx := context.Background()

	first, err := svc.Close(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Amend(ctx, 2027)
	if err != nil {
		t.Fatalf("amend: %v", err)
	}
	if second.ID() == first.ID() {
		t.Errorf("amend should insert a new report")
	}

	// window stays CLOSED
	w, _, _ := persistence.NewWindowRepository(sqlc.New(conn)).FindByYear(ctx, 2027)
	if w.State() != model.WindowClosed {
		t.Errorf("window should stay CLOSED, got %s", w.State())
	}
	// latest non-superseded report is the amended one
	latest, ok, _ := persistence.NewReportRepository(sqlc.New(conn)).FindLatestByYear(ctx, 2027)
	if !ok || latest.ID() != second.ID() {
		t.Errorf("latest report = %d, want amended %d", latest.ID(), second.ID())
	}
	// a REPORT_GENERATED audit exists
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawGen bool
	for _, a := range audits {
		if a.Kind() == model.AuditReportGenerated {
			sawGen = true
		}
	}
	if !sawGen {
		t.Errorf("no REPORT_GENERATED audit")
	}
}

func TestAmend_RejectsNonClosed(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	_, _ = svc.CreateYear(ctx, 2027)
	if _, err := svc.Amend(ctx, 2027); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState amending a DRAFT window, got %v", err)
	}
}
