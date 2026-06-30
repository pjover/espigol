package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/domain/model"
)

// Colour palette. Kept centralised so panels (Task 11/12) reuse the same
// look instead of inventing their own ad-hoc styles.
var (
	colorGrey   = lipgloss.Color("243")
	colorYellow = lipgloss.Color("220")
	colorGreen  = lipgloss.Color("42")
	colorBlue   = lipgloss.Color("39")
	colorRed    = lipgloss.Color("196")
	colorWhite  = lipgloss.Color("255")
)

// titleStyle renders top-level titles (the top bar business name, panel
// headers).
var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)

// dimStyle renders secondary/inactive text (unfocused panel titles, hints).
var dimStyle = lipgloss.NewStyle().Foreground(colorGrey)

// redStyle highlights errors or destructive state.
var redStyle = lipgloss.NewStyle().Foreground(colorRed)

// focusedPanelStyle highlights the currently focused panel title in the
// left-hand column.
var focusedPanelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)

// helpStyle renders the bottom keybinding/help line.
var helpStyle = lipgloss.NewStyle().Foreground(colorGrey)

// stateStyle returns the lipgloss style used to render a
// model.WindowState badge: DRAFT is grey/yellow, OPEN is green, CLOSED is
// blue.
func stateStyle(state model.WindowState) lipgloss.Style {
	switch state {
	case model.WindowDraft:
		return lipgloss.NewStyle().Foreground(colorYellow).Background(colorGrey)
	case model.WindowOpen:
		return lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	case model.WindowClosed:
		return lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}
