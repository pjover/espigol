package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedDraft2026 builds a DRAFT 2026 window with taxonomy a1(CURRENT)/b1(INVESTMENT),
// sections oliva/ramaderia, and partner 5 (soci, not a board member).
func seedDraft2026(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(2026, model.WindowDraft, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = persistence.NewWindowRepository(q).Save(ctx, w)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	sr := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)
	pr := persistence.NewPartnerRepository(q)
	soci, _ := model.NewPartner(5, "Soci Cinc", "Soci Cinc", "", "", "u5@e.test", "", model.Productor, 0, fcNow(), false)
	_ = pr.Save(ctx, soci)
}

// seedClosed2026 builds a CLOSED 2026 window with the same taxonomy/sections/partner 5.
func seedClosed2026(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(2026, model.WindowClosed, ptrTime(fcNow()), ptrTime(fcNow()),
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = persistence.NewWindowRepository(q).Save(ctx, w)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	sr := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)
	pr := persistence.NewPartnerRepository(q)
	soci, _ := model.NewPartner(5, "Soci Cinc", "Soci Cinc", "", "", "u5@e.test", "", model.Productor, 0, fcNow(), false)
	_ = pr.Save(ctx, soci)
}

// auditEventsFor returns the audit events recorded for the given entity, in insertion order.
func auditEventsFor(t *testing.T, conn *sql.DB, entityType, entityID string) []model.AuditEvent {
	t.Helper()
	all, err := persistence.NewAuditLog(sqlc.New(conn)).List(context.Background())
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	var out []model.AuditEvent
	for _, e := range all {
		if e.EntityType() == entityType && e.EntityID() == entityID {
			out = append(out, e)
		}
	}
	return out
}

func TestForecastService_AdminCreate_DraftYear_OwnedByImpersonatedPartner(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedDraft2026(t, conn)
	ctx := context.Background()

	in := partnerInput("500.00")
	f, err := svc.AdminCreate(ctx, adminEmail, 2026, 5, in)
	if err != nil {
		t.Fatalf("admin create on DRAFT year: %v", err)
	}
	if f.Partner().ID() != 5 {
		t.Errorf("want owned by partner 5, got %d", f.Partner().ID())
	}
	if f.Scope().Kind() != model.ScopePartner {
		t.Errorf("want PARTNER scope, got %v", f.Scope().Kind())
	}

	events := auditEventsFor(t, conn, "ExpenseForecast", f.ID())
	if len(events) == 0 {
		t.Fatal("expected an audit event for admin create")
	}
	last := events[len(events)-1]
	if last.ActorEmail() != adminEmail {
		t.Errorf("audit actor email = %q, want %q", last.ActorEmail(), adminEmail)
	}
	if last.Kind() != model.AuditForecastCreated {
		t.Errorf("audit kind = %v, want AuditForecastCreated", last.Kind())
	}
}

func TestForecastService_AdminCreate_OpenYear_CommonForecast_NoBoardMembership(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn) // partner 1 is a plain soci, not a board member
	ctx := context.Background()

	common := application.ForecastInput{Concept: "Comú", GrossAmount: mustMoney(t, "100.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1", ScopeKind: model.ScopeCommon}

	f, err := svc.AdminCreate(ctx, adminEmail, 2026, 1, common)
	if err != nil {
		t.Fatalf("admin create COMMON on OPEN year despite no board membership: %v", err)
	}
	if f.Scope().Kind() != model.ScopeCommon {
		t.Errorf("want COMMON scope, got %v", f.Scope().Kind())
	}
	if f.Partner().ID() != 1 {
		t.Errorf("want owned by partner 1, got %d", f.Partner().ID())
	}
}

func TestForecastService_AdminUpdateAndDelete_DraftAndOpen(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedDraft2026(t, conn)
	ctx := context.Background()

	in := partnerInput("500.00")
	f, err := svc.AdminCreate(ctx, adminEmail, 2026, 5, in)
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}

	updateIn := partnerInput("750.00")
	if err := svc.AdminUpdate(ctx, adminEmail, f.ID(), updateIn); err != nil {
		t.Fatalf("admin update on DRAFT year: %v", err)
	}

	events := auditEventsFor(t, conn, "ExpenseForecast", f.ID())
	foundEdit := false
	for _, e := range events {
		if e.Kind() == model.AuditForecastEdited && e.ActorEmail() == adminEmail {
			foundEdit = true
		}
	}
	if !foundEdit {
		t.Error("expected an AuditForecastEdited event with admin actor email")
	}

	if err := svc.AdminDelete(ctx, adminEmail, f.ID()); err != nil {
		t.Fatalf("admin delete on DRAFT year: %v", err)
	}

	events = auditEventsFor(t, conn, "ExpenseForecast", f.ID())
	foundDelete := false
	for _, e := range events {
		if e.Kind() == model.AuditForecastDeleted && e.ActorEmail() == adminEmail {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Error("expected an AuditForecastDeleted event with admin actor email")
	}
}

func TestForecastService_AdminCreate_ClosedYear_ErrWindowNotEditable(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedClosed2026(t, conn)
	ctx := context.Background()

	in := partnerInput("500.00")
	if _, err := svc.AdminCreate(ctx, adminEmail, 2026, 5, in); !errors.Is(err, application.ErrWindowNotEditable) {
		t.Errorf("admin create on CLOSED year: want ErrWindowNotEditable, got %v", err)
	}
}

func TestForecastService_AdminUpdateDelete_ClosedYear_ErrWindowNotEditable(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedDraft2026(t, conn)
	ctx := context.Background()

	in := partnerInput("500.00")
	f, err := svc.AdminCreate(ctx, adminEmail, 2026, 5, in)
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}

	// transition the window to CLOSED directly via the repository to simulate
	// the year having closed after the forecast was created.
	q := sqlc.New(conn)
	wr := persistence.NewWindowRepository(q)
	w, ok, err := wr.FindByYear(ctx, 2026)
	if err != nil || !ok {
		t.Fatalf("find window: ok=%v err=%v", ok, err)
	}
	closed := w.WithState(model.WindowClosed).WithClosedAt(fcNow())
	if err := wr.Save(ctx, closed); err != nil {
		t.Fatalf("close window: %v", err)
	}

	if err := svc.AdminUpdate(ctx, adminEmail, f.ID(), partnerInput("999.00")); !errors.Is(err, application.ErrWindowNotEditable) {
		t.Errorf("admin update on CLOSED year: want ErrWindowNotEditable, got %v", err)
	}
	if err := svc.AdminDelete(ctx, adminEmail, f.ID()); !errors.Is(err, application.ErrWindowNotEditable) {
		t.Errorf("admin delete on CLOSED year: want ErrWindowNotEditable, got %v", err)
	}
}
