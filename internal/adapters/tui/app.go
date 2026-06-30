package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panelInitMsg is sent once to every panel from rootModel.Init, giving each
// panel a chance to return its initial load command (e.g. "fetch the
// partner list"). Panels that don't need an initial load can ignore it and
// return (p, nil). This exists because Panel.Update takes a tea.Msg and the
// Panel interface intentionally has no separate Init method — keeping the
// interface small is what lets T11/T12 add panels without touching this
// file.
type panelInitMsg struct{}

// modalClosedMsg signals that the active modal (confirmModal/formModal, or
// any future tea.Model used as a modal) is done and the root model should
// clear m.modal. Modals emit it via closeModalCmd; panels that build their
// own bespoke modal should do the same so the root's modal-routing in
// Update keeps working unchanged.
type modalClosedMsg struct{}

// closeModalCmd is the tea.Cmd modals return to signal modalClosedMsg.
func closeModalCmd() tea.Msg { return modalClosedMsg{} }

// openModalMsg asks the root model to open the given tea.Model as the
// active modal. Panels that want to show a confirm/form modal return a
// tea.Cmd producing this message from their Update (rather than mutating
// root state directly, since a Panel only has access to itself).
type openModalMsg struct {
	modal tea.Model
}

// openModalCmd builds the tea.Cmd a panel returns to ask the root model to
// open modal as the active modal overlay.
func openModalCmd(modal tea.Model) tea.Cmd {
	return func() tea.Msg { return openModalMsg{modal: modal} }
}

// yearSelectedMsg lets a panel (the Anys panel, Task 11) change the root
// year context, e.g. when the user moves the selection in the years list.
// The root model updates m.year and then forwards the same message down to
// every panel's Update (including the sender), so other panels (Previsions,
// Informes, Tipus i subtipus — Task 12) can react and reload for the new
// year.
type yearSelectedMsg struct {
	Year int
}

// yearSelectedCmd is the tea.Cmd a panel returns to change the root year
// context.
func yearSelectedCmd(year int) tea.Cmd {
	return func() tea.Msg { return yearSelectedMsg{Year: year} }
}

// rootModel is the Bubble Tea root model. It owns global navigation (panel
// focus, help overlay, modal overlay, window size, year context) and
// delegates everything else to the focused Panel.
//
// Panel extension point: panels are injected via NewApp's panels argument
// (see NewApp below) rather than constructed here. Tasks 11/12 add their
// panels by building a []Panel (e.g. []Panel{NewYearsPanel(deps),
// NewPartnersPanel(deps), NewSectionsPanel(deps), ...}) and passing it to
// NewApp. The order of the slice is the left-to-right/tab order and the
// order panels are listed in the left column.
type rootModel struct {
	deps    Deps
	panels  []Panel
	focused int
	year    int

	help     help.Model
	showHelp bool
	modal    tea.Model

	width  int
	height int
}

// newRootModel builds the initial root model for the given deps and panel
// set. year defaults to the current calendar year as a starting context;
// panels that care about year selection (the Anys panel, Task 11) update it
// via a yearSelectedMsg as they implement that behaviour.
func newRootModel(deps Deps, panels []Panel) rootModel {
	return rootModel{
		deps:   deps,
		panels: panels,
		help:   help.New(),
		year:   time.Now().Year(),
	}
}

// FocusedTitle returns the Title() of the currently focused panel, or ""
// if there are no panels. It exists primarily as a test accessor.
func (m rootModel) FocusedTitle() string {
	if len(m.panels) == 0 {
		return ""
	}
	return m.panels[m.focused].Title()
}

// Init implements tea.Model: it sends panelInitMsg to every panel and
// batches the resulting commands (typically each panel's initial load from
// its application service).
func (m rootModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.panels))
	for i, p := range m.panels {
		updated, cmd := p.Update(panelInitMsg{})
		m.panels[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case openModalMsg:
		m.modal = msg.modal
		var cmd tea.Cmd
		if init, ok := m.modal.(interface{ Init() tea.Cmd }); ok {
			cmd = init.Init()
		}
		return m, cmd

	case modalClosedMsg:
		m.modal = nil
		return m, nil

	case yearSelectedMsg:
		m.year = msg.Year
		cmds := make([]tea.Cmd, 0, len(m.panels))
		for i, p := range m.panels {
			updated, cmd := p.Update(msg)
			m.panels[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}

	// While a modal is active, route all other messages to it exclusively.
	if m.modal != nil {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "tab", "right":
			m.focused = m.nextFocus(1)
			return m, nil
		case "shift+tab", "left":
			m.focused = m.nextFocus(-1)
			return m, nil
		}
	}

	if len(m.panels) == 0 {
		return m, nil
	}
	panel, cmd := m.panels[m.focused].Update(msg)
	m.panels[m.focused] = panel
	return m, cmd
}

// nextFocus computes the next focused panel index, wrapping around, moving
// by delta (typically +1 or -1).
func (m rootModel) nextFocus(delta int) int {
	n := len(m.panels)
	if n == 0 {
		return 0
	}
	return ((m.focused+delta)%n + n) % n
}

// View implements tea.Model.
func (m rootModel) View() string {
	var b strings.Builder

	businessName := ""
	if m.deps.Cfg != nil {
		businessName = m.deps.Cfg.BusinessName
	}
	topBar := titleStyle.Render(businessName) + "   " + dimStyle.Render("Any: ") + titleStyle.Render(strconv.Itoa(m.year))
	b.WriteString(topBar)
	b.WriteString("\n\n")

	left := m.renderPanelList()
	main := m.renderMain()

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", main)
	b.WriteString(body)
	b.WriteString("\n\n")

	b.WriteString(m.renderHelpLine())

	view := b.String()
	if m.modal != nil {
		view = view + "\n\n" + m.modal.View()
	}
	return view
}

// renderPanelList renders the left-hand column of panel titles, the
// focused one highlighted.
func (m rootModel) renderPanelList() string {
	var lines []string
	for i, p := range m.panels {
		title := p.Title()
		if i == m.focused {
			title = focusedPanelStyle.Render("> " + title)
		} else {
			title = dimStyle.Render("  " + title)
		}
		lines = append(lines, title)
	}
	return strings.Join(lines, "\n")
}

// renderMain renders the focused panel's main content and detail.
func (m rootModel) renderMain() string {
	if len(m.panels) == 0 {
		return dimStyle.Render("(cap panell)")
	}
	p := m.panels[m.focused]
	main := p.View(m.width, m.height)
	detail := p.Detail()
	if detail == "" {
		return main
	}
	return main + "\n\n" + detail
}

// renderHelpLine builds the bottom keybinding line from the focused
// panel's Actions() plus the global keys.
func (m rootModel) renderHelpLine() string {
	var parts []string
	if len(m.panels) > 0 {
		for _, a := range m.panels[m.focused].Actions() {
			parts = append(parts, a.Key+": "+a.Label)
		}
	}
	parts = append(parts, "tab: panell seguent", "?: ajuda", "q: surt")
	line := strings.Join(parts, "  ·  ")
	if m.showHelp {
		line = line + "\n" + helpStyle.Render("shift+tab/left: panell anterior  ·  ctrl+c: surt")
	}
	return helpStyle.Render(line)
}

// App is the TUI adapter's entry point: it wraps the root Bubble Tea model
// and exposes Run for cmd/espigol to start the program.
type App struct {
	model rootModel
}

// NewApp builds the application's root TUI model from deps and an
// initial set of panels.
//
// Panel extension point: callers (Task 13's wire.TUI, and tests in this
// package) build the []Panel slice and pass it here; NewApp itself does
// not know about specific panel types. Tasks 11/12 add new panel
// constructors (e.g. NewYearsPanel, NewPartnersPanel, ...) and the wiring
// layer lists them in display order, e.g.:
//
//	panels := []Panel{
//	    tui.NewYearsPanel(deps),
//	    tui.NewPartnersPanel(deps),
//	    tui.NewSectionsPanel(deps),
//	    tui.NewTaxonomyPanel(deps),
//	    tui.NewForecastsPanel(deps),
//	    tui.NewReportsPanel(deps),
//	}
//	app := tui.NewApp(deps, panels)
func NewApp(deps Deps, panels []Panel) *App {
	return &App{model: newRootModel(deps, panels)}
}

// Run starts the Bubble Tea program and blocks until the user quits.
func (a *App) Run() error {
	_, err := tea.NewProgram(a.model).Run()
	return err
}
