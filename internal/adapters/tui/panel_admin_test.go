package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedDraftTaxonomyYear seeds a DRAFT window for year with one type (A,
// CURRENT) and one subtype (a1) under it.
func seedDraftTaxonomyYear(t *testing.T, q *sqlc.Queries, year int) {
	t.Helper()
	seedWindow(t, q, year, model.WindowDraft)
	tax := persistence.NewTaxonomyRepository(q)
	ctx := context.Background()
	ta, err := model.NewExpenseType(year, "A", "[a]", model.CategoryCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if err := tax.SaveType(ctx, ta); err != nil {
		t.Fatal(err)
	}
	sa, err := model.NewExpenseSubtype(year, "a1", "[a1]", "A")
	if err != nil {
		t.Fatal(err)
	}
	if err := tax.SaveSubtype(ctx, sa); err != nil {
		t.Fatal(err)
	}
}

// --- Tipus i subtipus panel ---

func TestTaxonomyPanel_ListsTypesAndSubtypesForYear(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)

	p := NewTaxonomyPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	if cmd != nil {
		runCmd(t, cmd) // panelInitMsg's load uses p.year==0; no window found, harmless
	}
	p, cmd = p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(taxonomyLoadedMsg)
	p, _ = p.Update(loaded)

	view := p.View(80, 20)
	if !strings.Contains(view, "A") || !strings.Contains(view, "[a]") {
		t.Errorf("View() = %q, want it to contain the seeded type", view)
	}
	if !strings.Contains(view, "a1") || !strings.Contains(view, "[a1]") {
		t.Errorf("View() = %q, want it to contain the seeded subtype", view)
	}
}

func TestTaxonomyPanel_DraftYear_ActionsPresentAndMutationsWork(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)

	p := NewTaxonomyPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(taxonomyLoadedMsg)
	p, _ = p.Update(loaded)

	actions := p.Actions()
	if len(actions) == 0 {
		t.Fatal("expected mutating actions to be present on a DRAFT year")
	}
	var hasNew, hasEdit, hasDelete bool
	for _, a := range actions {
		switch a.Key {
		case "n":
			hasNew = true
		case "e":
			hasEdit = true
		case "d":
			hasDelete = true
		}
	}
	if !hasNew || !hasEdit || !hasDelete {
		t.Errorf("Actions() = %+v, want n/e/d present on DRAFT", actions)
	}

	// "t" opens a new-type form; submit creates a second type.
	_, cmd = p.Update(pKey("t"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	form, ok := modalMsg.modal.(formModal)
	if !ok {
		t.Fatalf("expected formModal, got %T", modalMsg.modal)
	}
	values := map[string]string{
		"Codi":      "B",
		"Etiqueta":  "[b]",
		"Categoria": string(model.CategoryInvestment),
	}
	resultMsg := submitForm(t, form, values)
	msgs := drainMsgs(resultMsg)
	var sawNewType bool
	for _, m := range msgs {
		if tl, ok := m.(taxonomyLoadedMsg); ok {
			p, _ = p.Update(tl)
			for _, item := range tl.items {
				if item.isType && item.typ.Code() == "B" {
					sawNewType = true
				}
			}
		}
	}
	if !sawNewType {
		t.Error("expected the newly created type B to appear in the reloaded list")
	}

	// "d" on the newly created type B deletes it (no subtype references it).
	tp := p.(taxonomyPanel)
	for i, item := range tp.items {
		if item.isType && item.typ.Code() == "B" {
			tp.selected = i
		}
	}
	p = tp
	_, cmd = p.Update(pKey("d"))
	msg = runCmd(t, cmd)
	modalMsg, ok = msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg for delete confirm, got %T", msg)
	}
	confirm, ok := modalMsg.modal.(confirmModal)
	if !ok {
		t.Fatalf("expected confirmModal, got %T", modalMsg.modal)
	}
	_, confirmCmd := confirm.Update(pKey("y"))
	msgs = drainMsgs(runCmd(t, confirmCmd))
	var deleted bool
	for _, m := range msgs {
		if tl, ok := m.(taxonomyLoadedMsg); ok {
			deleted = true
			for _, item := range tl.items {
				if item.isType && item.typ.Code() == "B" {
					t.Error("type B should have been deleted")
				}
			}
		}
	}
	if !deleted {
		t.Error("expected the delete confirm to trigger a reload")
	}
}

func TestTaxonomyPanel_NonDraftYear_ActionsHiddenAndNoticeShown(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2026, model.WindowOpen)
	tax := persistence.NewTaxonomyRepository(q)
	ctx := context.Background()
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)

	p := NewTaxonomyPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(taxonomyLoadedMsg)
	p, _ = p.Update(loaded)

	actions := p.Actions()
	if len(actions) != 0 {
		t.Errorf("Actions() = %+v, want empty (no mutations) on a non-DRAFT (OPEN) year", actions)
	}

	view := p.View(80, 20)
	if !strings.Contains(view, "només editable en esborrany") {
		t.Errorf("View() = %q, want the Catalan DRAFT-only notice", view)
	}
}

func TestTaxonomyPanel_LockedMutationSurfacesError(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)

	p := NewTaxonomyPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(taxonomyLoadedMsg)
	p, _ = p.Update(loaded)

	// Open the year so subsequent direct service calls are locked, then
	// drive the reloadCmd's underlying mutation directly (bypassing the
	// Actions()-gated UI, the way a stale/race keypress might) to assert the
	// error-surfacing convention itself: ErrTaxonomyLocked must show up in
	// Detail() via errDetail, not be swallowed.
	// Opening requires at least one CURRENT and one INVESTMENT type.
	tax := persistence.NewTaxonomyRepository(q)
	tb, err := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	if err != nil {
		t.Fatal(err)
	}
	if err := tax.SaveType(context.Background(), tb); err != nil {
		t.Fatal(err)
	}
	sb, err := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	if err != nil {
		t.Fatal(err)
	}
	if err := tax.SaveSubtype(context.Background(), sb); err != nil {
		t.Fatal(err)
	}
	if err := deps.Windows.Open(context.Background(), 2026); err != nil {
		t.Fatalf("opening year: %v", err)
	}

	tp := p.(taxonomyPanel)
	cmd = tp.reloadCmd(func(ctx context.Context) error {
		return deps.Taxonomy.CreateType(ctx, application.TypeInput{
			Year: 2026, Code: "Z", Label: "[z]", Category: model.CategoryCurrent,
		})
	})
	msg := runCmd(t, cmd)
	tl, ok := msg.(taxonomyLoadedMsg)
	if !ok {
		t.Fatalf("expected taxonomyLoadedMsg, got %T", msg)
	}
	if tl.err == nil || !errors.Is(tl.err, application.ErrTaxonomyLocked) {
		t.Fatalf("expected ErrTaxonomyLocked, got %v", tl.err)
	}
	p, _ = p.Update(tl)
	detail := p.Detail()
	if !strings.Contains(detail, "Error") {
		t.Errorf("Detail() = %q, want it to surface the locked-taxonomy error", detail)
	}
}

// --- Previsions panel ---

func seedForecastPartner(t *testing.T, deps Deps, id int) model.Partner {
	t.Helper()
	p, err := deps.Partners.Create(context.Background(), application.PartnerInput{
		ID: id, Name: "Joan", Surname: "Garcia", VatCode: "12345678A",
		Email: "soci" + string(rune('0'+id)) + "@example.com", Mobile: "600000000",
		PartnerType: model.Productor, RiaNumber: id,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestForecastsPanel_ListsForecastsForYear(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)

	if _, err := deps.Forecasts.AdminCreate(context.Background(), testAdminEmail, 2026, 1, application.ForecastInput{
		Concept: "Llavors", GrossAmount: mustMoneyTUI(t, "150.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		SubtypeCode: "a1", ScopeKind: model.ScopePartner,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	view := p.View(80, 20)
	if !strings.Contains(view, "Llavors") {
		t.Errorf("View() = %q, want it to contain the seeded forecast", view)
	}
}

func TestForecastsPanel_InvalidGrossKeepsFormOpenWithError(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
	form := runCmd(t, cmd).(openModalMsg).modal.(*forecastFormModal)
	form.concept.SetValue("Adobs")
	form.gross.SetValue("not-a-number")
	form.plannedDate.SetValue("2026-04-15")
	// Move to the last field so Enter triggers submit (and validation).
	form.focused = int(fieldPlannedDate)

	var tm tea.Model = form
	updated, submitCmd := tm.Update(pKey("enter"))
	// On a validation error the form does not emit a close/submit command...
	if submitCmd != nil {
		t.Fatalf("expected nil cmd (form stays open) on invalid gross, got a command")
	}
	// ...and the inline error is visible.
	if fm := updated.(*forecastFormModal); !strings.Contains(fm.View(), "Import brut no vàlid") {
		t.Errorf("expected an inline gross-validation error in the form view; got:\n%s", fm.View())
	}
}

func TestForecastsPanel_CreateViaFormCallsAdminCreate(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	form, ok := modalMsg.modal.(*forecastFormModal)
	if !ok {
		t.Fatalf("expected *forecastFormModal, got %T", modalMsg.modal)
	}
	form.concept.SetValue("Adobs")
	form.description.SetValue("Adobs de primavera")
	form.gross.SetValue("250.00")
	form.plannedDate.SetValue("2026-04-15")
	// Move to the last field so Enter triggers submit.
	form.focused = int(fieldPlannedDate)

	var tm tea.Model = form
	_, submitCmd := tm.Update(pKey("enter"))
	if submitCmd == nil {
		t.Fatal("expected a non-nil cmd from submitting the forecast form")
	}
	msgs := drainMsgs(submitCmd())
	var sawCreated bool
	for _, m := range msgs {
		if fl, ok := m.(forecastsLoadedMsg); ok {
			if fl.err != nil {
				t.Fatalf("unexpected error from AdminCreate: %v", fl.err)
			}
			for _, f := range fl.forecasts {
				if f.Concept() == "Adobs" && f.Partner().ID() == 1 {
					sawCreated = true
				}
			}
		}
	}
	if !sawCreated {
		t.Error("expected the newly created forecast (via AdminCreate) to appear in the reloaded list")
	}
}

func TestForecastsPanel_CreateOnClosedYearSurfacesWindowNotEditable(t *testing.T) {
	deps, q := testDeps(t)
	seedForecastPartner(t, deps, 1)
	seedWindow(t, q, 2026, model.WindowClosed)

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	fp := p.(forecastsPanel)
	cmd = fp.reloadCmd(func(ctx context.Context) error {
		_, err := deps.Forecasts.AdminCreate(ctx, testAdminEmail, 2026, 1, application.ForecastInput{
			Concept: "Tard", GrossAmount: mustMoneyTUI(t, "10.00"),
			PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			SubtypeCode: "a1", ScopeKind: model.ScopePartner,
		})
		return err
	})
	msg := runCmd(t, cmd)
	fl, ok := msg.(forecastsLoadedMsg)
	if !ok {
		t.Fatalf("expected forecastsLoadedMsg, got %T", msg)
	}
	if fl.err == nil || !errors.Is(fl.err, application.ErrWindowNotEditable) {
		t.Fatalf("expected ErrWindowNotEditable, got %v", fl.err)
	}
}

func TestForecastsPanel_DeleteConfirmCallsAdminDelete(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)
	f, err := deps.Forecasts.AdminCreate(context.Background(), testAdminEmail, 2026, 1, application.ForecastInput{
		Concept: "Esborrar", GrossAmount: mustMoneyTUI(t, "50.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		SubtypeCode: "a1", ScopeKind: model.ScopePartner,
	})
	if err != nil {
		t.Fatal(err)
	}

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("d"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	confirm, ok := modalMsg.modal.(confirmModal)
	if !ok {
		t.Fatalf("expected confirmModal, got %T", modalMsg.modal)
	}
	_, confirmCmd := confirm.Update(pKey("y"))
	msgs := drainMsgs(runCmd(t, confirmCmd))
	var sawDeleted bool
	for _, m := range msgs {
		if fl, ok := m.(forecastsLoadedMsg); ok {
			sawDeleted = true
			for _, fc := range fl.forecasts {
				if fc.ID() == f.ID() {
					t.Error("forecast should have been deleted")
				}
			}
		}
	}
	if !sawDeleted {
		t.Error("expected the delete confirm to trigger a reload")
	}
}

func mustMoneyTUI(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// --- Admin panel ---

func TestAdminPanel_ClosedYear_GeneratesViaExportAndFilesExist(t *testing.T) {
	deps, q := testDeps(t)
	ctx := context.Background()

	// Build a CLOSED 2026 year with a stored Report via the real Close flow
	// (CreateYear -> seed taxonomy -> Open -> Close), matching the
	// WindowService's own test conventions.
	seedWindow(t, q, 2025, model.WindowClosed)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2025, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2025, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	if _, err := deps.Windows.CreateYear(ctx, 2027); err != nil {
		t.Fatal(err)
	}
	if err := deps.Windows.Open(ctx, 2027); err != nil {
		t.Fatal(err)
	}
	if _, err := deps.Windows.Close(ctx, 2027); err != nil {
		t.Fatal(err)
	}

	p := NewAdminPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2027})
	if cmd != nil {
		runCmd(t, cmd)
	}

	_, cmd = p.Update(pKey("f"))
	msg := runCmd(t, cmd)
	wsMsg, ok := msg.(windowStateMsg)
	if !ok {
		t.Fatalf("expected windowStateMsg, got %T", msg)
	}
	if wsMsg.state != model.WindowClosed {
		t.Fatalf("expected CLOSED state, got %s", wsMsg.state)
	}
	p, cmd = p.Update(wsMsg)
	if cmd == nil {
		t.Fatal("expected generateReportCmd after resolving CLOSED state")
	}
	doneMsg := runCmd(t, cmd).(reportDoneMsg)
	if doneMsg.err != nil {
		t.Fatalf("generateReportCmd error: %v", doneMsg.err)
	}
	p, _ = p.Update(doneMsg)

	outputDir := deps.Cfg.OutputDir
	for _, name := range []string{"Previsions de despeses 2027.pdf", "Previsions de despeses 2027.md"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	detail := p.Detail()
	if !strings.Contains(detail, "2027") && !strings.Contains(detail, "pdf") {
		t.Errorf("Detail() = %q, want it to show the written paths", detail)
	}
}

func TestAdminPanel_OpenYear_GeneratesViaExportDataAndFilesExist(t *testing.T) {
	deps, q := testDeps(t)
	ctx := context.Background()
	seedWindow(t, q, 2026, model.WindowOpen)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, sa)

	p := NewAdminPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	if cmd != nil {
		runCmd(t, cmd)
	}

	_, cmd = p.Update(pKey("f"))
	msg := runCmd(t, cmd)
	wsMsg, ok := msg.(windowStateMsg)
	if !ok {
		t.Fatalf("expected windowStateMsg, got %T", msg)
	}
	if wsMsg.state != model.WindowOpen {
		t.Fatalf("expected OPEN state, got %s", wsMsg.state)
	}
	p, cmd = p.Update(wsMsg)
	if cmd == nil {
		t.Fatal("expected generateReportCmd after resolving OPEN state")
	}
	doneMsg := runCmd(t, cmd).(reportDoneMsg)
	if doneMsg.err != nil {
		t.Fatalf("generateReportCmd error: %v", doneMsg.err)
	}
	p, _ = p.Update(doneMsg)

	outputDir := deps.Cfg.OutputDir
	for _, name := range []string{"Previsions de despeses 2026.pdf", "Previsions de despeses 2026.md"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}

func TestAdminPanel_Import_CreatesForecasts(t *testing.T) {
	deps, q := testDeps(t)
	ctx := context.Background()

	// Seed an OPEN 2025 year with taxonomy a1 and partner 7.
	seedWindow(t, q, 2025, model.WindowOpen)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, sa)
	p7, _ := model.NewPartner(7, "Soci", "", "", "s7@e.test", "", model.Productor, 0,
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = persistence.NewPartnerRepository(q).Save(ctx, p7)

	// Write the import file into Cfg.ImportDir.
	if err := os.MkdirAll(deps.Cfg.ImportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"year":2025,"forecasts":[
	  {"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"Assegurança","grossAmount":"2880.00","plannedDate":"2025-06-15"},
	  {"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"Segona","grossAmount":"100.00","plannedDate":"2025-07-01"}
	]}`
	if err := os.WriteFile(filepath.Join(deps.Cfg.ImportDir, "2025-forecasts.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	p := NewAdminPanel(deps)
	p, _ = p.Update(yearSelectedMsg{Year: 2025})
	_, cmd := p.Update(pKey("i"))
	msg := runCmd(t, cmd).(forecastsImportedMsg)
	if msg.err != nil {
		t.Fatalf("import error: %v", msg.err)
	}
	if msg.result.Created != 2 {
		t.Errorf("Created = %d, want 2", msg.result.Created)
	}
	p, _ = p.Update(msg)
	if got := p.Detail(); !strings.Contains(got, "Importats 2") {
		t.Errorf("Detail = %q, want it to mention Importats 2", got)
	}
}

func TestAdminPanel_Import_ClosedYearSurfacesError(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2025, model.WindowDraft) // not OPEN
	if err := os.MkdirAll(deps.Cfg.ImportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"year":2025,"forecasts":[{"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"1.00","plannedDate":"2025-06-15"}]}`
	if err := os.WriteFile(filepath.Join(deps.Cfg.ImportDir, "2025-forecasts.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewAdminPanel(deps)
	p, _ = p.Update(yearSelectedMsg{Year: 2025})
	_, cmd := p.Update(pKey("i"))
	msg := runCmd(t, cmd).(forecastsImportedMsg)
	if msg.err == nil {
		t.Fatal("expected error importing into a non-OPEN year")
	}
}

func TestAdminPanel_Backup_CreatesFileAndShowsPath(t *testing.T) {
	deps, _ := testDeps(t)
	p := NewAdminPanel(deps)
	_, cmd := p.Update(pKey("b"))
	msg := runCmd(t, cmd).(backupDoneMsg)
	if msg.err != nil {
		t.Fatalf("backup error: %v", msg.err)
	}
	if _, err := os.Stat(msg.path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	p, _ = p.Update(msg)
	if got := p.Detail(); !strings.Contains(got, msg.path) {
		t.Errorf("Detail = %q, want it to contain the backup path", got)
	}
}
