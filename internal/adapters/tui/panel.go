package tui

import tea "github.com/charmbracelet/bubbletea"

// errDetail renders a panel's mutation/load error (p.err) as a red "Error: "
// line for use in Detail(), or "" if err is nil. Centralised here so the
// Anys/Socis/Seccions panels (and Task 12's) render mutation failures the
// same way instead of each inventing its own framing.
func errDetail(err error) string {
	if err == nil {
		return ""
	}
	return redStyle.Render("Error: " + err.Error())
}

// Action describes a single-letter keybinding a panel offers, used to render
// the bottom help/action line and to document the panel's behaviour.
type Action struct {
	Key   string
	Label string
}

// Panel is the contract every entity panel (Anys, Socis, Seccions, Tipus i
// subtipus, Previsions, Informes — Tasks 11/12) implements. The root model
// (app.go) holds a []Panel and delegates navigation, input and rendering to
// the focused one.
//
// Extension point: to add a new panel, construct it (typically with a
// constructor like NewYearsPanel(deps Deps) Panel) and append it to the
// slice passed into NewApp's panels argument — see the "Panel extension
// point" doc comment on NewApp in app.go.
type Panel interface {
	// Title is the short Catalan label shown in the left-hand panel list.
	Title() string

	// Update handles a Bubble Tea message and returns the (possibly new)
	// panel state plus an optional command. Panels are treated as
	// immutable value-ish types: callers must store the returned Panel.
	Update(tea.Msg) (Panel, tea.Cmd)

	// View renders the panel's main content area at the given size.
	View(width, height int) string

	// Detail renders extra information about the current selection (e.g.
	// shown in a side/bottom area by the root layout).
	Detail() string

	// Actions returns the panel's current single-letter keybindings, used
	// to build the bottom help line. It may change dynamically (e.g. a
	// taxonomy panel hides mutating actions outside DRAFT windows).
	Actions() []Action
}
