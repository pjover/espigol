package model

import "testing"

func TestNewBoardAuthorization(t *testing.T) {
	a, err := NewBoardAuthorization(7, ScopeSection, "oliva")
	if err != nil || a.ScopeKind() != ScopeSection || a.SectionCode() != "oliva" {
		t.Fatalf("got (%+v,%v)", a, err)
	}
	if _, err := NewBoardAuthorization(7, ScopeCommon, ""); err != nil {
		t.Errorf("COMMON without section should be valid: %v", err)
	}
	if _, err := NewBoardAuthorization(7, ScopePartner, ""); err == nil {
		t.Error("PARTNER scope must be rejected for board authorization")
	}
	if _, err := NewBoardAuthorization(7, ScopeSection, ""); err == nil {
		t.Error("SECTION without code must be rejected")
	}
}
