package transform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/cmd/adopt/transform"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

const fixture = "../../../testdata/legacy-espigol.db"

func TestRun_AdoptsRealFixture(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping")
	}
	dest := filepath.Join(t.TempDir(), "espigol.db")

	counts, err := transform.Run(context.Background(), fixture, dest)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if counts.Partners != 8 || counts.Forecasts != 35 || counts.Reports != 1 {
		t.Fatalf("counts = %+v", counts)
	}

	conn, err := db.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	q := sqlc.New(conn)
	ctx := context.Background()

	// sections seeded + memberships derived
	secs, _ := persistence.NewSectionRepository(q).List(ctx)
	if len(secs) != 2 {
		t.Errorf("sections = %d, want 2", len(secs))
	}
	mem, _ := persistence.NewSectionRepository(q).ListMembershipsByPartner(ctx, 1)
	if len(mem) != 2 { // partner 1 had olive_section=1, livestock_section=1
		t.Errorf("partner 1 memberships = %d, want 2", len(mem))
	}

	// money exact, scope mapped
	fs, _ := persistence.NewForecastRepository(conn, q).ListByYear(ctx, 2026)
	if len(fs) != 35 {
		t.Errorf("forecasts = %d, want 35", len(fs))
	}
	var sawSection, sawExactReal bool
	for _, f := range fs {
		if f.Scope().Kind() == model.ScopeSection && f.Scope().SectionCode() == "oliva" {
			sawSection = true
		}
		if f.GrossAmount().String() == "1322.22" {
			sawExactReal = true
		}
	}
	if !sawSection {
		t.Error("expected at least one oliva SECTION forecast")
	}
	if !sawExactReal {
		t.Error("expected the former-REAL 1322.22 stored exactly")
	}

	// audit: 61 carried + 1 MIGRATION
	audits, _ := persistence.NewAuditLog(q).List(ctx)
	if len(audits) != 62 {
		t.Errorf("audits = %d, want 62 (61 + MIGRATION)", len(audits))
	}
}

func TestRun_RefusesExistingDest(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping")
	}
	dest := filepath.Join(t.TempDir(), "espigol.db")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Run(context.Background(), fixture, dest); err == nil {
		t.Error("expected Run to refuse an existing destination")
	}
}
