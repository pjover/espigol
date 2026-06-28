package services

import (
	"strconv"
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func names(ids ...int) map[int]string {
	m := map[int]string{}
	for _, id := range ids {
		m[id] = "P" + itoa(id)
	}
	return m
}

// itoa is the shared test helper for partner-id strings (handles multi-digit ids, e.g. 11).
func itoa(i int) string { return strconv.Itoa(i) }

func allocByID(r fairShareResult) map[int]string {
	out := map[int]string{}
	for _, a := range r.allocations {
		out[a.PartnerID] = a.Allocated.String()
	}
	return out
}

func TestDistribute_NoExcess_EveryoneFull(t *testing.T) {
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(200)}
	r := distribute(model.MoneyOf(1000), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "100.00" || got[2] != "200.00" {
		t.Errorf("allocations = %v, want full", got)
	}
	if r.finalRemainder.String() != "700.00" {
		t.Errorf("finalRemainder = %q, want 700.00", r.finalRemainder.String())
	}
}

func TestDistribute_Excess_CapsHighRequesters(t *testing.T) {
	// budget 300, three partners want 100/100/400 (total 600 > 300).
	// Round 1: mean=100. p1,p2 (=100) fixed, budget 100 left, 1 unfixed.
	// Round 2: mean=100. p3 alloc 400 > 100 -> none newly fixed -> cap p3 at 100.
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(100), 3: model.MoneyOf(400)}
	r := distribute(model.MoneyOf(300), totals, names(1, 2, 3))
	got := allocByID(r)
	if got[1] != "100.00" || got[2] != "100.00" || got[3] != "100.00" {
		t.Errorf("allocations = %v, want 100/100/100", got)
	}
	if r.finalRemainder.String() != "0.00" {
		t.Errorf("finalRemainder = %q, want 0.00", r.finalRemainder.String())
	}
}

func TestDistribute_AllAboveMean_CappedEqually(t *testing.T) {
	// budget 300, two partners want 400/500 -> both above mean 150 -> cap both at 150.
	totals := map[int]model.Money{1: model.MoneyOf(400), 2: model.MoneyOf(500)}
	r := distribute(model.MoneyOf(300), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "150.00" || got[2] != "150.00" {
		t.Errorf("allocations = %v, want 150/150", got)
	}
	if r.finalRemainder.String() != "0.00" {
		t.Errorf("finalRemainder = %q, want 0.00", r.finalRemainder.String())
	}
}

func TestDistribute_NonPositivePool_NoClamp(t *testing.T) {
	// negative remainder: everyone capped at a negative mean, no clamp to zero.
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(100)}
	r := distribute(model.MoneyOf(-10), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "-5.00" || got[2] != "-5.00" {
		t.Errorf("allocations = %v, want -5.00/-5.00 (no clamp)", got)
	}
}

func TestDistribute_Empty(t *testing.T) {
	r := distribute(model.MoneyOf(50), map[int]model.Money{}, map[int]string{})
	if len(r.allocations) != 0 || r.finalRemainder.String() != "50.00" {
		t.Errorf("empty: allocations=%d finalRemainder=%q", len(r.allocations), r.finalRemainder.String())
	}
}
