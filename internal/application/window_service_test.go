package application_test

import (
	"context"
	"database/sql"
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
	"github.com/pjover/espigol/internal/domain/ports"
)

// fixedClock is a deterministic Clock.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var _ ports.Clock = fixedClock{}

func newSvc(t *testing.T) (*application.WindowService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	clock := fixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	svc := application.NewWindowService(persistence.NewTxManager(conn), appreport.NoopRenderer{}, clock)
	return svc, conn
}

// seedClosedYear creates a CLOSED window for `year` with a minimal taxonomy
// directly via repositories (so CreateYear/Open have a prior year to copy).
func seedClosedYear(t *testing.T, conn *sql.DB, year int) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(year, model.WindowClosed, nil, nil,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(year, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(year, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(year, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(year, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
}

func TestCreateYear_CopiesTaxonomyAndLimits(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()

	w, err := svc.CreateYear(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if w.State() != model.WindowDraft || w.Year() != 2027 {
		t.Errorf("new window = %+v", w)
	}
	if w.CurrentExpenseLimit().String() != "30000.00" || w.InvestmentExpenseLimit().String() != "70000.00" {
		t.Errorf("limits not copied: %s / %s", w.CurrentExpenseLimit(), w.InvestmentExpenseLimit())
	}
	tax := persistence.NewTaxonomyRepository(sqlc.New(conn))
	types, _ := tax.ListTypes(ctx, 2027)
	subs, _ := tax.ListSubtypes(ctx, 2027)
	if len(types) != 2 || len(subs) != 2 {
		t.Errorf("taxonomy not copied: types=%d subs=%d", len(types), len(subs))
	}
}

func TestCreateYear_Errors(t *testing.T) {
	svc, conn := newSvc(t)
	ctx := context.Background()
	if _, err := svc.CreateYear(ctx, 2027); !errors.Is(err, application.ErrNoPriorYear) {
		t.Errorf("want ErrNoPriorYear, got %v", err)
	}
	seedClosedYear(t, conn, 2026)
	if _, err := svc.CreateYear(ctx, 2026); !errors.Is(err, application.ErrYearExists) {
		t.Errorf("want ErrYearExists, got %v", err)
	}
}

func TestOpen_HappyPathAndValidations(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	if _, err := svc.CreateYear(ctx, 2027); err != nil {
		t.Fatal(err)
	}

	if err := svc.Open(ctx, 2027); err != nil {
		t.Fatalf("open: %v", err)
	}
	wr := persistence.NewWindowRepository(sqlc.New(conn))
	w, _, _ := wr.FindByYear(ctx, 2027)
	if w.State() != model.WindowOpen || w.OpenedAt() == nil {
		t.Errorf("window not opened: %+v", w)
	}
	// re-open a non-DRAFT window
	if err := svc.Open(ctx, 2027); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState reopening, got %v", err)
	}
	// audit written
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawOpen bool
	for _, a := range audits {
		if a.Kind() == model.AuditWindowOpened {
			sawOpen = true
		}
	}
	if !sawOpen {
		t.Errorf("no WINDOW_OPENED audit event")
	}
}

func TestOpen_RejectsAnotherOpen(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	_, _ = svc.CreateYear(ctx, 2027)
	_, _ = svc.CreateYear(ctx, 2028)
	if err := svc.Open(ctx, 2027); err != nil {
		t.Fatal(err)
	}
	if err := svc.Open(ctx, 2028); !errors.Is(err, application.ErrAnotherWindowOpen) {
		t.Errorf("want ErrAnotherWindowOpen, got %v", err)
	}
}
