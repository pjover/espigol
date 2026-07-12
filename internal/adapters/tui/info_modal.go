package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// infoModal shows the outcome of an admin action (report generated, imported,
// backup, error, …) and dismisses on Enter/Esc. onClose, if set, runs when the
// modal closes — used to refresh a panel after the user acknowledges (e.g.
// reload the years-with-reports list). Closing is signalled to the root model
// via modalClosedMsg, exactly like confirmModal.
type infoModal struct {
	message string
	onClose tea.Cmd
}

func newInfoModal(message string, onClose tea.Cmd) infoModal {
	return infoModal{message: message, onClose: onClose}
}

// Init implements tea.Model.
func (m infoModal) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m infoModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "enter", "esc":
		if m.onClose != nil {
			return m, tea.Batch(m.onClose, closeModalCmd)
		}
		return m, closeModalCmd
	}
	return m, nil
}

// View implements tea.Model.
func (m infoModal) View() string {
	return modalStyle.Render(m.message + "\n\n" + helpStyle.Render("[Enter] tanca"))
}
