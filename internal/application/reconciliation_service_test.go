package application_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/adapters/system"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

// buildReconWorld seeds a 2025 window + subtype a6 + partner + one forecast and
// returns the tx manager and the forecast id. Reuses the shared newTestTxWorld
// helper convention from window_service_test.go.
func TestReconciliationImport_HappyPathAndWarnings(t *testing.T) {
	world := newReconWorld(t) // helper below
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-02", SubtypeCode: "a6", Concept: "Adob orgànic",
			RequestedTotal: model.MoneyOf(6580), GrantedAmount: model.MoneyOf(6580),
			ForecastIDs: []string{world.forecastID},
		}},
		Invoices: []application.InvoiceInput{{
			Year: 2025, Issuer: "Sup", Nif: "B1", Number: "F1",
			IssueDate: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), NetAmount: model.MoneyOf(500),
			Payments: []application.PaymentInput{{PaidOn: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), Amount: model.MoneyOf(500)}},
			Links:    []application.LinkInput{{ForecastID: world.forecastID, Amount: model.MoneyOf(500)}},
		}},
	}
	res, err := svc.AdminImport(ctx, in)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Concessions != 1 || res.Invoices != 1 {
		t.Fatalf("counts: %+v", res)
	}
	// forecast GrossAmount is 500 but Demanat is 6580 -> soft warning expected.
	if len(res.Warnings) == 0 {
		t.Errorf("expected a Demanat vs Previst warning")
	}

	got, _ := svc.ListConcessions(ctx, 2025)
	if len(got) != 1 {
		t.Fatalf("ListConcessions = %d", len(got))
	}
}

func TestReconciliationImport_UnknownForecastRollsBack(t *testing.T) {
	world := newReconWorld(t)
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-02", SubtypeCode: "a6", Concept: "x",
			RequestedTotal: model.ZeroMoney(), GrantedAmount: model.ZeroMoney(),
			ForecastIDs: []string{"CP25999"}, // does not exist
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err == nil {
		t.Fatal("expected error for unknown forecast")
	}
	got, _ := svc.ListConcessions(ctx, 2025)
	if len(got) != 0 {
		t.Fatalf("rollback failed: %d concessions", len(got))
	}
}

func TestReconciliationImport_UnknownSubtypeFails(t *testing.T) {
	world := newReconWorld(t)
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()
	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "Z9-01", SubtypeCode: "zz", Concept: "x",
			RequestedTotal: model.ZeroMoney(), GrantedAmount: model.ZeroMoney(),
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err == nil {
		t.Fatal("expected error for unknown subtype")
	}
}

type reconWorld struct {
	tx         *persistence.TxManager
	forecastID string
}

func newReconWorld(t *testing.T) reconWorld {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2025, model.WindowClosed, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	typ, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(2025, "a6", "[a6]", "A")
	_ = tax.SaveSubtype(ctx, st)
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p7)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Adob", "d", model.MoneyOf(500), model.ZeroMoney(),
		nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
	f, _ := fr.Create(ctx, uf)

	return reconWorld{tx: persistence.NewTxManager(conn), forecastID: f.ID()}
}

func TestReconciliationService_Compute_HappyPath(t *testing.T) {
	world := newReconWorld(t) // seeds 2025 window, subtype a6, partner 7, one forecast CP25001
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	// Seed a concession + a fully-paid invoice via AdminImport so we exercise
	// the same import path used in production.
	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-01", SubtypeCode: "a6", Concept: "Adob",
			RequestedTotal: model.MoneyOf(500), GrantedAmount: model.MoneyOf(500),
			ForecastIDs:    []string{world.forecastID},
		}},
		Invoices: []application.InvoiceInput{{
			Year: 2025, Issuer: "Sup", Nif: "B1", Number: "F1",
			IssueDate: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), NetAmount: model.MoneyOf(500),
			Payments: []application.PaymentInput{{PaidOn: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), Amount: model.MoneyOf(500)}},
			Links:    []application.LinkInput{{ForecastID: world.forecastID, Amount: model.MoneyOf(500)}},
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err != nil {
		t.Fatalf("seed AdminImport: %v", err)
	}

	got, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("Categories = %d, want 1", len(got.Categories))
	}
	// The world helper builds only subtype a6 (CURRENT).
	if got.Categories[0].Category != model.CategoryCurrent {
		t.Errorf("Category = %v, want CURRENT", got.Categories[0].Category)
	}
	if got.Categories[0].Assigned.String() != "500.00" {
		t.Errorf("Assigned = %s, want 500.00", got.Categories[0].Assigned.String())
	}
}

// newReconWorldWithForecasts seeds a 2025 window + subtype a6 + partner and
// creates forecasts until the given ids exist (CP250nn are allocated in order).
func newReconWorldWithForecasts(t *testing.T, ids ...string) reconWorld {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2025, model.WindowClosed, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	typ, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(2025, "a6", "[a6]", "A")
	_ = tax.SaveSubtype(ctx, st)
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p7)

	// Allocate forecasts CP25001.. until all requested ids are present.
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	have := map[string]bool{}
	for len(have) < len(want) {
		uf, _ := model.NewUnsavedExpenseForecast(p7, "f", "d", model.MoneyOf(6940), model.ZeroMoney(),
			nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
		f, err := fr.Create(ctx, uf)
		if err != nil {
			t.Fatal(err)
		}
		if want[f.ID()] {
			have[f.ID()] = true
		}
		if f.ID() > "CP25099" { // safety stop
			t.Fatalf("could not allocate ids %v (got up to %s)", ids, f.ID())
		}
	}
	return reconWorld{tx: persistence.NewTxManager(conn), forecastID: ids[0]}
}

// TestReconciliation2025Fixture_ComputeMatchesWorkbook drives the end-to-end
// pipeline (importer.LoadReconciliation → AdminImport → Compute) against the
// real 2025 payload in private/export-reconciliation.json (gitignored). Skips
// when the file isn't present (dev machines without the private data).
func TestReconciliation2025Fixture_ComputeMatchesWorkbook(t *testing.T) {
	// Repo root is two directories up from internal/application.
	path := filepath.Join("..", "..", "private", "export-reconciliation.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("private fixture missing: %s", path)
	}

	world := new2025World(t) // helper defined below
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in, err := importer.LoadReconciliation(path, 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	if _, err := svc.AdminImport(ctx, in); err != nil {
		t.Fatalf("AdminImport: %v", err)
	}

	got, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	// Assert per-subtype Executed against workbook figures.
	wantExec := map[string]string{
		"a2": "5989.00", "a3": "0.00", "a4": "1381.11",
		"a6": "18672.09", "b1": "52752.80", "b2": "1460.00",
	}
	haveExec := map[string]string{}
	for _, cat := range got.Categories {
		for _, st := range cat.Subtypes {
			haveExec[st.Code] = st.Executed.String()
		}
	}
	for code, want := range wantExec {
		if got := haveExec[code]; got != want {
			t.Errorf("subtype %s Executed = %s, want %s", code, got, want)
		}
	}

	// Assert grand total Executed = 80255.00.
	total := model.ZeroMoney()
	for _, cat := range got.Categories {
		total = total.Plus(cat.Executed)
	}
	if total.String() != "80255.00" {
		t.Errorf("grand total Executed = %s, want 80255.00", total.String())
	}

	// Every forecast has a defined status (no zero value that isn't
	// intentionally StatusFullyJustified).
	for _, cat := range got.Categories {
		for _, st := range cat.Subtypes {
			for _, cn := range st.Concessions {
				for _, fr := range cn.Forecasts {
					// Any of the 5 declared values is fine; guard against
					// bogus large ints creeping in.
					if fr.Status < services.StatusFullyJustified || fr.Status > services.StatusNoInvoice {
						t.Errorf("forecast %s: invalid status %d", fr.ForecastID, fr.Status)
					}
				}
			}
		}
	}

	// B2-01 Arreglar marges: partially justified per workbook.
	// Granted 1766.12, Executed 1460.00, Assigned 1460.00, PartiallyJustified.
	var b201 *services.ConcessionReconciliation
	for i := range got.Categories {
		for j := range got.Categories[i].Subtypes {
			for k := range got.Categories[i].Subtypes[j].Concessions {
				c := &got.Categories[i].Subtypes[j].Concessions[k]
				if c.GroupCode == "B2-01" {
					b201 = c
				}
			}
		}
	}
	if b201 == nil {
		t.Fatal("B2-01 concession not found in output")
	}
	if b201.Granted.String() != "1766.12" || b201.Executed.String() != "1460.00" || b201.Assigned.String() != "1460.00" {
		t.Errorf("B2-01 mismatch: granted=%s executed=%s assigned=%s",
			b201.Granted, b201.Executed, b201.Assigned)
	}
	if len(b201.Forecasts) != 1 || b201.Forecasts[0].Status != services.StatusPartiallyJustified {
		t.Errorf("B2-01 forecast status = %v, want StatusPartiallyJustified", b201.Forecasts[0].Status)
	}
}

// new2025World seeds a scratch SQLite DB with the 2025 taxonomy (a2/a3/a4/a6/b1/b2
// + their CURRENT/INVESTMENT types), section "oliva", partners 1/2/4/5/6/7/8/11 (per the actual export-forecasts.json fixture),
// an OPEN 2025 window, and 38 forecasts CP25001..CP25038 with the concepts and
// gross amounts that match the workbook. Reads the concepts + gross amounts
// from private/export-forecasts.json.
func new2025World(t *testing.T) reconWorld {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)

	// OPEN 2025 window (required for forecast import).
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	w, _ := model.NewSubmissionWindow(2025, model.WindowOpen, &planned, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)

	// Taxonomy — A (CURRENT), B (INVESTMENT), and 6 subtypes.
	tA, _ := model.NewExpenseType(2025, "A", "Corrents", model.CategoryCurrent)
	tB, _ := model.NewExpenseType(2025, "B", "Inversió", model.CategoryInvestment)
	_ = tax.SaveType(ctx, tA)
	_ = tax.SaveType(ctx, tB)
	for _, code := range []string{"a2", "a3", "a4", "a6"} {
		st, _ := model.NewExpenseSubtype(2025, code, code, "A")
		_ = tax.SaveSubtype(ctx, st)
	}
	for _, code := range []string{"b1", "b2"} {
		st, _ := model.NewExpenseSubtype(2025, code, code, "B")
		_ = tax.SaveSubtype(ctx, st)
	}

	// Section "oliva".
	sec, _ := model.NewSection("oliva", "Secció Oliva", true, 1)
	_ = sr.Save(ctx, sec)

	// Partners 1/2/4/5/6/7/8/11 (derived from export-forecasts.json).
	for _, pid := range []int{1, 2, 4, 5, 6, 7, 8, 11} {
		p, _ := model.NewPartner(pid, fmt.Sprintf("P%d", pid), "S", "V",
			fmt.Sprintf("p%d@e.cat", pid), "6", model.Productor, 1, planned, false)
		_ = pr.Save(ctx, p)
	}

	// Import 38 forecasts from private/export-forecasts.json via the same
	// path production uses.
	fcPath := filepath.Join("..", "..", "private", "export-forecasts.json")
	fs := application.NewForecastService(persistence.NewTxManager(conn), system.SystemClock{})
	entries, err := importer.Load(fcPath, 2025)
	if err != nil {
		t.Fatalf("LoadForecasts: %v", err)
	}
	if _, err := fs.AdminImport(ctx, "admin@espigol.test", 2025, entries); err != nil {
		t.Fatalf("forecast AdminImport: %v", err)
	}

	return reconWorld{tx: persistence.NewTxManager(conn)}
}
