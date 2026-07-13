package services

import (
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
)

// ProjecteData is the grouped view of a year's enabled forecasts shared by both
// Consorci documents: Tipus (A/B) -> Apartat (subtype) -> Concept (forecasts
// merged by concept name, summed, with the contributing CP codes).
type ProjecteData struct {
	Year  int
	Tipus []TipusProjecte
	Total model.Money
}

type TipusProjecte struct {
	Code     string
	Label    string
	Category model.ExpenseCategory
	Apartats []ApartatProjecte
	Total    model.Money
}

type ApartatProjecte struct {
	Code     string
	Label    string
	Concepts []ConcepteProjecte
	Total    model.Money
}

type ConcepteProjecte struct {
	Name  string
	CPs   []string
	Total model.Money
}

// ProjecteInput is everything ComputeProjecte needs (assembled by the caller).
type ProjecteInput struct {
	Year      int
	Forecasts []model.ExpenseForecast
	Types     []model.ExpenseType
	Subtypes  []model.ExpenseSubtype
}

// ComputeProjecte groups the enabled forecasts by subtype -> concept, sums the
// amounts, collects+sorts CP codes, resolves apartat/tipus labels from the
// taxonomy, and orders tipus (CURRENT then INVESTMENT), apartats (by code) and
// concepts (by name). Pure: no I/O.
func ComputeProjecte(in ProjecteInput) ProjecteData {
	subLabel := map[string]string{}
	subType := map[string]string{}
	for _, s := range in.Subtypes {
		subLabel[s.Code()] = s.Label()
		subType[s.Code()] = s.TypeCode()
	}
	typeLabel := map[string]string{}
	typeCat := map[string]model.ExpenseCategory{}
	for _, t := range in.Types {
		typeLabel[t.Code()] = t.Label()
		typeCat[t.Code()] = t.Category()
	}

	type acc struct {
		total model.Money
		cps   []string
	}
	bySubtype := map[string]map[string]*acc{}
	for _, f := range in.Forecasts {
		if !f.Enabled() {
			continue
		}
		sc := f.SubtypeCode()
		concepts, ok := bySubtype[sc]
		if !ok {
			concepts = map[string]*acc{}
			bySubtype[sc] = concepts
		}
		a, ok := concepts[f.Concept()]
		if !ok {
			a = &acc{total: model.ZeroMoney()}
			concepts[f.Concept()] = a
		}
		a.total = a.total.Plus(f.GrossAmount())
		a.cps = append(a.cps, f.ID())
	}

	apartatBySub := map[string]ApartatProjecte{}
	for sc, concepts := range bySubtype {
		names := make([]string, 0, len(concepts))
		for n := range concepts {
			names = append(names, n)
		}
		sort.Strings(names)
		ap := ApartatProjecte{Code: sc, Label: labelOr(subLabel, sc), Total: model.ZeroMoney()}
		for _, n := range names {
			a := concepts[n]
			cps := append([]string(nil), a.cps...)
			sort.Strings(cps)
			ap.Concepts = append(ap.Concepts, ConcepteProjecte{Name: n, CPs: cps, Total: a.total})
			ap.Total = ap.Total.Plus(a.total)
		}
		apartatBySub[sc] = ap
	}

	subs := make([]string, 0, len(apartatBySub))
	for sc := range apartatBySub {
		subs = append(subs, sc)
	}
	sort.Strings(subs)

	tipusByCode := map[string]*TipusProjecte{}
	tipusOrder := []string{}
	for _, sc := range subs {
		tc := subType[sc]
		tp, ok := tipusByCode[tc]
		if !ok {
			tp = &TipusProjecte{Code: tc, Label: labelOr(typeLabel, tc), Category: typeCat[tc], Total: model.ZeroMoney()}
			tipusByCode[tc] = tp
			tipusOrder = append(tipusOrder, tc)
		}
		ap := apartatBySub[sc]
		tp.Apartats = append(tp.Apartats, ap)
		tp.Total = tp.Total.Plus(ap.Total)
	}

	sort.SliceStable(tipusOrder, func(i, j int) bool {
		ti, tj := tipusByCode[tipusOrder[i]], tipusByCode[tipusOrder[j]]
		if ri, rj := categoryRank(ti.Category), categoryRank(tj.Category); ri != rj {
			return ri < rj
		}
		return ti.Code < tj.Code
	})

	out := ProjecteData{Year: in.Year, Total: model.ZeroMoney()}
	for _, tc := range tipusOrder {
		tp := tipusByCode[tc]
		out.Tipus = append(out.Tipus, *tp)
		out.Total = out.Total.Plus(tp.Total)
	}
	return out
}

func labelOr(m map[string]string, code string) string {
	if l, ok := m[code]; ok && l != "" {
		return l
	}
	return code
}

func categoryRank(c model.ExpenseCategory) int {
	if c == model.CategoryInvestment {
		return 1
	}
	return 0
}
