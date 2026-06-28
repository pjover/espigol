package model

import (
	"testing"
	"time"
)

func TestNewPartner_Valid(t *testing.T) {
	p, err := NewPartner(1, "Pau", "Bosch", "X1", "p@e.cat", "600", Productor, 13937,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != 1 || p.Name() != "Pau" || p.PartnerType() != Productor {
		t.Errorf("accessors wrong: %+v", p)
	}
	if p.WithBoardMember(true).BoardMember() != true {
		t.Error("WithBoardMember failed")
	}
	if p.BoardMember() != false {
		t.Error("WithBoardMember mutated the original")
	}
}

func TestNewPartner_RejectsNegativeID(t *testing.T) {
	if _, err := NewPartner(-1, "x", "y", "v", "e", "m", Productor, 0, time.Now(), false); err == nil {
		t.Fatal("expected error for negative id")
	}
}
