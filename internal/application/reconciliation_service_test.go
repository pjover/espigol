package application_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
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
