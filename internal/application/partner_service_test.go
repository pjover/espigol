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

const adminEmail = "admin@espigol.test"

func ptNow() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC) }

func newPtSvc(t *testing.T) (*application.PartnerService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "pt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewPartnerService(persistence.NewTxManager(conn), fixedClock{t: ptNow()}, adminEmail)
	return svc, conn
}

// seedSections seeds oliva and ramaderia sections into the DB.
func seedSections(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	sr := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ramaderia, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	if err := sr.Save(ctx, oliva); err != nil {
		t.Fatal(err)
	}
	if err := sr.Save(ctx, ramaderia); err != nil {
		t.Fatal(err)
	}
}

func baseInput(id int) application.PartnerInput {
	return application.PartnerInput{
		ID:          id,
		Name:        "Test",
		Surname:     "Partner",
		VatCode:     "12345678A",
		Email:       "tp@e.test",
		Mobile:      "612345678",
		PartnerType: model.Productor,
		RiaNumber:   0,
		BoardMember: false,
	}
}

func TestPartnerService_CreateAppearsInList(t *testing.T) {
	svc, _ := newPtSvc(t)
	ctx := context.Background()

	in := baseInput(42)
	p, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID() != 42 || p.Name() != "Test" || p.Email() != "tp@e.test" {
		t.Errorf("created partner wrong: %+v", p)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, lp := range list {
		if lp.ID() == 42 {
			found = true
		}
	}
	if !found {
		t.Errorf("created partner not found in List")
	}
}

func TestPartnerService_CreateDuplicateIDReturnsErrPartnerExists(t *testing.T) {
	svc, _ := newPtSvc(t)
	ctx := context.Background()

	in := baseInput(10)
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := svc.Create(ctx, in); !errors.Is(err, application.ErrPartnerExists) {
		t.Errorf("duplicate id: want ErrPartnerExists, got %v", err)
	}
}

func TestPartnerService_CreateDuplicateEmailReturnsErrEmailTaken(t *testing.T) {
	svc, _ := newPtSvc(t)
	ctx := context.Background()

	in1 := baseInput(11)
	in1.Email = "shared@e.test"
	if _, err := svc.Create(ctx, in1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	in2 := baseInput(12)
	in2.Email = "shared@e.test"
	if _, err := svc.Create(ctx, in2); !errors.Is(err, application.ErrEmailTaken) {
		t.Errorf("duplicate email: want ErrEmailTaken, got %v", err)
	}
}

func TestPartnerService_UpdateChangesFields(t *testing.T) {
	svc, _ := newPtSvc(t)
	ctx := context.Background()

	in := baseInput(20)
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	originalAddedOn := created.AddedOn()

	updated := in
	updated.Name = "Updated"
	updated.Email = "updated@e.test"
	if err := svc.Update(ctx, 20, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list, _ := svc.List(ctx)
	for _, p := range list {
		if p.ID() == 20 {
			if p.Name() != "Updated" || p.Email() != "updated@e.test" {
				t.Errorf("Update not applied: %+v", p)
			}
			if p.AddedOn() != originalAddedOn {
				t.Errorf("AddedOn changed: got %v, want %v", p.AddedOn(), originalAddedOn)
			}
			return
		}
	}
	t.Error("partner 20 not found after update")
}

func TestPartnerService_UpdateNotFoundReturnsErrPartnerNotFound(t *testing.T) {
	svc, _ := newPtSvc(t)
	ctx := context.Background()

	if err := svc.Update(ctx, 999, baseInput(999)); !errors.Is(err, application.ErrPartnerNotFound) {
		t.Errorf("Update missing: want ErrPartnerNotFound, got %v", err)
	}
}

func TestPartnerService_SetBoardMemberFlipsFlag(t *testing.T) {
	svc, conn := newPtSvc(t)
	ctx := context.Background()

	in := baseInput(30)
	in.BoardMember = false
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.SetBoardMember(ctx, 30, true); err != nil {
		t.Fatalf("SetBoardMember: %v", err)
	}

	p, ok, err := persistence.NewPartnerRepository(sqlc.New(conn)).FindByID(ctx, 30)
	if err != nil || !ok {
		t.Fatalf("FindByID: %v", err)
	}
	if !p.BoardMember() {
		t.Errorf("BoardMember not flipped to true")
	}
}

func TestPartnerService_SetSectionMembershipsReplaces(t *testing.T) {
	svc, conn := newPtSvc(t)
	seedSections(t, conn)
	ctx := context.Background()

	in := baseInput(50)
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// First assignment: oliva
	if err := svc.SetSectionMemberships(ctx, 50, []string{"oliva"}); err != nil {
		t.Fatalf("SetSectionMemberships(oliva): %v", err)
	}

	// Second assignment: ramaderia — should replace oliva
	if err := svc.SetSectionMemberships(ctx, 50, []string{"ramaderia"}); err != nil {
		t.Fatalf("SetSectionMemberships(ramaderia): %v", err)
	}

	sr := persistence.NewSectionRepository(sqlc.New(conn))
	memberships, err := sr.ListMembershipsByPartner(ctx, 50)
	if err != nil {
		t.Fatalf("ListMembershipsByPartner: %v", err)
	}
	if len(memberships) != 1 || memberships[0].SectionCode() != "ramaderia" {
		t.Errorf("expected only ramaderia membership, got %+v", memberships)
	}
}

func TestPartnerService_SetSectionMembershipsUnknownSectionReturnsErrSectionNotFound(t *testing.T) {
	svc, conn := newPtSvc(t)
	seedSections(t, conn)
	ctx := context.Background()

	in := baseInput(55)
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.SetSectionMemberships(ctx, 55, []string{"nonexistent"}); !errors.Is(err, application.ErrSectionNotFound) {
		t.Errorf("want ErrSectionNotFound, got %v", err)
	}
}

func TestPartnerService_AuditActorIsAdminEmail(t *testing.T) {
	svc, conn := newPtSvc(t)
	ctx := context.Background()

	in := baseInput(60)
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	found := false
	for _, a := range audits {
		if a.EntityType() == "Partner" && a.EntityID() == "60" {
			if a.ActorEmail() != adminEmail {
				t.Errorf("audit actor email = %q, want %q", a.ActorEmail(), adminEmail)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("no audit event found for partner 60")
	}
}
