package model

import "testing"

func TestParsePartnerType(t *testing.T) {
	pt, err := ParsePartnerType("Productor")
	if err != nil || pt != Productor {
		t.Fatalf("got (%v,%v), want Productor", pt, err)
	}
	if _, err := ParsePartnerType("Nope"); err == nil {
		t.Fatal("expected error for unknown partner type")
	}
}

func TestParseWindowState(t *testing.T) {
	if s, err := ParseWindowState("OPEN"); err != nil || s != WindowOpen {
		t.Fatalf("got (%v,%v), want OPEN", s, err)
	}
	if _, err := ParseWindowState("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseExpenseCategory(t *testing.T) {
	if c, err := ParseExpenseCategory("INVESTMENT"); err != nil || c != CategoryInvestment {
		t.Fatalf("got (%v,%v), want INVESTMENT", c, err)
	}
}

func TestParseAuditKind(t *testing.T) {
	if k, err := ParseAuditKind("MIGRATION"); err != nil || k != AuditMigration {
		t.Fatalf("got (%v,%v), want MIGRATION", k, err)
	}
}
