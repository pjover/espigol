// Package tui is the admin-facing terminal UI driving adapter. In phase 1 it
// is a minimal Bubble Tea program; panels and keymaps arrive in a later phase.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/config"
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg *config.Config
}

// NewModel builds the initial model.
func NewModel(cfg *config.Config) Model {
	return Model{cfg: cfg}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

var titleStyle = lipgloss.NewStyle().Bold(true)

// View implements tea.Model.
func (m Model) View() string {
	return titleStyle.Render("Espígol") + "\n\nprem q per sortir\n"
}

// Run starts the Bubble Tea program.
func Run(cfg *config.Config) error {
	_, err := tea.NewProgram(NewModel(cfg)).Run()
	return err
}
