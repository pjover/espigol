// Package transform adopts the legacy espigol-java SQLite database into the new
// espigol schema using a single all-or-nothing transaction.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pjover/espigol/cmd/adopt/legacy"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

// Counts holds the number of rows inserted per table.
type Counts struct {
	Partners    int
	Sections    int
	Memberships int
	Types       int
	Subtypes    int
	Windows     int
	Forecasts   int
	Reports     int
	Audits      int
}

// Run transforms the legacy DB at legacyPath into a new espigol DB at destPath.
// It refuses if destPath already exists, runs one transaction, validates counts,
// and appends a MIGRATION audit event on success.
func Run(ctx context.Context, legacyPath, destPath string) (Counts, error) {
	// 1. Refuse if destination already exists.
	if _, err := os.Stat(destPath); err == nil {
		return Counts{}, fmt.Errorf("destination %q already exists; remove it or use --force", destPath)
	}

	// 2. Read the legacy dump.
	d, err := legacy.Read(legacyPath)
	if err != nil {
		return Counts{}, fmt.Errorf("reading legacy DB: %w", err)
	}

	// 3. Open (creates + migrates) the new DB.
	conn, err := db.Open(destPath)
	if err != nil {
		return Counts{}, fmt.Errorf("opening new DB: %w", err)
	}
	defer conn.Close()

	// 4. One transaction.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Counts{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	q := sqlc.New(conn).WithTx(tx)
	secRepo := persistence.NewSectionRepository(q)
	winRepo := persistence.NewWindowRepository(q)
	taxRepo := persistence.NewTaxonomyRepository(q)
	partRepo := persistence.NewPartnerRepository(q)
	fcRepo := persistence.NewForecastRepository(conn, q)
	repRepo := persistence.NewReportRepository(q)
	auditLog := persistence.NewAuditLog(q)

	counts := Counts{}

	// 5. Seed sections.
	oliva, err := model.NewSection("oliva", "Secció d'oliva", true, 1)
	if err != nil {
		return Counts{}, fmt.Errorf("building oliva section: %w", err)
	}
	if err := secRepo.Save(ctx, oliva); err != nil {
		return Counts{}, fmt.Errorf("saving oliva section: %w", err)
	}
	ramaderia, err := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	if err != nil {
		return Counts{}, fmt.Errorf("building ramaderia section: %w", err)
	}
	if err := secRepo.Save(ctx, ramaderia); err != nil {
		return Counts{}, fmt.Errorf("saving ramaderia section: %w", err)
	}
	counts.Sections = 2

	// 6. Insert windows.
	for _, lw := range d.Windows {
		state, err := model.ParseWindowState(lw.State)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing window state %q: %w", lw.State, err)
		}
		currentLimit, err := model.MoneyFromString(lw.CurrentLimit)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing current limit %q: %w", lw.CurrentLimit, err)
		}
		investmentLimit, err := model.MoneyFromString(lw.InvestmentLimit)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing investment limit %q: %w", lw.InvestmentLimit, err)
		}
		w, err := model.NewSubmissionWindow(lw.Year, state, lw.OpenedAt, lw.ClosedAt,
			lw.Deadline, currentLimit, investmentLimit)
		if err != nil {
			return Counts{}, fmt.Errorf("building window year %d: %w", lw.Year, err)
		}
		if err := winRepo.Save(ctx, w); err != nil {
			return Counts{}, fmt.Errorf("saving window year %d: %w", lw.Year, err)
		}
		counts.Windows++
	}

	// 7. Insert types.
	for _, lt := range d.Types {
		cat, err := model.ParseExpenseCategory(lt.Category)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing category %q: %w", lt.Category, err)
		}
		t, err := model.NewExpenseType(lt.Year, lt.Code, lt.Label, cat)
		if err != nil {
			return Counts{}, fmt.Errorf("building type %s: %w", lt.Code, err)
		}
		if err := taxRepo.SaveType(ctx, t); err != nil {
			return Counts{}, fmt.Errorf("saving type %s: %w", lt.Code, err)
		}
		counts.Types++
	}

	// Insert subtypes.
	for _, ls := range d.Subtypes {
		s, err := model.NewExpenseSubtype(ls.Year, ls.Code, ls.Label, ls.TypeCode)
		if err != nil {
			return Counts{}, fmt.Errorf("building subtype %s: %w", ls.Code, err)
		}
		if err := taxRepo.SaveSubtype(ctx, s); err != nil {
			return Counts{}, fmt.Errorf("saving subtype %s: %w", ls.Code, err)
		}
		counts.Subtypes++
	}

	// 8. Insert partners; add memberships.
	for _, lp := range d.Partners {
		pt, err := model.ParsePartnerType(lp.PartnerType)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing partner type %q: %w", lp.PartnerType, err)
		}
		p, err := model.NewPartner(lp.ID, lp.Name, lp.Name, lp.Surname, lp.VatCode, lp.Email, lp.Mobile,
			pt, lp.RiaNumber, lp.AddedOn, lp.BoardMember)
		if err != nil {
			return Counts{}, fmt.Errorf("building partner %d: %w", lp.ID, err)
		}
		if err := partRepo.Save(ctx, p); err != nil {
			return Counts{}, fmt.Errorf("saving partner %d: %w", lp.ID, err)
		}
		counts.Partners++

		if lp.OliveSection {
			m, err := model.NewPartnerSection(lp.ID, "oliva")
			if err != nil {
				return Counts{}, fmt.Errorf("building oliva membership for partner %d: %w", lp.ID, err)
			}
			if err := secRepo.AddMembership(ctx, m); err != nil {
				return Counts{}, fmt.Errorf("adding oliva membership for partner %d: %w", lp.ID, err)
			}
			counts.Memberships++
		}
		if lp.LivestockSection {
			m, err := model.NewPartnerSection(lp.ID, "ramaderia")
			if err != nil {
				return Counts{}, fmt.Errorf("building ramaderia membership for partner %d: %w", lp.ID, err)
			}
			if err := secRepo.AddMembership(ctx, m); err != nil {
				return Counts{}, fmt.Errorf("adding ramaderia membership for partner %d: %w", lp.ID, err)
			}
			counts.Memberships++
		}
	}

	// 9. Insert forecasts via InsertWithID (keep existing CPYYnnn ids).
	for _, lf := range d.Forecasts {
		gross, err := model.MoneyFromString(lf.GrossAmount)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing gross for %s: %w", lf.ID, err)
		}
		approved, err := model.MoneyFromString(lf.ApprovedAmount)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing approved for %s: %w", lf.ID, err)
		}
		scope, err := mapScope(lf.Scope)
		if err != nil {
			return Counts{}, fmt.Errorf("mapping scope for %s: %w", lf.ID, err)
		}
		partner, ok, err := partRepo.FindByID(ctx, lf.PartnerID)
		if err != nil {
			return Counts{}, fmt.Errorf("finding partner for %s: %w", lf.ID, err)
		}
		if !ok {
			return Counts{}, fmt.Errorf("partner %d not found for forecast %s", lf.PartnerID, lf.ID)
		}
		f, err := model.NewExpenseForecast(lf.ID, partner, lf.Concept, lf.Description,
			gross, approved, lf.ApprovedOn, lf.PlannedDate, lf.Year, lf.SubtypeCode, scope,
			lf.AddedOn, lf.Enabled)
		if err != nil {
			return Counts{}, fmt.Errorf("building forecast %s: %w", lf.ID, err)
		}
		if err := fcRepo.InsertWithID(ctx, f); err != nil {
			return Counts{}, fmt.Errorf("inserting forecast %s: %w", lf.ID, err)
		}
		counts.Forecasts++
	}

	// 10. Insert reports (new autoincrement ids are fine).
	for _, lr := range d.Reports {
		rep, err := model.NewReport(lr.ID, lr.Year, lr.GeneratedAt, lr.SnapshotJSON, lr.Pdf, lr.SupersededAt)
		if err != nil {
			return Counts{}, fmt.Errorf("building report %d: %w", lr.ID, err)
		}
		if _, err := repRepo.Insert(ctx, rep); err != nil {
			return Counts{}, fmt.Errorf("inserting report %d: %w", lr.ID, err)
		}
		counts.Reports++
	}

	// 11. Append all legacy audit events.
	for _, la := range d.Audits {
		kind, err := model.ParseAuditKind(la.Kind)
		if err != nil {
			return Counts{}, fmt.Errorf("parsing audit kind %q: %w", la.Kind, err)
		}
		e, err := model.NewAuditEvent(la.ID, la.ActorID, la.ActorEmail, kind,
			la.EntityType, la.EntityID, la.Timestamp, la.Payload)
		if err != nil {
			return Counts{}, fmt.Errorf("building audit event %d: %w", la.ID, err)
		}
		if err := auditLog.Append(ctx, e); err != nil {
			return Counts{}, fmt.Errorf("appending audit event %d: %w", la.ID, err)
		}
		counts.Audits++
	}

	// 12. Validate counts against source dump.
	if err := validateCounts(counts, d); err != nil {
		return Counts{}, err
	}

	// 13. Append MIGRATION audit event.
	payload, err := json.Marshal(counts)
	if err != nil {
		return Counts{}, fmt.Errorf("marshalling migration payload: %w", err)
	}
	payloadStr := string(payload)
	migEvent, err := model.NewAuditEvent(0, nil, "system@espigol", model.AuditMigration,
		"Database", "adopt", time.Now().UTC(), &payloadStr)
	if err != nil {
		return Counts{}, fmt.Errorf("building migration audit event: %w", err)
	}
	if err := auditLog.Append(ctx, migEvent); err != nil {
		return Counts{}, fmt.Errorf("appending migration audit event: %w", err)
	}
	counts.Audits++

	// 14. Commit.
	if err := tx.Commit(); err != nil {
		return Counts{}, fmt.Errorf("committing transaction: %w", err)
	}
	return counts, nil
}

// validateCounts checks that inserted rows match the source dump lengths.
func validateCounts(c Counts, d *legacy.Dump) error {
	if c.Partners != len(d.Partners) {
		return fmt.Errorf("partners count mismatch: inserted %d, source %d", c.Partners, len(d.Partners))
	}
	// Always exactly 2 sections (oliva + ramaderia) are seeded.
	if c.Sections != 2 {
		return fmt.Errorf("sections count mismatch: inserted %d, want 2", c.Sections)
	}
	// Memberships = sum of OliveSection + LivestockSection booleans across all partners.
	wantMemberships := 0
	for _, lp := range d.Partners {
		if lp.OliveSection {
			wantMemberships++
		}
		if lp.LivestockSection {
			wantMemberships++
		}
	}
	if c.Memberships != wantMemberships {
		return fmt.Errorf("memberships count mismatch: inserted %d, source %d", c.Memberships, wantMemberships)
	}
	if c.Windows != len(d.Windows) {
		return fmt.Errorf("windows count mismatch: inserted %d, source %d", c.Windows, len(d.Windows))
	}
	if c.Types != len(d.Types) {
		return fmt.Errorf("types count mismatch: inserted %d, source %d", c.Types, len(d.Types))
	}
	if c.Subtypes != len(d.Subtypes) {
		return fmt.Errorf("subtypes count mismatch: inserted %d, source %d", c.Subtypes, len(d.Subtypes))
	}
	if c.Forecasts != len(d.Forecasts) {
		return fmt.Errorf("forecasts count mismatch: inserted %d, source %d", c.Forecasts, len(d.Forecasts))
	}
	if c.Reports != len(d.Reports) {
		return fmt.Errorf("reports count mismatch: inserted %d, source %d", c.Reports, len(d.Reports))
	}
	if c.Audits != len(d.Audits) {
		return fmt.Errorf("audits count mismatch: inserted %d, source %d", c.Audits, len(d.Audits))
	}
	return nil
}

// mapScope maps a legacy Catalan scope string to a domain ExpenseScope.
func mapScope(catalan string) (model.ExpenseScope, error) {
	switch catalan {
	case "Comú":
		return model.NewCommonScope(), nil
	case "Soci":
		return model.NewPartnerScope(), nil
	case "Secció d'oliva":
		return model.NewSectionScope("oliva")
	case "Secció de ramaderia":
		return model.NewSectionScope("ramaderia")
	default:
		return model.ExpenseScope{}, fmt.Errorf("unknown legacy scope %q", catalan)
	}
}
