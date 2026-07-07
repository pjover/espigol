package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/pjover/espigol/internal/domain/model"
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
	Year  int
	State model.WindowState
}

// yearSelectedCmd is the tea.Cmd a panel returns to change the root year
// context.
func yearSelectedCmd(year int, state model.WindowState) tea.Cmd {
	return func() tea.Msg { return yearSelectedMsg{Year: year, State: state} }
}

// rootModel is the Bubble Tea root model. It owns global navigation (panel
// focus, modal overlay, window size, year context) and delegates everything
// else to the focused Panel.
//
// Panel extension point: panels are injected via NewApp's panels argument
// (see NewApp below) rather than constructed here. Tasks 11/12 add their
// panels by building a []Panel (e.g. []Panel{NewYearsPanel(deps),
// NewPartnersPanel(deps), NewSectionsPanel(deps), ...}) and passing it to
// NewApp. The order of the slice is the [1-6] key order and the order
// panels are listed in the sidebar.
type rootModel struct {
	deps      Deps
	panels    []Panel
	focused   int
	year      int
	yearState model.WindowState

	modal tea.Model

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
		m.yearState = msg.State
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
		case "1", "2", "3", "4", "5", "6":
			idx := int(keyMsg.String()[0] - '1')
			if idx >= 0 && idx < len(m.panels) {
				m.focused = idx
			}
			return m, nil
		}
	}

	if len(m.panels) == 0 {
		return m, nil
	}

	// Key messages go only to the focused panel (panel-specific actions).
	// All other messages (async load results like partnersLoadedMsg,
	// yearsLoadedMsg, etc.) are broadcast to every panel so that background
	// loads complete regardless of which panel currently has focus.
	if _, isKey := msg.(tea.KeyMsg); isKey {
		panel, cmd := m.panels[m.focused].Update(msg)
		m.panels[m.focused] = panel
		return m, cmd
	}
	var cmds []tea.Cmd
	for i, p := range m.panels {
		updated, cmd := p.Update(msg)
		m.panels[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m rootModel) View() string {
	if m.width > 0 && m.height > 0 && (m.width < 80 || m.height < 24) {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			redStyle.Render("Terminal massa petit (mínim 80×24)"))
	}

	sidebar := m.renderSidebar()
	center := m.renderCenter()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", center)
	view := body + "\n" + m.renderFooter()

	if m.modal != nil {
		view = lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center, m.modal.View())
	}
	return view
}

// sidebarOuterWidth is the total rendered width of the sidebar including border+padding.
// sidebarInnerWidth (20) + 2 (Padding(0,1)) + 2 (RoundedBorder left+right) = 24.
const sidebarOuterWidth = 24

// renderSidebar renders the left panel showing business name, year context,
// state badge, and the numbered panel list.
func (m rootModel) renderSidebar() string {
	var b strings.Builder

	businessName := ""
	if m.deps.Cfg != nil {
		businessName = m.deps.Cfg.BusinessName
	}
	b.WriteString(titleStyle.Render(businessName))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Any: ") + titleStyle.Render(strconv.Itoa(m.year)))
	b.WriteString("\n")
	b.WriteString(stateBadge(m.yearState))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", sidebarInnerWidth)))
	b.WriteString("\n")

	for i, p := range m.panels {
		// "[N] " prefix is 4 chars; truncate title to fill sidebarInnerWidth exactly.
		entry := fmt.Sprintf("[%d] %s", i+1, truncate(p.Title(), sidebarInnerWidth-4))
		if i == m.focused {
			entry = focusedPanelStyle.Render(entry)
		} else {
			entry = dimStyle.Render(entry)
		}
		b.WriteString(entry + "\n")
	}

	innerH := m.height - 1 - 2 // footer(1) + top/bottom border(2)
	if innerH < 3 {
		innerH = 3
	}
	return sidebarStyle.Height(innerH).Render(b.String())
}

// renderCenter renders the focused panel's content in the center pane.
func (m rootModel) renderCenter() string {
	// centerInnerW = total width - sidebar outer - gap(1) - center border+padding(4)
	centerInnerW := m.width - sidebarOuterWidth - 1 - 4
	if centerInnerW < 10 {
		centerInnerW = 10
	}
	centerInnerH := m.height - 1 - 2 // reserve footer(1) + center top+bottom border(2)
	if centerInnerH < 3 {
		centerInnerH = 3
	}
	if len(m.panels) == 0 {
		return centerStyle.Width(centerInnerW).Render(dimStyle.Render("(cap panell)"))
	}

	p := m.panels[m.focused]
	detail := p.Detail()

	// Give the list the full height when there is no detail; otherwise leave
	// room for blank-line + separator + detail (overhead = 2 + detailLines).
	listH := centerInnerH
	if detail != "" {
		detailLines := strings.Count(detail, "\n") + 1
		listH = centerInnerH - 2 - detailLines
		if listH < 3 {
			listH = 3
		}
	}

	list := p.View(centerInnerW, listH)

	var content string
	if detail == "" {
		content = list
	} else {
		sep := dimStyle.Render(strings.Repeat("─", centerInnerW-2))
		content = list + "\n" + sep + "\n" + detail
	}

	return centerStyle.Width(centerInnerW).Height(centerInnerH).Render(content)
}

// renderFooter builds the bottom keybinding line from the focused
// panel's Actions() plus the global keys.
func (m rootModel) renderFooter() string {
	var parts []string
	if len(m.panels) > 0 {
		for _, a := range m.panels[m.focused].Actions() {
			parts = append(parts, "["+a.Key+"] "+a.Label)
		}
	}
	parts = append(parts, "[↑↓] navegar", "[1-6] panell", "[q] surt")
	return helpStyle.Render(strings.Join(parts, "  "))
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
//	    tui.NewAdminPanel(deps),
//	}
//	app := tui.NewApp(deps, panels)
func NewApp(deps Deps, panels []Panel) *App {
	return &App{model: newRootModel(deps, panels)}
}

// Run starts the Bubble Tea program and blocks until the user quits.
func (a *App) Run() error {
	_, err := tea.NewProgram(a.model, tea.WithAltScreen()).Run()
	return err
}
