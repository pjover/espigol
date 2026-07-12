package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

// yearsLoadedMsg carries the result of (re)loading the windows list. err is
// either a plain load failure, or — when this message follows a mutation
// (mutateCmd/confirmCmd) — the mutation's own error, which always takes
// priority over the reload's (almost always nil) error. This is what lets
// Detail() show why an Open/Close/Amend/CreateYear was rejected instead of
// silently discarding it.
type yearsLoadedMsg struct {
	windows []model.SubmissionWindow
	err     error
	// selected is the row to select after this load: >= 0 forces that index
	// (used by the initial load to restore the stored active year), -1 means
	// "keep the current selection" (used by mutation reloads).
	selected int
}

// yearsPanel is the "Anys" panel: lists submission windows and lets the
// admin create/open/close/amend years. Selecting a row updates the root
// year context via yearSelectedCmd.
type yearsPanel struct {
	deps     Deps
	windows  []model.SubmissionWindow
	selected int
	err      error
}

// NewYearsPanel builds the Anys (submission windows) panel.
func NewYearsPanel(deps Deps) Panel {
	return yearsPanel{deps: deps}
}

func (p yearsPanel) Title() string { return "Anys" }

// loadWindows lists all windows via WindowService.List, sorted by year
// ascending (the repository makes no ordering guarantee).
func (p yearsPanel) loadWindows(ctx context.Context) ([]model.SubmissionWindow, error) {
	windows, err := p.deps.Windows.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].Year() < windows[j].Year() })
	return windows, nil
}

// loadCmd returns a tea.Cmd that lists all windows via WindowService.List and
// selects the restored active year (defaulting to the current calendar year).
func (p yearsPanel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		windows, err := p.loadWindows(ctx)
		if err != nil {
			return yearsLoadedMsg{windows: windows, err: err, selected: -1}
		}
		return yearsLoadedMsg{windows: windows, err: err, selected: p.initialSelection(ctx, windows)}
	}
}

// initialSelection returns the index of the window to select on boot: the
// stored active year if present (else the current calendar year), falling back
// to the most recent window when that year has no window.
func (p yearsPanel) initialSelection(ctx context.Context, windows []model.SubmissionWindow) int {
	if len(windows) == 0 {
		return 0
	}
	target := p.deps.Clock.Now().Year()
	if y, ok, err := p.deps.ActiveYear.ActiveYear(ctx); err == nil && ok {
		target = y
	}
	for i, w := range windows {
		if w.Year() == target {
			return i
		}
	}
	return len(windows) - 1 // windows are sorted ascending → most recent
}

// mutateCmd wraps a service call: if fn fails, the mutation error is what
// gets surfaced to the panel (the list is still reloaded so the view stays
// fresh, but the reload's own nil error must never clobber a real failure —
// see yearsLoadedMsg.err's doc comment). This is the convention every
// mutating panel (Anys/Socis/Seccions, and Task 12's panels) should mirror.
func (p yearsPanel) mutateCmd(fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		mutateErr := fn(context.Background())
		windows, loadErr := p.loadWindows(context.Background())
		err := loadErr
		if mutateErr != nil {
			err = mutateErr
		}
		return yearsLoadedMsg{windows: windows, err: err, selected: -1}
	}
}

func (p yearsPanel) selectedWindow() (model.SubmissionWindow, bool) {
	if p.selected < 0 || p.selected >= len(p.windows) {
		return model.SubmissionWindow{}, false
	}
	return p.windows[p.selected], true
}

func (p yearsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()

	case yearsLoadedMsg:
		p.windows = msg.windows
		p.err = msg.err
		if msg.selected >= 0 {
			p.selected = msg.selected
		}
		if p.selected >= len(p.windows) {
			p.selected = max(0, len(p.windows)-1)
		}
		if w, ok := p.selectedWindow(); ok {
			return p, yearSelectedCmd(w.Year(), w.State())
		}
		return p, nil

	case yearSelectedMsg:
		// Other panels react to year context changes; the Anys panel is the
		// source of these messages and doesn't need to act on its own echo.
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p yearsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		if w, ok := p.selectedWindow(); ok {
			return p, yearSelectedCmd(w.Year(), w.State())
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.windows)-1 {
			p.selected++
		}
		if w, ok := p.selectedWindow(); ok {
			return p, yearSelectedCmd(w.Year(), w.State())
		}
		return p, nil
	case "n":
		return p, openModalCmd(p.newYearForm())
	case "o":
		w, ok := p.selectedWindow()
		if !ok {
			return p, nil
		}
		if w.State() == model.WindowClosed {
			return p, p.confirmCmd("Reobrir l'any?", p.deps.Windows.Reopen)
		}
		return p, p.confirmCmd("Obrir l'any?", p.deps.Windows.Open)
	case "c":
		return p, p.confirmCmd("Tancar l'any?", func(ctx context.Context, year int) error {
			_, err := p.deps.Windows.Close(ctx, year)
			return err
		})
	case "e":
		w, ok := p.selectedWindow()
		if !ok || w.State() == model.WindowClosed {
			return p, nil
		}
		return p, openModalCmd(p.editYearForm(w))
	case "r":
		w, ok := p.selectedWindow()
		if !ok {
			return p, nil
		}
		return p, generateReportCmd(p.deps, w.Year(), w.State())
	}
	return p, nil
}

// confirmCmd builds a confirm modal that, on "y", calls fn(ctx, year) for
// the currently selected window's year. As with mutateCmd, a mutation
// failure (e.g. ErrWrongState trying to close a DRAFT year) takes priority
// over the subsequent reload's error so it actually reaches p.err.
func (p yearsPanel) confirmCmd(message string, fn func(ctx context.Context, year int) error) tea.Cmd {
	w, ok := p.selectedWindow()
	if !ok {
		return nil
	}
	year := w.Year()
	onConfirm := func() tea.Msg {
		mutateErr := fn(context.Background(), year)
		windows, loadErr := p.loadWindows(context.Background())
		err := loadErr
		if mutateErr != nil {
			err = mutateErr
		}
		return yearsLoadedMsg{windows: windows, err: err, selected: -1}
	}
	return openModalCmd(newConfirmModal(message, onConfirm))
}

// newYearForm builds the "create year" form modal: a single field for the
// new year, submitting calls WindowService.CreateYear.
func (p yearsPanel) newYearForm() formModal {
	nextYear := ""
	if w, ok := p.selectedWindow(); ok {
		nextYear = strconv.Itoa(w.Year() + 1)
	}
	fields := []formFieldDef{
		{Label: "Any", Placeholder: "2027", Value: nextYear},
	}
	onSubmit := func(values map[string]string) tea.Cmd {
		year, err := strconv.Atoi(strings.TrimSpace(values["Any"]))
		if err != nil {
			return nil
		}
		return p.mutateCmd(func(ctx context.Context) error {
			_, err := p.deps.Windows.CreateYear(ctx, year)
			return err
		})
	}
	return newFormModal("Nou any", fields, onSubmit)
}

// editYearForm builds the edit form for an existing DRAFT or OPEN year:
// deadline (YYYY-MM-DD), currentExpenseLimit, investmentExpenseLimit.
func (p yearsPanel) editYearForm(w model.SubmissionWindow) formModal {
	fields := []formFieldDef{
		{Label: "Termini", Placeholder: "2026-12-31", Value: w.Deadline().Format("2006-01-02")},
		{Label: "Límit corrent", Placeholder: "30000.00", Value: w.CurrentExpenseLimit().String()},
		{Label: "Límit inversió", Placeholder: "70000.00", Value: w.InvestmentExpenseLimit().String()},
	}
	onSubmit := func(values map[string]string) tea.Cmd {
		deadline, err := time.Parse("2006-01-02", strings.TrimSpace(values["Termini"]))
		if err != nil {
			return nil
		}
		current, err := model.MoneyFromString(strings.TrimSpace(values["Límit corrent"]))
		if err != nil {
			return nil
		}
		investment, err := model.MoneyFromString(strings.TrimSpace(values["Límit inversió"]))
		if err != nil {
			return nil
		}
		year := w.Year()
		return p.mutateCmd(func(ctx context.Context) error {
			return p.deps.Windows.EditYear(ctx, year, deadline, current, investment)
		})
	}
	return newFormModal("Editar any", fields, onSubmit)
}

func (p yearsPanel) View(width, height int) string {
	if len(p.windows) == 0 {
		return dimStyle.Render("(cap any)")
	}
	off := scrollOffset(p.selected, len(p.windows), height)
	end := off + height
	if end > len(p.windows) {
		end = len(p.windows)
	}
	var b strings.Builder
	for i, w := range p.windows[off:end] {
		idx := off + i
		raw := truncate(fmt.Sprintf("%d  %s", w.Year(), w.State()), width-2)
		var styled string
		if idx == p.selected {
			styled = focusedPanelStyle.Render("> ") + stateStyle(w.State()).Render(raw)
		} else {
			styled = "  " + stateStyle(w.State()).Render(raw)
		}
		b.WriteString(styled)
		b.WriteString("\n")
	}
	return b.String()
}

func (p yearsPanel) Detail() string {
	w, ok := p.selectedWindow()
	if !ok {
		return errDetail(p.err)
	}
	opened := "-"
	if w.OpenedAt() != nil {
		opened = w.OpenedAt().Format("2006-01-02")
	}
	closed := "-"
	if w.ClosedAt() != nil {
		closed = w.ClosedAt().Format("2006-01-02")
	}
	detail := fmt.Sprintf("Any %d  ·  Estat: %s  ·  Obert: %s  ·  Tancat: %s  ·  Límit corrent: %s  ·  Límit inversió: %s",
		w.Year(), w.State(), opened, closed, w.CurrentExpenseLimit(), w.InvestmentExpenseLimit())
	if errLine := errDetail(p.err); errLine != "" {
		detail += "\n" + errLine
	}
	return detail
}

func (p yearsPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nou any"},
		{Key: "o", Label: "obrir"},
		{Key: "c", Label: "tancar"},
		{Key: "e", Label: "edita"},
		{Key: "r", Label: "informe"},
	}
}
