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
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func txNow() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC) }

func newTxSvc(t *testing.T) (*application.TaxonomyService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewTaxonomyService(persistence.NewTxManager(conn), fixedClock{t: txNow()}, adminEmail)
	return svc, conn
}

// seedWindow inserts a SubmissionWindow with the given year/state directly,
// bypassing WindowService so taxonomy mutations can be tested in isolation.
func seedWindow(t *testing.T, conn *sql.DB, year int, state model.WindowState) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()

	var openedAt, closedAt *time.Time
	if state == model.WindowOpen || state == model.WindowClosed {
		openedAt = ptrTime(txNow())
	}
	if state == model.WindowClosed {
		closedAt = ptrTime(txNow())
	}

	w, err := model.NewSubmissionWindow(year, state, openedAt, closedAt,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}
}

func typeAInput(year int) application.TypeInput {
	return application.TypeInput{Year: year, Code: "A", Label: "[a]", Category: model.CategoryCurrent}
}

func subtypeA1Input(year int) application.SubtypeInput {
	return application.SubtypeInput{Year: year, Code: "a1", Label: "[a1]", TypeCode: "A"}
}

// seedForecastUsingSubtype seeds a partner and a forecast referencing the
// given year/subtype code so a delete-in-use check can be exercised. The
// window for that year must already exist (any state) before calling this.
func seedForecastUsingSubtype(t *testing.T, conn *sql.DB, year int, subtypeCode string) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()

	pr := persistence.NewPartnerRepository(q)
	soci, err := model.NewPartner(1, "Soci U", "", "", "u1@e.test", "", model.Productor, 0, txNow(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := pr.Save(ctx, soci); err != nil {
		t.Fatal(err)
	}

	scope := model.NewCommonScope()
	gross := model.MoneyOf(100)
	f, err := model.NewUnsavedExpenseForecast(soci, "Concept", "", gross, model.ZeroMoney(), nil,
		time.Date(year, 6, 15, 0, 0, 0, 0, time.UTC), year, subtypeCode, scope, txNow(), true)
	if err != nil {
		t.Fatal(err)
	}
	fr := persistence.NewForecastRepository(conn, q)
	if _, err := fr.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
}

func TestTaxonomyService_CreateTypeAndSubtypeAppearInLists(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	if err := svc.CreateType(ctx, typeAInput(2026)); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); err != nil {
		t.Fatalf("CreateSubtype: %v", err)
	}

	types, err := svc.ListTypes(ctx, 2026)
	if err != nil {
		t.Fatalf("ListTypes: %v", err)
	}
	foundType := false
	for _, ty := range types {
		if ty.Code() == "A" {
			foundType = true
			if ty.Category() != model.CategoryCurrent {
				t.Errorf("type category = %v, want CURRENT", ty.Category())
			}
		}
	}
	if !foundType {
		t.Errorf("created type not found in ListTypes")
	}

	subtypes, err := svc.ListSubtypes(ctx, 2026)
	if err != nil {
		t.Fatalf("ListSubtypes: %v", err)
	}
	foundSubtype := false
	for _, st := range subtypes {
		if st.Code() == "a1" {
			foundSubtype = true
			if st.TypeCode() != "A" {
				t.Errorf("subtype typeCode = %q, want %q", st.TypeCode(), "A")
			}
		}
	}
	if !foundSubtype {
		t.Errorf("created subtype not found in ListSubtypes")
	}
}

func TestTaxonomyService_CreateSubtypeMissingTypeReturnsErrTypeNotFound(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	in := subtypeA1Input(2026)
	in.TypeCode = "ZZZ"
	if err := svc.CreateSubtype(ctx, in); !errors.Is(err, application.ErrTypeNotFound) {
		t.Errorf("want ErrTypeNotFound, got %v", err)
	}
}

func TestTaxonomyService_DeleteSubtypeRemovesIt(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	if err := svc.CreateType(ctx, typeAInput(2026)); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); err != nil {
		t.Fatalf("CreateSubtype: %v", err)
	}

	if err := svc.DeleteSubtype(ctx, 2026, "a1"); err != nil {
		t.Fatalf("DeleteSubtype: %v", err)
	}

	subtypes, err := svc.ListSubtypes(ctx, 2026)
	if err != nil {
		t.Fatalf("ListSubtypes: %v", err)
	}
	for _, st := range subtypes {
		if st.Code() == "a1" {
			t.Errorf("subtype a1 still present after delete")
		}
	}
}

func TestTaxonomyService_DeleteTypeBlockedWhileSubtypeReferencesIt(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	if err := svc.CreateType(ctx, typeAInput(2026)); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); err != nil {
		t.Fatalf("CreateSubtype: %v", err)
	}

	if err := svc.DeleteType(ctx, 2026, "A"); !errors.Is(err, application.ErrTypeInUse) {
		t.Errorf("want ErrTypeInUse, got %v", err)
	}

	if err := svc.DeleteSubtype(ctx, 2026, "a1"); err != nil {
		t.Fatalf("DeleteSubtype: %v", err)
	}

	if err := svc.DeleteType(ctx, 2026, "A"); err != nil {
		t.Errorf("DeleteType after subtype removed: %v", err)
	}
}

func TestTaxonomyService_DeleteSubtypeBlockedIfForecastUsesIt(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	if err := svc.CreateType(ctx, typeAInput(2026)); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); err != nil {
		t.Fatalf("CreateSubtype: %v", err)
	}
	seedForecastUsingSubtype(t, conn, 2026, "a1")

	if err := svc.DeleteSubtype(ctx, 2026, "a1"); !errors.Is(err, application.ErrSubtypeInUse) {
		t.Errorf("want ErrSubtypeInUse, got %v", err)
	}
}

func TestTaxonomyService_OpenYearBlocksAllMutations(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowOpen)

	if err := svc.CreateType(ctx, typeAInput(2026)); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("CreateType: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.UpdateType(ctx, typeAInput(2026)); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("UpdateType: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.DeleteType(ctx, 2026, "A"); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("DeleteType: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("CreateSubtype: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.UpdateSubtype(ctx, subtypeA1Input(2026)); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("UpdateSubtype: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.DeleteSubtype(ctx, 2026, "a1"); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("DeleteSubtype: want ErrTaxonomyLocked, got %v", err)
	}
}

func TestTaxonomyService_ClosedYearBlocksAllMutations(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2025, model.WindowClosed)

	if err := svc.CreateType(ctx, typeAInput(2025)); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("CreateType: want ErrTaxonomyLocked, got %v", err)
	}
	if err := svc.DeleteSubtype(ctx, 2025, "a1"); !errors.Is(err, application.ErrTaxonomyLocked) {
		t.Errorf("DeleteSubtype: want ErrTaxonomyLocked, got %v", err)
	}
}

func TestTaxonomyService_UnknownYearReturnsErrWindowNotFound(t *testing.T) {
	svc, _ := newTxSvc(t)
	ctx := context.Background()

	if err := svc.CreateType(ctx, typeAInput(2099)); !errors.Is(err, application.ErrWindowNotFound) {
		t.Errorf("want ErrWindowNotFound, got %v", err)
	}
}

func TestTaxonomyService_AuditAppendedOnSuccess(t *testing.T) {
	svc, conn := newTxSvc(t)
	ctx := context.Background()
	seedWindow(t, conn, 2026, model.WindowDraft)

	if err := svc.CreateType(ctx, typeAInput(2026)); err != nil {
		t.Fatalf("CreateType: %v", err)
	}
	if err := svc.CreateSubtype(ctx, subtypeA1Input(2026)); err != nil {
		t.Fatalf("CreateSubtype: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	foundType, foundSubtype := false, false
	for _, a := range audits {
		if a.EntityType() == "ExpenseType" && a.EntityID() == "A" {
			foundType = true
			if a.Kind() != model.AuditTaxonomySaved {
				t.Errorf("type audit kind = %v, want AuditTaxonomySaved", a.Kind())
			}
			if a.ActorEmail() != adminEmail {
				t.Errorf("type audit actor = %q, want %q", a.ActorEmail(), adminEmail)
			}
		}
		if a.EntityType() == "ExpenseSubtype" && a.EntityID() == "a1" {
			foundSubtype = true
			if a.Kind() != model.AuditTaxonomySaved {
				t.Errorf("subtype audit kind = %v, want AuditTaxonomySaved", a.Kind())
			}
		}
	}
	if !foundType {
		t.Errorf("no audit event found for ExpenseType A")
	}
	if !foundSubtype {
		t.Errorf("no audit event found for ExpenseSubtype a1")
	}

	if err := svc.DeleteSubtype(ctx, 2026, "a1"); err != nil {
		t.Fatalf("DeleteSubtype: %v", err)
	}
	audits, err = persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	foundDelete := false
	for _, a := range audits {
		if a.EntityType() == "ExpenseSubtype" && a.EntityID() == "a1" && a.Kind() == model.AuditTaxonomyDeleted {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Errorf("no AuditTaxonomyDeleted event found for ExpenseSubtype a1")
	}
}
