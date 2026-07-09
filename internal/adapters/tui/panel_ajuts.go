package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/domain/model"
)

type ajutsView int

const (
	ajutsConcessions ajutsView = iota
	ajutsInvoices
)

// ajutsLoadedMsg carries a (re)load of the year's concessions + invoices. err
// follows the reload-priority convention (mutation error wins over reload error).
type ajutsLoadedMsg struct {
	year        int
	concessions []model.Concession
	links       []model.ConcessionForecast
	invoices    []model.Invoice
	err         error
}

// ajutsPanel is the "Ajuts" panel: lists the year-context's subsidy
// concessions and the invoices reconciled against them. Task 9 is read-only
// (a tab toggles between the two lists); create/edit/delete keys land in
// Tasks 11-12, and JSON import in Task 10.
type ajutsPanel struct {
	deps        Deps
	year        int
	view        ajutsView
	concessions []model.Concession
	links       []model.ConcessionForecast
	invoices    []model.Invoice
	selected    int
	err         error
	status      string
}

// NewAjutsPanel builds the Ajuts panel.
func NewAjutsPanel(deps Deps) Panel { return ajutsPanel{deps: deps} }

func (p ajutsPanel) Title() string { return "Ajuts" }

func (p ajutsPanel) load(ctx context.Context, year int) ajutsLoadedMsg {
	if p.deps.Reconciliation == nil {
		return ajutsLoadedMsg{year: year}
	}
	cs, err := p.deps.Reconciliation.ListConcessions(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	links, err := p.deps.Reconciliation.ListConcessionLinks(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	inv, err := p.deps.Reconciliation.ListInvoices(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	return ajutsLoadedMsg{year: year, concessions: cs, links: links, invoices: inv}
}

func (p ajutsPanel) loadCmd() tea.Cmd {
	year := p.year
	return func() tea.Msg { return p.load(context.Background(), year) }
}

// reloadCmd runs a mutation then reloads; the mutation error takes priority.
func (p ajutsPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	year := p.year
	return func() tea.Msg {
		mutateErr := run(context.Background())
		msg := p.load(context.Background(), year)
		if mutateErr != nil {
			msg.err = mutateErr
		}
		return msg
	}
}

// ajutsImportedMsg carries the outcome of an "i" JSON import trigger.
type ajutsImportedMsg struct {
	year   int
	result string
	err    error
}

func (p ajutsPanel) importCmd() tea.Cmd {
	year := p.year
	deps := p.deps
	return func() tea.Msg {
		if deps.Reconciliation == nil || deps.Cfg == nil {
			return ajutsImportedMsg{year: year, err: fmt.Errorf("importació no disponible")}
		}
		path := filepath.Join(deps.Cfg.ImportDir, fmt.Sprintf("reconciliation-%d.json", year))
		in, err := importer.LoadReconciliation(path, year)
		if err != nil {
			return ajutsImportedMsg{year: year, err: err}
		}
		res, err := deps.Reconciliation.AdminImport(context.Background(), in)
		if err != nil {
			return ajutsImportedMsg{year: year, err: err}
		}
		msg := fmt.Sprintf("Importat: %d concessions, %d factures", res.Concessions, res.Invoices)
		if len(res.Warnings) > 0 {
			msg += fmt.Sprintf(" (%d avisos)", len(res.Warnings))
		}
		return ajutsImportedMsg{year: year, result: msg}
	}
}

func (p ajutsPanel) rowCount() int {
	if p.view == ajutsConcessions {
		return len(p.concessions)
	}
	return len(p.invoices)
}

func (p ajutsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()
	case yearSelectedMsg:
		p.year = msg.Year
		p.selected = 0
		return p, p.loadCmd()
	case ajutsLoadedMsg:
		if msg.year != p.year {
			return p, nil
		}
		p.concessions = msg.concessions
		p.links = msg.links
		p.invoices = msg.invoices
		p.err = msg.err
		if p.selected >= p.rowCount() {
			p.selected = max(0, p.rowCount()-1)
		}
		return p, nil
	case ajutsImportedMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.status = msg.result
		p.err = nil
		return p, p.loadCmd()
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p ajutsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if p.view == ajutsConcessions {
			p.view = ajutsInvoices
		} else {
			p.view = ajutsConcessions
		}
		p.selected = 0
		return p, nil
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < p.rowCount()-1 {
			p.selected++
		}
		return p, nil
	case "i":
		return p, p.importCmd()
	}
	return p, nil
}

// forecastIDsFor returns the CP ids linked to a concession group, comma-joined.
func (p ajutsPanel) forecastIDsFor(groupCode string) string {
	var ids []string
	for _, l := range p.links {
		if l.GroupCode() == groupCode {
			ids = append(ids, l.ForecastID())
		}
	}
	return strings.Join(ids, ",")
}

func (p ajutsPanel) View(width, height int) string {
	header := "[ Concessions | Factures ]"
	if p.view == ajutsInvoices {
		header = "[ concessions | FACTURES ]"
	} else {
		header = "[ CONCESSIONS | factures ]"
	}
	var b strings.Builder
	b.WriteString(dimStyle.Render(truncate(header+"   (tab per canviar)", width)))
	b.WriteString("\n")

	if p.view == ajutsConcessions {
		if len(p.concessions) == 0 {
			b.WriteString(dimStyle.Render("(cap concessió)"))
			return b.String()
		}
		listH := height - 1
		off := scrollOffset(p.selected, len(p.concessions), listH)
		end := min(off+listH, len(p.concessions))
		for i := off; i < end; i++ {
			c := p.concessions[i]
			raw := truncate(fmt.Sprintf("%s  %s  Demanat %s  Concedit %s",
				c.GroupCode(), c.Concept(), c.RequestedTotal(), c.GrantedAmount()), width-2)
			b.WriteString(renderRow(raw, i == p.selected))
			b.WriteString("\n")
		}
		return b.String()
	}

	if len(p.invoices) == 0 {
		b.WriteString(dimStyle.Render("(cap factura)"))
		return b.String()
	}
	listH := height - 1
	off := scrollOffset(p.selected, len(p.invoices), listH)
	end := min(off+listH, len(p.invoices))
	for i := off; i < end; i++ {
		inv := p.invoices[i]
		raw := truncate(fmt.Sprintf("%s  %s  Import %s  %s",
			inv.Number(), inv.Issuer(), inv.NetAmount(), paidLabel(inv)), width-2)
		b.WriteString(renderRow(raw, i == p.selected))
		b.WriteString("\n")
	}
	return b.String()
}

func renderRow(raw string, selected bool) string {
	if selected {
		return focusedPanelStyle.Render("> " + raw)
	}
	return "  " + raw
}

// paidLabel is the Catalan payment status derived from payments vs net.
func paidLabel(inv model.Invoice) string {
	paid := inv.PaidTotal()
	switch {
	case paid.IsZero():
		return "no pagat"
	case paid.Cmp(inv.NetAmount()) >= 0:
		return "pagat"
	default:
		return "parcial"
	}
}

func (p ajutsPanel) Detail() string {
	prefix := ""
	if p.status != "" {
		prefix = p.status + "\n"
	}
	if p.err != nil {
		return prefix + errDetail(p.err)
	}
	if p.view == ajutsConcessions {
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return prefix
		}
		c := p.concessions[p.selected]
		return prefix + fmt.Sprintf("Concessió %s · %s · subtipus %s · Previsions: %s",
			c.GroupCode(), c.Concept(), c.SubtypeCode(), p.forecastIDsFor(c.GroupCode()))
	}
	if p.selected < 0 || p.selected >= len(p.invoices) {
		return prefix
	}
	inv := p.invoices[p.selected]
	return prefix + fmt.Sprintf("Factura %s · %s · Import %s · %d enllaços · %s",
		inv.Number(), inv.Issuer(), inv.NetAmount(), len(inv.Links()), paidLabel(inv))
}

func (p ajutsPanel) Actions() []Action {
	return []Action{
		{Key: "tab", Label: "canvia vista"},
		{Key: "i", Label: "importa JSON"},
	}
}
