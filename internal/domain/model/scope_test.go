package model

import "testing"

func TestNewSectionScope(t *testing.T) {
	s, err := NewSectionScope("oliva")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind() != ScopeSection || s.SectionCode() != "oliva" {
		t.Errorf("got kind=%q code=%q", s.Kind(), s.SectionCode())
	}
}

func TestNewSectionScope_RejectsEmpty(t *testing.T) {
	if _, err := NewSectionScope(""); err == nil {
		t.Fatal("expected error for empty section code")
	}
}

func TestCommonAndPartnerScopesHaveNoSection(t *testing.T) {
	if NewCommonScope().SectionCode() != "" || NewCommonScope().Kind() != ScopeCommon {
		t.Error("common scope wrong")
	}
	if NewPartnerScope().SectionCode() != "" || NewPartnerScope().Kind() != ScopePartner {
		t.Error("partner scope wrong")
	}
}

func TestNewScope_InvariantSectionIffSectionKind(t *testing.T) {
	if _, err := NewScope(ScopeCommon, "oliva"); err == nil {
		t.Error("COMMON with a section code must error")
	}
	if _, err := NewScope(ScopeSection, ""); err == nil {
		t.Error("SECTION without a section code must error")
	}
	if _, err := NewScope(ScopeSection, "oliva"); err != nil {
		t.Errorf("SECTION with code must succeed: %v", err)
	}
}
