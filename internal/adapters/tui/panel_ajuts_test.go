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

func TestAjutsPanel_ImportActionAdvertised(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	found := false
	for _, a := range p.(ajutsPanel).Actions() {
		if a.Key == "i" {
			found = true
		}
	}
	if !found {
		t.Error("expected an 'i' import action")
	}
}

// TestAjutsPanel_CRUDActionsBothViews guards the Task 12 routing broadening:
// n/e/d must be advertised on both the Concessions view (Task 11) and the
// Factures view (Task 12), not just one.
func TestAjutsPanel_CRUDActionsBothViews(t *testing.T) {
	hasNED := func(actions []Action) bool {
		want := map[string]bool{"n": false, "e": false, "d": false}
		for _, a := range actions {
			if _, ok := want[a.Key]; ok {
				want[a.Key] = true
			}
		}
		return want["n"] && want["e"] && want["d"]
	}

	p := NewAjutsPanel(Deps{}).(ajutsPanel)
	p.view = ajutsConcessions
	if !hasNED(p.Actions()) {
		t.Error("expected n/e/d actions on the Concessions view")
	}
	p.view = ajutsInvoices
	if !hasNED(p.Actions()) {
		t.Error("expected n/e/d actions on the Factures view")
	}
}
