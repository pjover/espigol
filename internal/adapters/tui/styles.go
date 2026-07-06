package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/domain/model"
)

var (
	colorFgMuted    = lipgloss.AdaptiveColor{Light: "#5c6370", Dark: "#565f89"}
	colorFgEmphasis = lipgloss.AdaptiveColor{Light: "#1a1b26", Dark: "#e0e0e0"}

	colorBgSelection = lipgloss.AdaptiveColor{Light: "#dce0f5", Dark: "#364a82"}

	colorAccent = lipgloss.AdaptiveColor{Light: "#2055c7", Dark: "#7aa2f7"}

	colorError   = lipgloss.AdaptiveColor{Light: "#c0392b", Dark: "#f7768e"}
	colorWarning = lipgloss.AdaptiveColor{Light: "#a06800", Dark: "#e0af68"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#2e7d32", Dark: "#9ece6a"}
	colorInfo    = lipgloss.AdaptiveColor{Light: "#0277bd", Dark: "#7dcfff"}

	colorDraftBg = lipgloss.AdaptiveColor{Light: "#fffde7", Dark: "#3d3200"}
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

// truncate shortens s to at most max runes, replacing the last rune with "…"
// if trimmed. Safe on multi-byte Unicode.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

// scrollOffset returns the top-of-viewport index that keeps selected roughly
// centered and clamped so the viewport never extends past the list bounds.
func scrollOffset(selected, count, height int) int {
	if height <= 0 || count <= height {
		return 0
	}
	off := selected - height/2
	if off < 0 {
		off = 0
	}
	if off+height > count {
		off = count - height
	}
	return off
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
