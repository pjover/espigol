package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/persistence/backup"
)

// backupSelectModal lets the admin pick a backup file to restore. Enter stages
// the chosen file (StageRestore) and closes; Esc cancels. It follows the same
// modalClosedMsg/openModalCmd convention as confirmModal.
type backupSelectModal struct {
	deps   Deps
	files  []backup.BackupFile
	cursor int
}

func newBackupSelectModal(deps Deps, files []backup.BackupFile) backupSelectModal {
	return backupSelectModal{deps: deps, files: files}
}

// Init implements tea.Model.
func (m backupSelectModal) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m backupSelectModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
		return m, nil
	case "enter":
		if len(m.files) == 0 {
			return m, closeModalCmd
		}
		chosen := m.files[m.cursor]
		return m, tea.Batch(stageRestoreCmd(m.deps, chosen.Path), closeModalCmd)
	case "esc":
		return m, closeModalCmd
	}
	return m, nil
}

// View implements tea.Model.
func (m backupSelectModal) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Restaura una còpia de seguretat"))
	b.WriteString("\n\n")
	for i, f := range m.files {
		line := "  " + f.Name
		if i == m.cursor {
			line = focusedPanelStyle.Render("> " + f.Name)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑↓: mou · enter: restaura · esc: cancel·la"))
	return modalStyle.Render(b.String())
}

// restoreStagedMsg carries the outcome of stageRestoreCmd.
type restoreStagedMsg struct {
	name string
	err  error
}

func stageRestoreCmd(deps Deps, path string) tea.Cmd {
	return func() tea.Msg {
		err := deps.Backup.StageRestore(path)
		return restoreStagedMsg{name: filepath.Base(path), err: err}
	}
}
