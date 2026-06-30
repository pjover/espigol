package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmModal is a yes/no modal. Pressing "y" runs onConfirm (if set) and
// closes the modal; "n" or "esc" cancels without running it. Either way the
// modal closing is signalled to the root model via modalClosedMsg, which is
// how the root knows to clear m.modal (see app.go's Update).
type confirmModal struct {
	message   string
	onConfirm tea.Cmd
}

// newConfirmModal builds a confirmation modal with the given Catalan
// message; onConfirm may be nil if there's nothing to run on "y" beyond
// closing the modal.
func newConfirmModal(message string, onConfirm tea.Cmd) confirmModal {
	return confirmModal{message: message, onConfirm: onConfirm}
}

// Init implements tea.Model.
func (m confirmModal) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m confirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y":
		if m.onConfirm != nil {
			return m, tea.Batch(m.onConfirm, closeModalCmd)
		}
		return m, closeModalCmd
	case "n", "esc":
		return m, closeModalCmd
	}
	return m, nil
}

// View implements tea.Model.
func (m confirmModal) View() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Foreground(colorWhite)
	return box.Render(fmt.Sprintf("%s\n\n[y] sí    [n] no", m.message))
}
