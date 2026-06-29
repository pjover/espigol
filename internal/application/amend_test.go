package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestAmend_SupersedesAndKeepsClosed(t *testing.T) {
	svc, conn := newSvc(t)
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
