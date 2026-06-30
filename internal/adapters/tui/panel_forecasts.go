package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// forecastsLoadedMsg carries the result of (re)loading the year-context's
// forecasts (all partners, all scopes). err is either a plain load failure,
// or — when this message follows a mutation (reloadCmd) — the mutation's own
// error, which always takes priority over the reload's (almost always nil)
// error. Mirrors panel_years.go's convention.
type forecastsLoadedMsg struct {
	year      int
	forecasts []model.ExpenseForecast
	err       error
}

// forecastsPanel is the "Previsions" panel: lists every forecast for the
// year-context (all partners, all scopes) and lets the admin create/edit/
// delete them via ForecastService's Admin* methods (impersonation, no scope
// authorization check — the admin acts as cfg.Admin.Email).
type forecastsPanel struct {
	deps      Deps
	year      int
	forecasts []model.ExpenseForecast
	selected  int
	err       error
}

// NewForecastsPanel builds the Previsions panel.
func NewForecastsPanel(deps Deps) Panel {
	return forecastsPanel{deps: deps}
}

func (p forecastsPanel) Title() string { return "Previsions" }

func (p forecastsPanel) loadCmd() tea.Cmd {
	year := p.year
	return func() tea.Msg {
		forecasts, err := p.deps.Forecasts.ListByYear(context.Background(), year)
		return forecastsLoadedMsg{year: year, forecasts: forecasts, err: err}
	}
}

// reloadCmd wraps a service call: if run fails, the mutation error is what
// gets surfaced to the panel (the list is still reloaded so the view stays
// fresh, but the reload's own nil error must never clobber a real failure).
// Mirrors panel_years.go/panel_sections.go/panel_partners.go's convention.
func (p forecastsPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	year := p.year
	return func() tea.Msg {
		mutateErr := run(context.Background())
		forecasts, loadErr := p.deps.Forecasts.ListByYear(context.Background(), year)
		err := loadErr
		if mutateErr != nil {
			err = mutateErr
		}
		return forecastsLoadedMsg{year: year, forecasts: forecasts, err: err}
	}
}

func (p forecastsPanel) selectedForecast() (model.ExpenseForecast, bool) {
	if p.selected < 0 || p.selected >= len(p.forecasts) {
		return model.ExpenseForecast{}, false
	}
	return p.forecasts[p.selected], true
}

func (p forecastsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()

	case yearSelectedMsg:
		p.year = msg.Year
		p.selected = 0
		return p, p.loadCmd()

	case forecastsLoadedMsg:
		if msg.year != p.year {
			// Stale reply from a year we've since navigated away from.
			return p, nil
		}
		p.forecasts = msg.forecasts
		p.err = msg.err
		if p.selected >= len(p.forecasts) {
			p.selected = max(0, len(p.forecasts)-1)
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p forecastsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.forecasts)-1 {
			p.selected++
		}
		return p, nil
	case "n":
		return p, openModalCmd(p.forecastForm(nil))
	case "e":
		if existing, ok := p.selectedForecast(); ok {
			return p, openModalCmd(p.forecastForm(&existing))
		}
		return p, nil
	case "d":
		existing, ok := p.selectedForecast()
		if !ok {
			return p, nil
		}
		id := existing.ID()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Forecasts.AdminDelete(ctx, p.adminEmail(), id)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar la previsió %s?", id), onConfirm))
	}
	return p, nil
}

func (p forecastsPanel) adminEmail() string {
	if p.deps.Cfg == nil {
		return ""
	}
	return p.deps.Cfg.Admin.Email
}

// forecastForm builds the create/edit forecast form: a bespoke tea.Model
// (not the generic textinput-only formModal) combining text fields with
// cycling selectors for partner/scope/subtype, since form.go's formModal
// intentionally has no selector widget (see its doc comment). existing ==
// nil means "create".
func (p forecastsPanel) forecastForm(existing *model.ExpenseForecast) tea.Model {
	return newForecastFormModal(p.deps, p.year, existing, p.reloadCmd)
}

func (p forecastsPanel) View(width, height int) string {
	if len(p.forecasts) == 0 {
		return dimStyle.Render("(cap previsió)")
	}
	var b strings.Builder
	for i, f := range p.forecasts {
		line := fmt.Sprintf("%s  soci %d  %s  %s  %s €", f.ID(), f.PartnerID(), f.Scope().Kind(), f.Concept(), f.GrossAmount())
		if i == p.selected {
			line = focusedPanelStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (p forecastsPanel) Detail() string {
	f, ok := p.selectedForecast()
	if !ok {
		return errDetail(p.err)
	}
	scope := string(f.Scope().Kind())
	if f.Scope().Kind() == model.ScopeSection {
		scope += ":" + f.Scope().SectionCode()
	}
	detail := fmt.Sprintf("%s  ·  soci %d  ·  %s  ·  %s  ·  Subtipus: %s  ·  Import: %s €  ·  Data prevista: %s",
		f.ID(), f.PartnerID(), scope, f.Concept(), f.SubtypeCode(), f.GrossAmount(), f.PlannedDate().Format("2006-01-02"))
	if errLine := errDetail(p.err); errLine != "" {
		detail += "\n" + errLine
	}
	return detail
}

func (p forecastsPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nova"},
		{Key: "e", Label: "edita"},
		{Key: "d", Label: "elimina"},
	}
}

// --- bespoke forecast form modal (text fields + cycling selectors) ---

// forecastFormModal is the Previsions panel's create/edit modal. Unlike the
// generic formModal (form.go), it needs selector-style fields (partner id,
// scope kind, subtype code) alongside free-text fields, so it's a standalone
// tea.Model that still honours the same modalClosedMsg/openModalCmd
// convention every other modal uses.
type forecastFormModal struct {
	deps     Deps
	year     int
	existing *model.ExpenseForecast
	reload   func(run func(ctx context.Context) error) tea.Cmd

	title string

	// Selector fields, cycled with left/right when focused.
	partnerIdx int
	partners   []model.Partner
	scopeIdx   int
	scopes     []model.ScopeKind
	sectionIdx int
	sections   []model.Section
	subtypeIdx int
	subtypes   []model.ExpenseSubtype

	// Text fields.
	concept     textinput.Model
	description textinput.Model
	gross       textinput.Model
	plannedDate textinput.Model

	// focused indexes into fieldOrder.
	focused int
}

// forecastFormField identifies which field of the form is focused.
type forecastFormField int

const (
	fieldPartner forecastFormField = iota
	fieldScope
	fieldSection
	fieldSubtype
	fieldConcept
	fieldDescription
	fieldGross
	fieldPlannedDate
	forecastFormFieldCount
)

// newForecastFormModal builds the form, preloading the partner and subtype
// lists synchronously (small admin-only lists; matches the rest of the TUI's
// pattern of loading via tea.Cmd is unnecessary here since this only opens
// after the panel already has its year context — the lists are fetched at
// construction so the form can render selectors immediately).
func newForecastFormModal(deps Deps, year int, existing *model.ExpenseForecast, reload func(run func(ctx context.Context) error) tea.Cmd) *forecastFormModal {
	ctx := context.Background()
	partners, _ := deps.Partners.List(ctx)
	subtypes, _ := deps.Taxonomy.ListSubtypes(ctx, year)
	sections, _ := deps.Sections.List(ctx)
	scopes := []model.ScopeKind{model.ScopePartner, model.ScopeCommon, model.ScopeSection}

	title := "Nova previsió"
	concept, description, gross, plannedDate := textinput.New(), textinput.New(), textinput.New(), textinput.New()
	concept.Placeholder = "Concepte"
	description.Placeholder = "Descripció"
	gross.Placeholder = "100.00"
	plannedDate.Placeholder = "2026-03-01"

	partnerIdx, scopeIdx, sectionIdx, subtypeIdx := 0, 0, 0, 0

	if existing != nil {
		title = "Edita previsió"
		concept.SetValue(existing.Concept())
		description.SetValue(existing.Description())
		gross.SetValue(existing.GrossAmount().String())
		plannedDate.SetValue(existing.PlannedDate().Format("2006-01-02"))
		for i, pt := range partners {
			if pt.ID() == existing.PartnerID() {
				partnerIdx = i
				break
			}
		}
		for i, sk := range scopes {
			if sk == existing.Scope().Kind() {
				scopeIdx = i
				break
			}
		}
		for i, sec := range sections {
			if sec.Code() == existing.Scope().SectionCode() {
				sectionIdx = i
				break
			}
		}
		for i, st := range subtypes {
			if st.Code() == existing.SubtypeCode() {
				subtypeIdx = i
				break
			}
		}
	} else {
		plannedDate.SetValue(time.Now().Format("2006-01-02"))
	}

	concept.Focus()

	return &forecastFormModal{
		deps: deps, year: year, existing: existing, reload: reload,
		title:       title,
		partnerIdx:  partnerIdx,
		partners:    partners,
		scopeIdx:    scopeIdx,
		scopes:      scopes,
		sectionIdx:  sectionIdx,
		sections:    sections,
		subtypeIdx:  subtypeIdx,
		subtypes:    subtypes,
		concept:     concept,
		description: description,
		gross:       gross,
		plannedDate: plannedDate,
		focused:     int(fieldPartner),
	}
}

// Init implements tea.Model.
func (m *forecastFormModal) Init() tea.Cmd { return textinput.Blink }

func (m *forecastFormModal) isSelector(f forecastFormField) bool {
	switch f {
	case fieldPartner, fieldScope, fieldSection, fieldSubtype:
		return true
	}
	return false
}

// Update implements tea.Model.
func (m *forecastFormModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		m.updateTextField(msg)
		return m, nil
	}

	switch keyMsg.String() {
	case "esc":
		return m, closeModalCmd
	case "enter":
		return m, tea.Batch(m.submit(), closeModalCmd)
	case "tab", "down":
		m.blurCurrent()
		m.focused = (m.focused + 1) % int(forecastFormFieldCount)
		m.focusCurrent()
		return m, nil
	case "shift+tab", "up":
		m.blurCurrent()
		m.focused = (m.focused - 1 + int(forecastFormFieldCount)) % int(forecastFormFieldCount)
		m.focusCurrent()
		return m, nil
	case "left":
		if m.isSelector(forecastFormField(m.focused)) {
			m.cycleSelector(-1)
			return m, nil
		}
	case "right":
		if m.isSelector(forecastFormField(m.focused)) {
			m.cycleSelector(1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	if !m.isSelector(forecastFormField(m.focused)) {
		cmd = m.updateTextField(msg)
	}
	return m, cmd
}

func (m *forecastFormModal) cycleSelector(delta int) {
	switch forecastFormField(m.focused) {
	case fieldPartner:
		if n := len(m.partners); n > 0 {
			m.partnerIdx = ((m.partnerIdx+delta)%n + n) % n
		}
	case fieldScope:
		if n := len(m.scopes); n > 0 {
			m.scopeIdx = ((m.scopeIdx+delta)%n + n) % n
		}
	case fieldSection:
		if n := len(m.sections); n > 0 {
			m.sectionIdx = ((m.sectionIdx+delta)%n + n) % n
		}
	case fieldSubtype:
		if n := len(m.subtypes); n > 0 {
			m.subtypeIdx = ((m.subtypeIdx+delta)%n + n) % n
		}
	}
}

func (m *forecastFormModal) updateTextField(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch forecastFormField(m.focused) {
	case fieldConcept:
		m.concept, cmd = m.concept.Update(msg)
	case fieldDescription:
		m.description, cmd = m.description.Update(msg)
	case fieldGross:
		m.gross, cmd = m.gross.Update(msg)
	case fieldPlannedDate:
		m.plannedDate, cmd = m.plannedDate.Update(msg)
	}
	return cmd
}

func (m *forecastFormModal) blurCurrent() {
	switch forecastFormField(m.focused) {
	case fieldConcept:
		m.concept.Blur()
	case fieldDescription:
		m.description.Blur()
	case fieldGross:
		m.gross.Blur()
	case fieldPlannedDate:
		m.plannedDate.Blur()
	}
}

func (m *forecastFormModal) focusCurrent() {
	switch forecastFormField(m.focused) {
	case fieldConcept:
		m.concept.Focus()
	case fieldDescription:
		m.description.Focus()
	case fieldGross:
		m.gross.Focus()
	case fieldPlannedDate:
		m.plannedDate.Focus()
	}
}

// submit validates and builds the tea.Cmd that calls AdminCreate/AdminUpdate
// (via the panel's reload helper, so the list refreshes and any error is
// surfaced the same way every other mutation is). Returns nil (no-op) if a
// required field fails to parse — the modal still closes; this mirrors
// form.go's onSubmit convention where parse failures are simply dropped.
func (m *forecastFormModal) submit() tea.Cmd {
	gross, err := model.MoneyFromString(strings.TrimSpace(m.gross.Value()))
	if err != nil {
		return nil
	}
	plannedDate, err := time.Parse("2006-01-02", strings.TrimSpace(m.plannedDate.Value()))
	if err != nil {
		return nil
	}
	var scopeKind model.ScopeKind
	var sectionCode string
	if len(m.scopes) > 0 {
		scopeKind = m.scopes[m.scopeIdx]
	}
	if scopeKind == model.ScopeSection && len(m.sections) > 0 {
		sectionCode = m.sections[m.sectionIdx].Code()
	}

	in := application.ForecastInput{
		Concept:     m.concept.Value(),
		Description: m.description.Value(),
		GrossAmount: gross,
		PlannedDate: plannedDate,
		ScopeKind:   scopeKind,
		SectionCode: sectionCode,
	}
	if len(m.subtypes) > 0 {
		in.SubtypeCode = m.subtypes[m.subtypeIdx].Code()
	}

	adminEmail := ""
	if m.deps.Cfg != nil {
		adminEmail = m.deps.Cfg.Admin.Email
	}

	if m.existing == nil {
		partnerID := 0
		if len(m.partners) > 0 {
			partnerID = m.partners[m.partnerIdx].ID()
		}
		year := m.year
		return m.reload(func(ctx context.Context) error {
			_, err := m.deps.Forecasts.AdminCreate(ctx, adminEmail, year, partnerID, in)
			return err
		})
	}
	id := m.existing.ID()
	return m.reload(func(ctx context.Context) error {
		return m.deps.Forecasts.AdminUpdate(ctx, adminEmail, id, in)
	})
}

// View implements tea.Model.
func (m *forecastFormModal) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")

	b.WriteString(m.selectorLine("Soci", fieldPartner, m.partnerLabel()))
	b.WriteString(m.selectorLine("Abast", fieldScope, m.scopeLabel()))
	if m.scopes[m.scopeIdx] == model.ScopeSection {
		b.WriteString(m.selectorLine("Secció", fieldSection, m.sectionLabel()))
	}
	b.WriteString(m.selectorLine("Subtipus", fieldSubtype, m.subtypeLabel()))
	b.WriteString(m.textLine("Concepte", fieldConcept, m.concept))
	b.WriteString(m.textLine("Descripció", fieldDescription, m.description))
	b.WriteString(m.textLine("Import brut", fieldGross, m.gross))
	b.WriteString(m.textLine("Data prevista", fieldPlannedDate, m.plannedDate))

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("tab/shift+tab: mou camp · left/right: canvia selector · enter: desa · esc: cancel·la"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)
	return box.Render(b.String())
}

func (m *forecastFormModal) partnerLabel() string {
	if len(m.partners) == 0 {
		return "(cap soci)"
	}
	pt := m.partners[m.partnerIdx]
	return fmt.Sprintf("%d %s %s", pt.ID(), pt.Name(), pt.Surname())
}

func (m *forecastFormModal) scopeLabel() string {
	if len(m.scopes) == 0 {
		return "(cap abast)"
	}
	return string(m.scopes[m.scopeIdx])
}

func (m *forecastFormModal) sectionLabel() string {
	if len(m.sections) == 0 {
		return "(cap secció)"
	}
	sec := m.sections[m.sectionIdx]
	return fmt.Sprintf("%s %s", sec.Code(), sec.Label())
}

func (m *forecastFormModal) subtypeLabel() string {
	if len(m.subtypes) == 0 {
		return "(cap subtipus)"
	}
	st := m.subtypes[m.subtypeIdx]
	return fmt.Sprintf("%s %s", st.Code(), st.Label())
}

func (m *forecastFormModal) selectorLine(label string, field forecastFormField, value string) string {
	styledLabel := dimStyle.Render(label)
	if forecastFormField(m.focused) == field {
		styledLabel = focusedPanelStyle.Render(label)
	}
	return fmt.Sprintf("%s: < %s >\n", styledLabel, value)
}

func (m *forecastFormModal) textLine(label string, field forecastFormField, ti textinput.Model) string {
	styledLabel := dimStyle.Render(label)
	if forecastFormField(m.focused) == field {
		styledLabel = focusedPanelStyle.Render(label)
	}
	return fmt.Sprintf("%s: %s\n", styledLabel, ti.View())
}
