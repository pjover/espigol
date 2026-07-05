package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubPanel is a trivial Panel used to exercise root model navigation
// without depending on any real application service.
type stubPanel struct {
	title   string
	actions []Action
}

func (p stubPanel) Title() string { return p.title }

func (p stubPanel) Update(tea.Msg) (Panel, tea.Cmd) { return p, nil }

func (p stubPanel) View(width, height int) string { return "view:" + p.title }

func (p stubPanel) Detail() string { return "" }

func (p stubPanel) Actions() []Action { return p.actions }

func stubPanels() []Panel {
	return []Panel{
		stubPanel{title: "Anys", actions: []Action{{Key: "n", Label: "nou"}}},
		stubPanel{title: "Socis"},
		stubPanel{title: "Seccions"},
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestRootModel_StartsFocusedOnFirstPanel(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())
	if got := m.FocusedTitle(); got != "Anys" {
		t.Errorf("FocusedTitle() = %q, want %q", got, "Anys")
	}
}

func TestRootModel_NumberKeySwitchesPanel(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())

	// stubPanels has 3 panels: Anys(0), Socis(1), Seccions(2).
	// Press "2" → focus Socis.
	updated, _ := m.Update(keyMsg("2"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Socis" {
		t.Errorf("after '2', FocusedTitle() = %q, want %q", got, "Socis")
	}

	// Press "1" → back to Anys.
	updated, _ = m.Update(keyMsg("1"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Anys" {
		t.Errorf("after '1', FocusedTitle() = %q, want %q", got, "Anys")
	}

	// Press "6" → out of range (only 3 panels); focus stays on Anys.
	updated, _ = m.Update(keyMsg("6"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Anys" {
		t.Errorf("after out-of-range '6', FocusedTitle() = %q, want %q (unchanged)", got, "Anys")
	}
}

func TestRootModel_QReturnsQuitCmd(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())

	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("expected a non-nil command on 'q'")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestRootModel_CtrlCReturnsQuitCmd(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())

	_, cmd := m.Update(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("expected a non-nil command on ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestRootModel_ActionsAppearInHelpLine(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())
	if !strings.Contains(m.View(), "[n] nou") {
		t.Errorf("View() = %q, want it to contain the focused panel's action in [key] label format", m.View())
	}
}

func TestConfirmModal_YRunsOnConfirmAndCloses(t *testing.T) {
	ran := false
	modal := newConfirmModal("Segur?", func() tea.Msg {
		ran = true
		return nil
	})

	_, cmd := modal.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("expected a non-nil command on 'y'")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a tea.BatchMsg from 'y', got %T", msg)
	}
	sawClose := false
	for _, c := range batch {
		got := c()
		if _, ok := got.(modalClosedMsg); ok {
			sawClose = true
		}
	}
	if !ran {
		t.Error("expected onConfirm to have run")
	}
	if !sawClose {
		t.Error("expected the batch to include a modalClosedMsg")
	}
}

func TestConfirmModal_NCancelsWithoutRunning(t *testing.T) {
	ran := false
	modal := newConfirmModal("Segur?", func() tea.Msg {
		ran = true
		return nil
	})

	_, cmd := modal.Update(keyMsg("n"))
	if cmd == nil {
		t.Fatal("expected a non-nil command on 'n'")
	}
	if _, ok := cmd().(modalClosedMsg); !ok {
		t.Errorf("expected modalClosedMsg, got %T", cmd())
	}
	if ran {
		t.Error("expected onConfirm NOT to have run on 'n'")
	}
}

func TestConfirmModal_EscCancelsWithoutRunning(t *testing.T) {
	ran := false
	modal := newConfirmModal("Segur?", func() tea.Msg {
		ran = true
		return nil
	})

	_, cmd := modal.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected a non-nil command on 'esc'")
	}
	if _, ok := cmd().(modalClosedMsg); !ok {
		t.Errorf("expected modalClosedMsg, got %T", cmd())
	}
	if ran {
		t.Error("expected onConfirm NOT to have run on 'esc'")
	}
}

func TestRootModel_RoutesMsgsToActiveModalAndClearsOnClose(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())
	ran := false
	modal := newConfirmModal("Eliminar?", func() tea.Msg {
		ran = true
		return nil
	})

	updated, _ := m.Update(openModalMsg{modal: modal})
	m = updated.(rootModel)
	if m.modal == nil {
		t.Fatal("expected modal to be set after openModalMsg")
	}

	updated, cmd := m.Update(keyMsg("y"))
	m = updated.(rootModel)
	if cmd == nil {
		t.Fatal("expected a command from routing 'y' to the modal")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if _, ok := c().(modalClosedMsg); ok {
				updated, _ = m.Update(modalClosedMsg{})
				m = updated.(rootModel)
			}
		}
	}
	if !ran {
		t.Error("expected the modal's onConfirm to have run")
	}
	if m.modal != nil {
		t.Error("expected modal to be cleared after modalClosedMsg")
	}
}
