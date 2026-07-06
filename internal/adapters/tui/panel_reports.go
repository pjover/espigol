package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

// reportsPanel is the "Informes" panel: generates the report for the
// year-context (CLOSED -> stored Report export, DRAFT/OPEN -> live preview
// export) via the shared generateReportCmd helper (report_action.go,
// implemented in Task 11) and shows the resulting written paths or error.
// It also shows which years currently have a stored Report, for context.
type reportsPanel struct {
	deps  Deps
	year  int
	state model.WindowState

	years    []int // years with a stored report, ascending
	yearsErr error // error from loading the years-with-reports list

	lastResult *reportDoneMsg
}

// NewReportsPanel builds the Informes panel.
func NewReportsPanel(deps Deps) Panel {
	return reportsPanel{deps: deps}
}

func (p reportsPanel) Title() string { return "Informes" }

// reportYearsLoadedMsg carries the result of listing which years have a
// stored Report (used only for the optional "years with reports" list).
type reportYearsLoadedMsg struct {
	years []int
	err   error
}

func (p reportsPanel) loadYearsCmd() tea.Cmd {
	return func() tea.Msg {
		windows, err := p.deps.Windows.List(context.Background())
		if err != nil {
			return reportYearsLoadedMsg{err: err}
		}
		var years []int
		for _, w := range windows {
			if w.State() == model.WindowClosed {
				if _, ok, err := p.deps.Reports.Latest(context.Background(), w.Year()); err == nil && ok {
					years = append(years, w.Year())
				}
			}
		}
		sort.Ints(years)
		return reportYearsLoadedMsg{years: years}
	}
}

// findWindowStateCmd resolves the year-context's current window state (so
// "r" knows whether to use Export or ExportData) without keeping the whole
// window list around.
func (p reportsPanel) findWindowStateCmd(year int) tea.Cmd {
	return func() tea.Msg {
		windows, err := p.deps.Windows.List(context.Background())
		if err != nil {
			return reportDoneMsg{year: year, err: err}
		}
		for _, w := range windows {
			if w.Year() == year {
				return windowStateMsg{year: year, state: w.State(), found: true}
			}
		}
		return windowStateMsg{year: year, found: false}
	}
}

// windowStateMsg carries the year-context window's state, fetched before
// generating the report so generateReportCmd knows CLOSED vs. DRAFT/OPEN.
type windowStateMsg struct {
	year  int
	state model.WindowState
	found bool
}

func (p reportsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadYearsCmd()

	case yearSelectedMsg:
		p.year = msg.Year
		return p, p.loadYearsCmd()

	case reportYearsLoadedMsg:
		if msg.err != nil {
			p.yearsErr = msg.err
		} else {
			p.yearsErr = nil
			p.years = msg.years
		}
		return p, nil

	case windowStateMsg:
		if msg.year != p.year {
			return p, nil
		}
		if !msg.found {
			p.lastResult = &reportDoneMsg{year: p.year, err: fmt.Errorf("cap any %d trobat", p.year)}
			return p, nil
		}
		p.state = msg.state
		return p, generateReportCmd(p.deps, p.year, p.state)

	case reportDoneMsg:
		if msg.year != p.year {
			return p, nil
		}
		result := msg
		p.lastResult = &result
		return p, p.loadYearsCmd()

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p reportsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "r":
		return p, p.findWindowStateCmd(p.year)
	}
	return p, nil
}

func (p reportsPanel) View(width, height int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Any seleccionat: %d\n\n", p.year))

	if len(p.years) == 0 {
		b.WriteString(dimStyle.Render("(cap any amb informe desat)"))
	} else {
		b.WriteString("Anys amb informe desat: ")
		parts := make([]string, len(p.years))
		for i, y := range p.years {
			parts[i] = fmt.Sprintf("%d", y)
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	return b.String()
}

func (p reportsPanel) Detail() string {
	if p.yearsErr != nil {
		return errDetail(p.yearsErr)
	}
	if p.lastResult == nil {
		return dimStyle.Render("Prem 'r' per generar l'informe de l'any seleccionat.")
	}
	if p.lastResult.err != nil {
		return errDetail(p.lastResult.err)
	}
	if len(p.lastResult.paths) == 0 {
		return dimStyle.Render("Informe generat (cap fitxer).")
	}
	return "Informe generat:\n  " + strings.Join(p.lastResult.paths, "\n  ")
}

func (p reportsPanel) Actions() []Action {
	return []Action{
		{Key: "r", Label: "genera informe"},
	}
}
