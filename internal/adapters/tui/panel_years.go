package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

// yearsLoadedMsg carries the result of (re)loading the windows list.
type yearsLoadedMsg struct {
	windows []model.SubmissionWindow
	err     error
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

// loadCmd returns a tea.Cmd that lists all windows via WindowService.List.
func (p yearsPanel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		windows, err := p.loadWindows(context.Background())
		return yearsLoadedMsg{windows: windows, err: err}
	}
}

// mutateCmd wraps a service call so its error surfaces as a reload (the
// panel always reloads after a mutation, regardless of success/failure).
func (p yearsPanel) mutateCmd(fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		_ = fn(context.Background())
		windows, err := p.loadWindows(context.Background())
		return yearsLoadedMsg{windows: windows, err: err}
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
		if p.selected >= len(p.windows) {
			p.selected = max(0, len(p.windows)-1)
		}
		if w, ok := p.selectedWindow(); ok {
			return p, yearSelectedCmd(w.Year())
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
			return p, yearSelectedCmd(w.Year())
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.windows)-1 {
			p.selected++
		}
		if w, ok := p.selectedWindow(); ok {
			return p, yearSelectedCmd(w.Year())
		}
		return p, nil
	case "n":
		return p, openModalCmd(p.newYearForm())
	case "o":
		return p, p.confirmCmd("Obrir l'any?", p.deps.Windows.Open)
	case "c":
		return p, p.confirmCmd("Tancar l'any?", func(ctx context.Context, year int) error {
			_, err := p.deps.Windows.Close(ctx, year)
			return err
		})
	case "a":
		return p, p.confirmCmd("Esmenar l'any?", func(ctx context.Context, year int) error {
			_, err := p.deps.Windows.Amend(ctx, year)
			return err
		})
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
// the currently selected window's year.
func (p yearsPanel) confirmCmd(message string, fn func(ctx context.Context, year int) error) tea.Cmd {
	w, ok := p.selectedWindow()
	if !ok {
		return nil
	}
	year := w.Year()
	onConfirm := func() tea.Msg {
		_ = fn(context.Background(), year)
		windows, err := p.loadWindows(context.Background())
		return yearsLoadedMsg{windows: windows, err: err}
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

func (p yearsPanel) View(width, height int) string {
	if len(p.windows) == 0 {
		return dimStyle.Render("(cap any)")
	}
	var b strings.Builder
	for i, w := range p.windows {
		line := fmt.Sprintf("%d  %s", w.Year(), w.State())
		styled := stateStyle(w.State()).Render(line)
		if i == p.selected {
			styled = focusedPanelStyle.Render("> ") + styled
		} else {
			styled = "  " + styled
		}
		b.WriteString(styled)
		b.WriteString("\n")
	}
	return b.String()
}

func (p yearsPanel) Detail() string {
	w, ok := p.selectedWindow()
	if !ok {
		if p.err != nil {
			return redStyle.Render(p.err.Error())
		}
		return ""
	}
	opened := "-"
	if w.OpenedAt() != nil {
		opened = w.OpenedAt().Format("2006-01-02")
	}
	closed := "-"
	if w.ClosedAt() != nil {
		closed = w.ClosedAt().Format("2006-01-02")
	}
	return fmt.Sprintf("Any %d  ·  Estat: %s  ·  Obert: %s  ·  Tancat: %s  ·  Límit corrent: %s  ·  Límit inversió: %s",
		w.Year(), w.State(), opened, closed, w.CurrentExpenseLimit(), w.InvestmentExpenseLimit())
}

func (p yearsPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nou any"},
		{Key: "o", Label: "obrir"},
		{Key: "c", Label: "tancar"},
		{Key: "a", Label: "esmenar"},
		{Key: "r", Label: "informe"},
	}
}
