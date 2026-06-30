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

func scNow() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC) }

func newScSvc(t *testing.T) (*application.SectionService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "sc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewSectionService(persistence.NewTxManager(conn), fixedClock{t: scNow()}, adminEmail)
	return svc, conn
}

func vinyaInput() application.SectionInput {
	return application.SectionInput{
		Code:         "vinya",
		Label:        "Secció de vinya",
		Active:       true,
		DisplayOrder: 1,
	}
}

func TestSectionService_CreateAppearsInList(t *testing.T) {
	svc, _ := newScSvc(t)
	ctx := context.Background()

	in := vinyaInput()
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Code() != "vinya" || created.Label() != "Secció de vinya" || !created.Active() || created.DisplayOrder() != 1 {
		t.Errorf("created section wrong: %+v", created)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, s := range list {
		if s.Code() == "vinya" {
			found = true
		}
	}
	if !found {
		t.Errorf("created section not found in List")
	}
}

func TestSectionService_CreateDuplicateCodeReturnsErrSectionExists(t *testing.T) {
	svc, _ := newScSvc(t)
	ctx := context.Background()

	in := vinyaInput()
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := svc.Create(ctx, in); !errors.Is(err, application.ErrSectionExists) {
		t.Errorf("duplicate code: want ErrSectionExists, got %v", err)
	}
}

func TestSectionService_UpdateChangesLabelAndOrder(t *testing.T) {
	svc, _ := newScSvc(t)
	ctx := context.Background()

	in := vinyaInput()
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := in
	updated.Label = "Vinya actualitzada"
	updated.DisplayOrder = 5
	if err := svc.Update(ctx, "vinya", updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	s := findSection(t, svc, "vinya")
	if s.Label() != "Vinya actualitzada" || s.DisplayOrder() != 5 {
		t.Errorf("Update not applied: %+v", s)
	}
}

func TestSectionService_UpdateNotFoundReturnsErrSectionNotFound(t *testing.T) {
	svc, _ := newScSvc(t)
	ctx := context.Background()

	if err := svc.Update(ctx, "nonexistent", vinyaInput()); !errors.Is(err, application.ErrSectionNotFound) {
		t.Errorf("Update missing: want ErrSectionNotFound, got %v", err)
	}
}

func TestSectionService_UpdateToInactiveInUseReturnsErrSectionInUse(t *testing.T) {
	svc, conn := newScSvc(t)
	ctx := context.Background()

	in := vinyaInput()
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	seedOpenWindowWithSectionForecast(t, conn, "vinya")

	deactivated := in
	deactivated.Active = false
	if err := svc.Update(ctx, "vinya", deactivated); !errors.Is(err, application.ErrSectionInUse) {
		t.Errorf("want ErrSectionInUse, got %v", err)
	}
}

func TestSectionService_AuditActorIsAdminEmail(t *testing.T) {
	svc, conn := newScSvc(t)
	ctx := context.Background()

	in := vinyaInput()
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	found := false
	for _, a := range audits {
		if a.EntityType() == "Section" && a.EntityID() == "vinya" {
			if a.ActorEmail() != adminEmail {
				t.Errorf("audit actor email = %q, want %q", a.ActorEmail(), adminEmail)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("no audit event found for section vinya")
	}
}

// findSection returns the section with the given code from the service's List,
// failing the test if absent.
func findSection(t *testing.T, svc *application.SectionService, code string) model.Section {
	t.Helper()
	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range list {
		if s.Code() == code {
			return s
		}
	}
	t.Fatalf("section %q not found", code)
	return model.Section{}
}

// seedOpenWindowWithSectionForecast seeds an OPEN 2026 window, taxonomy a1/A,
// a partner, and a forecast scoped to the given section code, so an
// in-use check against that section should reject deactivation.
func seedOpenWindowWithSectionForecast(t *testing.T, conn *sql.DB, sectionCode string) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()

	w, err := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(scNow()), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	if err := tax.SaveType(ctx, ta); err != nil {
		t.Fatal(err)
	}
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	if err := tax.SaveSubtype(ctx, sa); err != nil {
		t.Fatal(err)
	}

	pr := persistence.NewPartnerRepository(q)
	soci, _ := model.NewPartner(1, "Soci U", "", "", "u1@e.test", "", model.Productor, 0, scNow(), false)
	if err := pr.Save(ctx, soci); err != nil {
		t.Fatal(err)
	}

	scope, err := model.NewSectionScope(sectionCode)
	if err != nil {
		t.Fatal(err)
	}
	gross := model.MoneyOf(100)
	f, err := model.NewUnsavedExpenseForecast(1, "Concept", "", gross, model.ZeroMoney(), nil,
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), 2026, "a1", scope, scNow(), true)
	if err != nil {
		t.Fatal(err)
	}
	fr := persistence.NewForecastRepository(conn, q)
	if _, err := fr.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
}
