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

func baNow() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC) }

func newBaSvc(t *testing.T) (*application.BoardAuthorizationService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "ba.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewBoardAuthorizationService(persistence.NewTxManager(conn), fixedClock{t: baNow()}, adminEmail)
	return svc, conn
}

// seedBoardPartner inserts a board-member partner with the given id.
func seedBoardPartner(t *testing.T, conn *sql.DB, id int, board bool) {
	t.Helper()
	q := sqlc.New(conn)
	pr := persistence.NewPartnerRepository(q)
	p, err := model.NewPartner(id, "Soci", "Cognom", "", "soci@e.test", "", model.Productor, 0, baNow(), board)
	if err != nil {
		t.Fatal(err)
	}
	if err := pr.Save(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// seedBaSection inserts a section with the given code.
func seedBaSection(t *testing.T, conn *sql.DB, code string) {
	t.Helper()
	q := sqlc.New(conn)
	sr := persistence.NewSectionRepository(q)
	sec, err := model.NewSection(code, "Secció "+code, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := sr.Save(context.Background(), sec); err != nil {
		t.Fatal(err)
	}
}

func TestBoardAuthorizationService_GrantCommonAppearsInList(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 1, true)

	if err := svc.Grant(ctx, 1, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	list, err := svc.ListByPartner(ctx, 1)
	if err != nil {
		t.Fatalf("ListByPartner: %v", err)
	}
	found := false
	for _, a := range list {
		if a.ScopeKind() == model.ScopeCommon && a.SectionCode() == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("granted COMMON authorization not found in ListByPartner: %+v", list)
	}
}

func TestBoardAuthorizationService_GrantNonBoardPartnerReturnsErrNotBoardMember(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 2, false)

	if err := svc.Grant(ctx, 2, model.ScopeCommon, ""); !errors.Is(err, application.ErrNotBoardMember) {
		t.Errorf("Grant non-board: want ErrNotBoardMember, got %v", err)
	}
}

func TestBoardAuthorizationService_GrantUnknownPartnerReturnsErrPartnerNotFound(t *testing.T) {
	svc, _ := newBaSvc(t)
	ctx := context.Background()

	if err := svc.Grant(ctx, 999, model.ScopeCommon, ""); !errors.Is(err, application.ErrPartnerNotFound) {
		t.Errorf("Grant unknown partner: want ErrPartnerNotFound, got %v", err)
	}
}

func TestBoardAuthorizationService_GrantSectionUnknownSectionReturnsErrSectionNotFound(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 3, true)

	if err := svc.Grant(ctx, 3, model.ScopeSection, "nonexistent"); !errors.Is(err, application.ErrSectionNotFound) {
		t.Errorf("Grant unknown section: want ErrSectionNotFound, got %v", err)
	}
}

func TestBoardAuthorizationService_GrantSectionKnownSectionSucceeds(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 4, true)
	seedBaSection(t, conn, "oliva")

	if err := svc.Grant(ctx, 4, model.ScopeSection, "oliva"); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	list, err := svc.ListByPartner(ctx, 4)
	if err != nil {
		t.Fatalf("ListByPartner: %v", err)
	}
	found := false
	for _, a := range list {
		if a.ScopeKind() == model.ScopeSection && a.SectionCode() == "oliva" {
			found = true
		}
	}
	if !found {
		t.Errorf("granted SECTION authorization not found in ListByPartner: %+v", list)
	}
}

func TestBoardAuthorizationService_GrantDuplicateReturnsErrAuthExists(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 5, true)

	if err := svc.Grant(ctx, 5, model.ScopeCommon, ""); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := svc.Grant(ctx, 5, model.ScopeCommon, ""); !errors.Is(err, application.ErrAuthExists) {
		t.Errorf("duplicate Grant: want ErrAuthExists, got %v", err)
	}
}

func TestBoardAuthorizationService_GrantDuplicateSectionReturnsErrAuthExists(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 6, true)
	seedBaSection(t, conn, "vinya")

	if err := svc.Grant(ctx, 6, model.ScopeSection, "vinya"); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := svc.Grant(ctx, 6, model.ScopeSection, "vinya"); !errors.Is(err, application.ErrAuthExists) {
		t.Errorf("duplicate Grant SECTION: want ErrAuthExists, got %v", err)
	}
}

func TestBoardAuthorizationService_RevokeCommonRemovesIt(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 7, true)

	if err := svc.Grant(ctx, 7, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := svc.Revoke(ctx, 7, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	list, err := svc.ListByPartner(ctx, 7)
	if err != nil {
		t.Fatalf("ListByPartner: %v", err)
	}
	for _, a := range list {
		if a.ScopeKind() == model.ScopeCommon && a.SectionCode() == "" {
			t.Errorf("COMMON authorization still present after Revoke: %+v", list)
		}
	}
}

func TestBoardAuthorizationService_RevokeSectionRemovesIt(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 8, true)
	seedBaSection(t, conn, "horta")

	if err := svc.Grant(ctx, 8, model.ScopeSection, "horta"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := svc.Revoke(ctx, 8, model.ScopeSection, "horta"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	list, err := svc.ListByPartner(ctx, 8)
	if err != nil {
		t.Fatalf("ListByPartner: %v", err)
	}
	for _, a := range list {
		if a.ScopeKind() == model.ScopeSection && a.SectionCode() == "horta" {
			t.Errorf("SECTION authorization still present after Revoke: %+v", list)
		}
	}
}

func TestBoardAuthorizationService_AuditActorIsAdminEmail(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 9, true)

	if err := svc.Grant(ctx, 9, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	found := false
	for _, a := range audits {
		if a.EntityType() == "BoardAuthorization" && a.EntityID() == "9" {
			if a.ActorEmail() != adminEmail {
				t.Errorf("audit actor email = %q, want %q", a.ActorEmail(), adminEmail)
			}
			if a.Kind() != model.AuditBoardAuthChanged {
				t.Errorf("audit kind = %q, want %q", a.Kind(), model.AuditBoardAuthChanged)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("no audit event found for BoardAuthorization 9")
	}
}

func TestBoardAuthorizationService_RevokeAuditAppended(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 10, true)

	if err := svc.Grant(ctx, 10, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := svc.Revoke(ctx, 10, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	count := 0
	for _, a := range audits {
		if a.EntityType() == "BoardAuthorization" && a.EntityID() == "10" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 audit events (grant + revoke) for partner 10, got %d", count)
	}
}

func TestBoardAuthorizationService_RevokeNonexistentDoesNotAudit(t *testing.T) {
	svc, conn := newBaSvc(t)
	ctx := context.Background()
	seedBoardPartner(t, conn, 11, true)

	// Revoke an authorization that was never granted: no row removed, no audit.
	if err := svc.Revoke(ctx, 11, model.ScopeCommon, ""); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	audits, err := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	if err != nil {
		t.Fatalf("List audits: %v", err)
	}
	for _, a := range audits {
		if a.EntityType() == "BoardAuthorization" && a.EntityID() == "11" {
			t.Errorf("no-op Revoke should not append an audit event, found %+v", a)
		}
	}
}
