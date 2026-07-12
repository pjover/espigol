package services

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func d(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatalf("money %q: %v", s, err)
	}
	return m
}

func mkForecast(t *testing.T, id string, partnerID int, gross string, scope model.ExpenseScope, subtype string) model.ExpenseForecast {
	t.Helper()
	planned := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	p, err := model.NewPartner(partnerID, "Soci", "Soci", "", "", "soci@e.test", "", model.Productor, 0, planned, false)
	if err != nil {
		t.Fatalf("partner %d: %v", partnerID, err)
	}
	f, err := model.NewExpenseForecast(id, p, "Concepte "+id, "", d(t, gross), model.ZeroMoney(),
		nil, planned, 2026, subtype, scope, planned, true)
	if err != nil {
		t.Fatalf("forecast %s: %v", id, err)
	}
	return f
}

func mkPartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "Soci "+itoa(id), "Soci "+itoa(id), "", "", "soci@x.test", "", model.Productor, 0,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatalf("partner %d: %v", id, err)
	}
	return p
}

func section(t *testing.T, code, label string, order int) model.Section {
	t.Helper()
	s, err := model.NewSection(code, label, true, order)
	if err != nil {
		t.Fatalf("section %s: %v", code, err)
	}
	return s
}

// A small CURRENT-only scenario: common 100, one section 'a' total 30, socis 0.
// limit 200 -> common remainder 100; availableForSections 100; sectionsRemainder 70.
func TestCompute_CommonSectionsSocis_Basic(t *testing.T) {
	common := mkForecast(t, "CP26001", 1, "100.00", model.NewCommonScope(), "a1")
	secScope, _ := model.NewSectionScope("a")
	sec := mkForecast(t, "CP26002", 1, "30.00", secScope, "a1")
	in := AllocationInput{
		Year:            2026,
		Forecasts:       []model.ExpenseForecast{common, sec},
		Partners:        []model.Partner{mkPartner(t, 1)},
		Sections:        []model.Section{section(t, "a", "Secció A", 1)},
		Memberships:     nil,
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent},
		CurrentLimit:    model.MoneyOf(200),
		InvestmentLimit: model.MoneyOf(0),
	}

	rd, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rd.Categories) != 2 {
		t.Fatalf("want 2 categories, got %d", len(rd.Categories))
	}
	cur := rd.Categories[0]
	if cur.Category != model.CategoryCurrent {
		t.Errorf("first category should be CURRENT")
	}
	if cur.Common.Total.String() != "100.00" || cur.Common.Remainder.String() != "100.00" {
		t.Errorf("common: total=%s remainder=%s", cur.Common.Total, cur.Common.Remainder)
	}
	if cur.Sections.Available.String() != "100.00" || cur.Sections.Total.String() != "30.00" || cur.Sections.Remainder.String() != "70.00" {
		t.Errorf("sections: avail=%s total=%s rem=%s", cur.Sections.Available, cur.Sections.Total, cur.Sections.Remainder)
	}
	if len(cur.Sections.SectionDetails) != 1 || cur.Sections.SectionDetails[0].Label != "Secció A" {
		t.Errorf("section detail wrong: %+v", cur.Sections.SectionDetails)
	}
	if cur.Sections.Partners.GrandTotal.String() != "0.00" || cur.Sections.Partners.FinalRemainder.String() != "70.00" {
		t.Errorf("socis: grand=%s final=%s", cur.Sections.Partners.GrandTotal, cur.Sections.Partners.FinalRemainder)
	}
	if cur.Warning != nil {
		t.Errorf("warning should be nil for positive remainder")
	}
}

func modelTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

// Excess socis with capping + per-item proration.
func TestCompute_SocisCappedProration(t *testing.T) {
	// sectionsRemainder must be small so socis exceed it. No common, no sections.
	// limit 100 -> availableForSections 100, sectionsRemainder 100.
	// two partners: p1 wants 40 (one item), p2 wants 400 (one item). total 440 > 100.
	// round1 mean 50: p1(40)<=50 fixed, budget 60, 1 unfixed.
	// round2 mean 60: p2(400)>60 none newly fixed -> cap p2 at 60.
	pScope := model.NewPartnerScope()
	f1 := mkForecast(t, "CP26010", 1, "40.00", pScope, "a1")
	f2 := mkForecast(t, "CP26011", 2, "400.00", pScope, "a1")
	in := AllocationInput{
		Year:            2026,
		Forecasts:       []model.ExpenseForecast{f1, f2},
		Partners:        []model.Partner{mkPartner(t, 1), mkPartner(t, 2)},
		Sections:        nil,
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent},
		CurrentLimit:    model.MoneyOf(100),
		InvestmentLimit: model.MoneyOf(0),
	}
	rd, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	pd := rd.Categories[0].Sections.Partners
	if !pd.HasExcess {
		t.Errorf("expected HasExcess true")
	}
	allocated := map[int]string{}
	for _, a := range pd.Allocations {
		allocated[a.PartnerID] = a.Allocated.String()
	}
	if allocated[1] != "40.00" || allocated[2] != "60.00" {
		t.Errorf("allocations = %v, want p1 40.00 p2 60.00", allocated)
	}
	// p2 is capped: its single item (gross 400) prorated by 60/400 -> approved 60.00
	var p2 *struct{}
	for _, det := range pd.PartnerDetails {
		if det.Name == "Soci 2" {
			if !det.IsCapped || det.MaxAuthorized.String() != "60.00" {
				t.Errorf("p2 detail: capped=%v max=%s", det.IsCapped, det.MaxAuthorized)
			}
			if det.Items[0].ApprovedAmount.String() != "60.00" {
				t.Errorf("p2 item approved = %s, want 60.00", det.Items[0].ApprovedAmount)
			}
			p2 = &struct{}{}
		}
	}
	if p2 == nil {
		t.Errorf("p2 detail not found")
	}
}
