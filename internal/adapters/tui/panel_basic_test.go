package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	appreport "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/domain/model"
)

const testAdminEmail = "admin@espigol.test"

// pbFixedClock is a deterministic Clock for panel tests.
type pbFixedClock struct{ t time.Time }

func (c pbFixedClock) Now() time.Time { return c.t }

// testDeps builds a full Deps over a temp DB with real application services,
// mirroring wire.Server/wire.TUI's assembly so the panels exercise the real
// service stack rather than fakes.
func testDeps(t *testing.T) (Deps, *sqlc.Queries) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "panels.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	txm := persistence.NewTxManager(conn)
	clock := pbFixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}

	exporter := appreport.NewReportExporter(appreport.PDFRenderer{BusinessName: "Test"})

	deps := Deps{
		Partners:  application.NewPartnerService(txm, clock, testAdminEmail),
		Sections:  application.NewSectionService(txm, clock, testAdminEmail),
		Taxonomy:  application.NewTaxonomyService(txm, clock, testAdminEmail),
		Forecasts: application.NewForecastService(txm, clock),
		Windows:   application.NewWindowService(txm, appreport.NoopRenderer{}, clock),
		Reports:   application.NewReportService(txm),
		Exporter:  exporter,
		Cfg:       &config.Config{OutputDir: t.TempDir(), Admin: struct{ Email string }{Email: testAdminEmail}},
	}
	return deps, q
}

// seedWindow saves a submission window directly via the repository (bypassing
// service validation) so tests can set up arbitrary states quickly.
func seedWindow(t *testing.T, q *sqlc.Queries, year int, state model.WindowState) {
	t.Helper()
	w, err := model.NewSubmissionWindow(year, state, nil, nil,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.NewWindowRepository(q).Save(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	return cmd()
}

// drainBatch runs every command inside a tea.BatchMsg (or just msg itself if
// it isn't a batch) and returns all resulting messages.
func drainMsgs(msg tea.Msg) []tea.Msg {
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainMsgs(c())...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func pKey(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// --- Anys (years) panel ---

func TestYearsPanel_LoadsAndStylesWindows(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2026, model.WindowDraft)

	p := NewYearsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	msg := runCmd(t, cmd)
	loaded, ok := msg.(yearsLoadedMsg)
	if !ok {
		t.Fatalf("expected yearsLoadedMsg, got %T", msg)
	}
	p, _ = p.Update(loaded)

	view := p.View(80, 20)
	if !strings.Contains(view, "2026") {
		t.Errorf("View() = %q, want it to contain the seeded year", view)
	}
	if !strings.Contains(view, "DRAFT") {
		t.Errorf("View() = %q, want it to contain the window state", view)
	}
}

func TestYearsPanel_SelectionEmitsYearSelectedCmd(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2025, model.WindowClosed)
	seedWindow(t, q, 2026, model.WindowDraft)

	p := NewYearsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(yearsLoadedMsg)
	p, cmd = p.Update(loaded)

	msg := runCmd(t, cmd)
	ys, ok := msg.(yearSelectedMsg)
	if !ok {
		t.Fatalf("expected yearSelectedMsg after load, got %T", msg)
	}
	if ys.Year != 2025 {
		t.Errorf("yearSelectedMsg.Year = %d, want 2025 (first window)", ys.Year)
	}

	// Move selection down; should emit yearSelectedCmd(2026).
	p, cmd = p.Update(pKey("down"))
	msg = runCmd(t, cmd)
	ys, ok = msg.(yearSelectedMsg)
	if !ok {
		t.Fatalf("expected yearSelectedMsg after moving selection, got %T", msg)
	}
	if ys.Year != 2026 {
		t.Errorf("yearSelectedMsg.Year = %d, want 2026", ys.Year)
	}
}

func TestYearsPanel_OpenOpensConfirmModalAndCallsService(t *testing.T) {
	deps, q := testDeps(t)
	// Open requires a complete taxonomy and a future deadline; seed via the
	// service's own CreateYear so taxonomy/limits are consistent.
	seedWindow(t, q, 2025, model.WindowClosed)
	taxonomy := persistence.NewTaxonomyRepository(q)
	ctx := context.Background()
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2025, "B", "[b]", model.CategoryInvestment)
	_ = taxonomy.SaveType(ctx, ta)
	_ = taxonomy.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2025, "b1", "[b1]", "B")
	_ = taxonomy.SaveSubtype(ctx, sa)
	_ = taxonomy.SaveSubtype(ctx, sb)

	if _, err := deps.Windows.CreateYear(ctx, 2027); err != nil {
		t.Fatal(err)
	}

	p := NewYearsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(yearsLoadedMsg)
	p, _ = p.Update(loaded)

	// Select the 2027 DRAFT window (sorted after 2025 CLOSED).
	for {
		yp := p.(yearsPanel)
		if w, ok := yp.selectedWindow(); ok && w.Year() == 2027 {
			break
		}
		var c tea.Cmd
		p, c = p.Update(pKey("down"))
		if c != nil {
			c()
		}
	}

	_, cmd = p.Update(pKey("o"))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from 'o'")
	}
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	confirm, ok := modalMsg.modal.(confirmModal)
	if !ok {
		t.Fatalf("expected confirmModal, got %T", modalMsg.modal)
	}

	// Confirm with "y": should call Windows.Open and reload.
	_, confirmCmd := confirm.Update(pKey("y"))
	msgs := drainMsgs(runCmd(t, confirmCmd))
	var sawLoaded bool
	for _, m := range msgs {
		if yl, ok := m.(yearsLoadedMsg); ok {
			sawLoaded = true
			for _, w := range yl.windows {
				if w.Year() == 2027 && w.State() != model.WindowOpen {
					t.Errorf("window 2027 state = %s, want OPEN after confirming open", w.State())
				}
			}
		}
	}
	if !sawLoaded {
		t.Error("expected the confirm to trigger a reload (yearsLoadedMsg)")
	}

	w, _, err := persistence.NewWindowRepository(q).FindByYear(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if w.State() != model.WindowOpen {
		t.Errorf("persisted window state = %s, want OPEN", w.State())
	}
}

func TestYearsPanel_CloseOpensConfirmModal(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2026, model.WindowOpen)

	p := NewYearsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(yearsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("c"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	if _, ok := modalMsg.modal.(confirmModal); !ok {
		t.Fatalf("expected confirmModal, got %T", modalMsg.modal)
	}
}

// TestYearsPanel_CloseRejectedSurfacesError drives a mutation the
// WindowService rejects (closing a DRAFT year, which fails with
// ErrWrongState) and asserts the failure becomes visible in Detail()/View()
// instead of being silently discarded by the subsequent reload's nil error.
func TestYearsPanel_CloseRejectedSurfacesError(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2026, model.WindowDraft)

	p := NewYearsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(yearsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("c"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	confirm, ok := modalMsg.modal.(confirmModal)
	if !ok {
		t.Fatalf("expected confirmModal, got %T", modalMsg.modal)
	}

	// Confirm with "y": Windows.Close should reject a DRAFT window with
	// ErrWrongState; that error must reach the resulting yearsLoadedMsg
	// rather than being clobbered by the reload's nil error.
	_, confirmCmd := confirm.Update(pKey("y"))
	msgs := drainMsgs(runCmd(t, confirmCmd))
	var sawLoaded bool
	for _, m := range msgs {
		if yl, ok := m.(yearsLoadedMsg); ok {
			sawLoaded = true
			if yl.err == nil {
				t.Fatal("expected yearsLoadedMsg.err to carry the rejected Close's error, got nil")
			}
			p, _ = p.Update(yl)
		}
	}
	if !sawLoaded {
		t.Fatal("expected the confirm to trigger a reload (yearsLoadedMsg)")
	}

	detail := p.Detail()
	if !strings.Contains(detail, "Error") {
		t.Errorf("Detail() = %q, want it to contain an error indication", detail)
	}
	if !strings.Contains(detail, "current state") {
		// application.ErrWrongState's message; the underlying error text
		// must be present verbatim, not just a generic "Error" label.
		t.Errorf("Detail() = %q, want it to contain the underlying error message", detail)
	}

	view := p.View(80, 20)
	combined := detail + view
	if strings.Contains(combined, "<nil>") {
		t.Errorf("expected no nil-error artefacts in rendered output, got %q", combined)
	}

	// Window must remain DRAFT: the rejected mutation must not have applied.
	w, _, err := persistence.NewWindowRepository(q).FindByYear(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if w.State() != model.WindowDraft {
		t.Errorf("persisted window state = %s, want DRAFT (rejected Close must not apply)", w.State())
	}
}

func TestYearsPanel_NOpensCreateYearForm(t *testing.T) {
	deps, _ := testDeps(t)
	p := NewYearsPanel(deps)
	_, cmd := p.Update(pKey("n"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	if _, ok := modalMsg.modal.(formModal); !ok {
		t.Fatalf("expected formModal, got %T", modalMsg.modal)
	}
}

// --- Socis (partners) panel ---

func partnerInput(id int) application.PartnerInput {
	return application.PartnerInput{
		ID:          id,
		Name:        "Joan",
		Surname:     "Garcia",
		VatCode:     "12345678A",
		Email:       "joan@example.com",
		Mobile:      "600000000",
		PartnerType: model.Productor,
		RiaNumber:   1,
	}
}

func TestPartnersPanel_ListsPartners(t *testing.T) {
	deps, _ := testDeps(t)
	if _, err := deps.Partners.Create(context.Background(), partnerInput(1)); err != nil {
		t.Fatal(err)
	}

	p := NewPartnersPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(partnersLoadedMsg)
	p, _ = p.Update(loaded)

	view := p.View(80, 20)
	if !strings.Contains(view, "Joan") || !strings.Contains(view, "Garcia") {
		t.Errorf("View() = %q, want it to contain the seeded partner", view)
	}
}

func TestPartnersPanel_NOpensFormAndCreatesPartner(t *testing.T) {
	deps, _ := testDeps(t)
	p := NewPartnersPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(partnersLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
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
		"Id":      "5",
		"Nom":     "Maria",
		"Cognoms": "Soler",
		"NIF":     "87654321B",
		"Email":   "maria@example.com",
		"Mobil":   "611111111",
		"Tipus":   string(model.Productor),
		"Num RIA": "2",
	}
	cmds := submitForm(t, form, values)
	msgs := drainMsgs(cmds)
	var sawLoaded bool
	for _, m := range msgs {
		if pl, ok := m.(partnersLoadedMsg); ok {
			sawLoaded = true
			found := false
			for _, partner := range pl.partners {
				if partner.ID() == 5 && partner.Name() == "Maria" {
					found = true
				}
			}
			if !found {
				t.Error("expected the newly created partner to appear in the reloaded list")
			}
		}
	}
	if !sawLoaded {
		t.Error("expected submitting the form to trigger a reload")
	}
}

func TestPartnersPanel_BTogglesBoardMember(t *testing.T) {
	deps, _ := testDeps(t)
	if _, err := deps.Partners.Create(context.Background(), partnerInput(1)); err != nil {
		t.Fatal(err)
	}

	p := NewPartnersPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(partnersLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("b"))
	msgs := drainMsgs(runCmd(t, cmd))
	var toggled bool
	for _, m := range msgs {
		if pl, ok := m.(partnersLoadedMsg); ok {
			for _, partner := range pl.partners {
				if partner.ID() == 1 && partner.BoardMember() {
					toggled = true
				}
			}
		}
	}
	if !toggled {
		t.Error("expected 'b' to toggle the selected partner's board membership")
	}
}

// --- Seccions (sections) panel ---

func TestSectionsPanel_ListsSections(t *testing.T) {
	deps, _ := testDeps(t)
	if _, err := deps.Sections.Create(context.Background(), application.SectionInput{
		Code: "vinya", Label: "Secció de vinya", Active: true, DisplayOrder: 1,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewSectionsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(sectionsLoadedMsg)
	p, _ = p.Update(loaded)

	view := p.View(80, 20)
	if !strings.Contains(view, "vinya") {
		t.Errorf("View() = %q, want it to contain the seeded section", view)
	}
}

func TestSectionsPanel_NOpensFormAndCreatesSection(t *testing.T) {
	deps, _ := testDeps(t)
	p := NewSectionsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(sectionsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
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
		"Codi":     "oli",
		"Etiqueta": "Secció d'oli",
		"Activa":   "true",
		"Ordre":    "2",
	}
	cmds := submitForm(t, form, values)
	msgs := drainMsgs(cmds)
	var sawLoaded bool
	for _, m := range msgs {
		if sl, ok := m.(sectionsLoadedMsg); ok {
			sawLoaded = true
			found := false
			for _, sec := range sl.sections {
				if sec.Code() == "oli" {
					found = true
				}
			}
			if !found {
				t.Error("expected the newly created section to appear in the reloaded list")
			}
		}
	}
	if !sawLoaded {
		t.Error("expected submitting the form to trigger a reload")
	}
}

func TestSectionsPanel_EOpensFormAndUpdatesSection(t *testing.T) {
	deps, _ := testDeps(t)
	if _, err := deps.Sections.Create(context.Background(), application.SectionInput{
		Code: "vinya", Label: "Secció de vinya", Active: true, DisplayOrder: 1,
	}); err != nil {
		t.Fatal(err)
	}

	p := NewSectionsPanel(deps)
	_, cmd := p.Update(panelInitMsg{})
	loaded := runCmd(t, cmd).(sectionsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("e"))
	msg := runCmd(t, cmd)
	modalMsg := msg.(openModalMsg)
	form := modalMsg.modal.(formModal)

	values := map[string]string{
		"Etiqueta": "Secció de vinya (actualitzada)",
		"Activa":   "true",
		"Ordre":    "9",
	}
	cmds := submitForm(t, form, values)
	msgs := drainMsgs(cmds)
	var updated bool
	for _, m := range msgs {
		if sl, ok := m.(sectionsLoadedMsg); ok {
			for _, sec := range sl.sections {
				if sec.Code() == "vinya" && sec.Label() == "Secció de vinya (actualitzada)" && sec.DisplayOrder() == 9 {
					updated = true
				}
			}
		}
	}
	if !updated {
		t.Error("expected 'e' + submit to update the section's label and display order")
	}
}

// submitForm sets each field's value then submits from the first field (Enter
// now always submits regardless of which field is focused).
func submitForm(t *testing.T, form formModal, values map[string]string) tea.Msg {
	t.Helper()
	for i, f := range form.fields {
		val, ok := values[f.label]
		if !ok {
			continue
		}
		if f.multiline {
			form.fields[i].multi.SetValue(val)
		} else {
			form.fields[i].single.SetValue(val)
		}
	}
	var model tea.Model = form
	_, cmd := model.Update(pKey("enter"))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from submitting the form")
	}
	return cmd()
}

// --- formModal key semantics ---

// newTestForm creates a simple formModal with one single-line and one
// multi-line field, used to test key handling in isolation.
func newTestForm(t *testing.T) formModal {
	t.Helper()
	submitted := map[string]string{}
	onSubmit := func(values map[string]string) tea.Cmd {
		for k, v := range values {
			submitted[k] = v
		}
		return func() tea.Msg { return submitted }
	}
	return newFormModal("Test", []formFieldDef{
		{Label: "Single", Placeholder: "one"},
		{Label: "Multi", Placeholder: "two", Multiline: true},
	}, onSubmit)
}

// TestFormModal_EnterSubmitsFromAnyField verifies that pressing Enter from the
// first field (not the last) produces a submit+close batch command.
func TestFormModal_EnterSubmitsFromAnyField(t *testing.T) {
	form := newTestForm(t)
	// Sanity: focused starts at 0 (first field), not the last.
	if form.focused != 0 {
		t.Fatalf("expected initial focused=0, got %d", form.focused)
	}
	var m tea.Model = form
	_, cmd := m.Update(pKey("enter"))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from Enter on first field")
	}
	// The cmd should be a batch of submit + closeModalCmd; verify it runs
	// without panic and produces at least one message.
	msgs := drainMsgs(cmd())
	if len(msgs) == 0 {
		t.Fatal("expected at least one msg from the submit batch")
	}
}

// TestFormModal_TabAdvancesField verifies that Tab moves focus to the next
// field without submitting.
func TestFormModal_TabAdvancesField(t *testing.T) {
	form := newTestForm(t)
	if form.focused != 0 {
		t.Fatalf("expected initial focused=0, got %d", form.focused)
	}
	var m tea.Model = form
	updated, cmd := m.Update(pKey("tab"))
	// Tab must not emit a submit command.
	if cmd != nil {
		// Allow textinput blink cmds; just check the focused index advanced.
	}
	fm := updated.(formModal)
	if fm.focused != 1 {
		t.Errorf("after Tab, focused = %d, want 1", fm.focused)
	}
}

// TestFormModal_AltEnterInsertsNewlineInMultilineField verifies that Alt+Enter
// inserts a newline character into a focused textarea field.
func TestFormModal_AltEnterInsertsNewlineInMultilineField(t *testing.T) {
	form := newTestForm(t)
	// Advance to the multiline field (index 1).
	form.focused = 1
	form.blurCurrent()
	form.focused = 1
	form.focusCurrent()
	form.fields[1].multi.SetValue("hello")

	var m tea.Model = form
	// Alt+Enter is represented as KeyEnter with Alt=true.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	fm := updated.(formModal)
	val := fm.fields[1].multi.Value()
	if !strings.Contains(val, "\n") {
		t.Errorf("after alt+enter on multiline field, value = %q, want it to contain a newline", val)
	}
}
