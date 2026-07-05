package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/domain/model"
)

var (
	colorFgMuted    = lipgloss.Color("#565f89")
	colorFgEmphasis = lipgloss.Color("#e0e0e0")

	colorBgSelection = lipgloss.Color("#364a82")

	colorAccent = lipgloss.Color("#7aa2f7")

	colorError   = lipgloss.Color("#f7768e")
	colorWarning = lipgloss.Color("#e0af68")
	colorSuccess = lipgloss.Color("#9ece6a")
	colorInfo    = lipgloss.Color("#7dcfff")

	colorDraftBg = lipgloss.Color("#3d3200")
)

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorFgEmphasis)
	dimStyle          = lipgloss.NewStyle().Foreground(colorFgMuted)
	redStyle          = lipgloss.NewStyle().Foreground(colorError)
	focusedPanelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpStyle         = lipgloss.NewStyle().Foreground(colorFgMuted)

	// sidebarInnerWidth is the content width inside the sidebar border+padding.
	// Outer rendered width = sidebarInnerWidth + 2 (Padding(0,1)) + 2 (border) = sidebarInnerWidth + 4.
	sidebarInnerWidth = 20

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFgMuted).
			Padding(0, 1).
			Width(sidebarInnerWidth)

	centerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
)

func stateStyle(state model.WindowState) lipgloss.Style {
	switch state {
	case model.WindowDraft:
		return lipgloss.NewStyle().Foreground(colorWarning).Background(colorDraftBg)
	case model.WindowOpen:
		return lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	case model.WindowClosed:
		return lipgloss.NewStyle().Foreground(colorInfo).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}

func stateBadge(state model.WindowState) string {
	switch state {
	case model.WindowDraft:
		return stateStyle(state).Render("● ESBORRANY")
	case model.WindowOpen:
		return stateStyle(state).Render("● OBERT")
	case model.WindowClosed:
		return stateStyle(state).Render("● TANCAT")
	default:
		return dimStyle.Render("● -")
	}
}
