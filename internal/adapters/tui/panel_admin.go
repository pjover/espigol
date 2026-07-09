package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// adminPanel is the "Admin" panel (formerly "Informes"). It operates on the
// selected-year context and offers: f generate report, p import forecasts
// (requires OPEN window), c import concessions + invoices / ajuts (no window
// gate), b backup the database, r restore it. It also lists which years have a
// stored Report, for context.
type adminPanel struct {
	deps  Deps
	year  int
	state model.WindowState

	years    []int // years with a stored report, ascending
	yearsErr error // error from loading the years-with-reports list

	result *adminResult
}

// adminResult holds the outcome of the last f/i/b/r action, rendered by Detail.
type adminResult struct {
	text string
	err  error
}

// NewAdminPanel builds the Admin panel.
func NewAdminPanel(deps Deps) Panel {
	return adminPanel{deps: deps}
}

func (p adminPanel) Title() string { return "Admin" }

// reportYearsLoadedMsg carries the result of listing which years have a stored
// Report (used only for the "years with reports" context list).
type reportYearsLoadedMsg struct {
	years []int
	err   error
}

// forecastsImportedMsg carries the outcome of importForecastsCmd.
type forecastsImportedMsg struct {
	year   int
	result application.ImportResult
	err    error
}

// reconciliationImportedMsg carries the outcome of importReconciliationCmd.
type reconciliationImportedMsg struct {
	year   int
	result string
	err    error
}

// backupDoneMsg carries the outcome of backupCmd.
type backupDoneMsg struct {
	path string
	err  error
}

// importForecastsCmd loads Home/import/<year>-forecasts.json and replaces the
// year's forecasts via AdminImport (which requires an OPEN window).
func importForecastsCmd(deps Deps, year int) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(deps.Cfg.ImportDir, fmt.Sprintf("%d-forecasts.json", year))
		entries, err := importer.Load(path, year)
		if err != nil {
			return forecastsImportedMsg{year: year, err: err}
		}
		adminEmail := ""
		if deps.Cfg != nil {
			adminEmail = deps.Cfg.Admin.Email
		}
		res, err := deps.Forecasts.AdminImport(context.Background(), adminEmail, year, entries)
		return forecastsImportedMsg{year: year, result: res, err: err}
	}
}

// importReconciliationCmd loads Home/import/reconciliation-<year>.json and
// replaces the year's concessions + invoices via ReconciliationService.AdminImport.
// No window-state gate: reconciliation is a year-keyed overlay editable in any
// window state (unlike forecast import which requires OPEN).
func importReconciliationCmd(deps Deps, year int) tea.Cmd {
	return func() tea.Msg {
		if deps.Reconciliation == nil || deps.Cfg == nil {
			return reconciliationImportedMsg{year: year, err: fmt.Errorf("importació no disponible")}
		}
		path := filepath.Join(deps.Cfg.ImportDir, fmt.Sprintf("reconciliation-%d.json", year))
		in, err := importer.LoadReconciliation(path, year)
		if err != nil {
			return reconciliationImportedMsg{year: year, err: err}
		}
		res, err := deps.Reconciliation.AdminImport(context.Background(), in)
		if err != nil {
			return reconciliationImportedMsg{year: year, err: err}
		}
		msg := fmt.Sprintf("Importat: %d concessions, %d factures", res.Concessions, res.Invoices)
		if len(res.Warnings) > 0 {
			msg += fmt.Sprintf(" (%d avisos)", len(res.Warnings))
		}
		return reconciliationImportedMsg{year: year, result: msg}
	}
}

func backupCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		path, err := deps.Backup.Backup(context.Background())
		return backupDoneMsg{path: path, err: err}
	}
}

func (p adminPanel) loadYearsCmd() tea.Cmd {
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

// findWindowStateCmd resolves the selected year's window state so the report
// action knows whether to Export (CLOSED) or ExportData (DRAFT/OPEN).
func (p adminPanel) findWindowStateCmd(year int) tea.Cmd {
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

// windowStateMsg carries the selected year's window state, fetched before
// generating the report.
type windowStateMsg struct {
	year  int
	state model.WindowState
	found bool
}

func (p adminPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
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
			p.result = &adminResult{err: fmt.Errorf("cap any %d trobat", p.year)}
			return p, nil
		}
		p.state = msg.state
		return p, generateReportCmd(p.deps, p.year, p.state)

	case reportDoneMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else if len(msg.paths) == 0 {
			p.result = &adminResult{text: "Informe generat (cap fitxer)."}
		} else {
			p.result = &adminResult{text: "Informe generat:\n  " + strings.Join(msg.paths, "\n  ")}
		}
		return p, p.loadYearsCmd()

	case forecastsImportedMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: fmt.Sprintf("Importats %d (esborrats %d)", msg.result.Created, msg.result.Deleted)}
		}
		return p, p.loadYearsCmd()

	case reconciliationImportedMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: msg.result}
		}
		return p, nil

	case backupDoneMsg:
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: "Còpia de seguretat creada:\n  " + msg.path}
		}
		return p, nil

	case restoreStagedMsg:
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: fmt.Sprintf("Restauració preparada: %s\nEs restaurarà en reiniciar l'aplicació.", msg.name)}
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p adminPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "f":
		return p, p.findWindowStateCmd(p.year)
	case "p":
		return p, importForecastsCmd(p.deps, p.year)
	case "c":
		return p, importReconciliationCmd(p.deps, p.year)
	case "b":
		return p, backupCmd(p.deps)
	case "r":
		files, err := p.deps.Backup.ListBackups()
		if err != nil {
			p.result = &adminResult{err: err}
			return p, nil
		}
		if len(files) == 0 {
			p.result = &adminResult{text: dimStyle.Render("(cap còpia de seguretat)")}
			return p, nil
		}
		return p, openModalCmd(newBackupSelectModal(p.deps, files))
	}
	return p, nil
}

func (p adminPanel) View(width, height int) string {
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

func (p adminPanel) Detail() string {
	if p.result != nil {
		if p.result.err != nil {
			return errDetail(p.result.err)
		}
		return p.result.text
	}
	if p.yearsErr != nil {
		return errDetail(p.yearsErr)
	}
	return dimStyle.Render("f: informe · p: importa previsions · c: importa concessions i factures · b: còpia · r: restaura")
}

func (p adminPanel) Actions() []Action {
	return []Action{
		{Key: "f", Label: "genera informe"},
		{Key: "p", Label: "importa previsions"},
		{Key: "c", Label: "importa concessions i factures"},
		{Key: "b", Label: "còpia de seguretat"},
		{Key: "r", Label: "restaura"},
	}
}
