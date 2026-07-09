package tui

import (
	"testing"
)

func TestAjutsPanel_Title(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	if p.Title() != "Ajuts" {
		t.Errorf("Title = %q, want Ajuts", p.Title())
	}
}

func TestAjutsPanel_EmptyView(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	out := p.View(80, 10)
	if out == "" {
		t.Error("expected non-empty view")
	}
}
