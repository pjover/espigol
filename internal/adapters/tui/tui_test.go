package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pjover/espigol/internal/config"
)

func TestView_ShowsCatalanQuitHint(t *testing.T) {
	m := NewModel(&config.Config{})
	if !strings.Contains(m.View(), "prem q per sortir") {
		t.Errorf("View() = %q, want it to contain the Catalan quit hint", m.View())
	}
}

func TestUpdate_QuitsOnQ(t *testing.T) {
	m := NewModel(&config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a command on 'q', got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestUpdate_QuitsOnCtrlC(t *testing.T) {
	m := NewModel(&config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command on ctrl+c, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}
