package report

import (
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func projData2025(t *testing.T) services.ProjecteData {
	t.Helper()
	m := func(s string) model.Money {
		v, err := model.MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	return services.ProjecteData{
		Year:  2025,
		Total: m("30168.47"),
		Tipus: []services.TipusProjecte{
			{
				Code: "A", Label: "Despeses corrents", Category: model.CategoryCurrent, Total: m("23557.73"),
				Apartats: []services.ApartatProjecte{
					{Code: "a2", Label: "Activitats d'informació i promoció", Total: m("8189.00"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Projecte de disseny i comunicació", CPs: []string{"CP25001"}, Total: m("8189.00")},
						}},
					{Code: "a6", Label: "Despeses de fertilitzants", Total: m("15368.73"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Adob foliar", CPs: []string{"CP25005"}, Total: m("1488.73")},
							{Name: "Adob orgànic", CPs: []string{"CP25006", "CP25007"}, Total: m("13880.00")},
						}},
				},
			},
			{
				Code: "B", Label: "Despeses d'inversió", Category: model.CategoryInvestment, Total: m("6610.74"),
				Apartats: []services.ApartatProjecte{
					{Code: "b1", Label: "Despeses d'adquisició de maquinària i materials", Total: m("6610.74"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Carretilla transportadora", CPs: []string{"CP25028"}, Total: m("6610.74")},
						}},
				},
			},
		},
	}
}

func TestPressupostRenderer_ContainsSummaryBreakdownAndCPs(t *testing.T) {
	out := string(PressupostRenderer{}.Render(projData2025(t)))

	mustContain(t, out, "# Pressupost del projecte d'actuació 2025")

	// Summary + grand total.
	mustContain(t, out, "## Resum per tipus de despesa")
	mustContain(t, out, "| A. Despeses corrents | a.2. Activitats d'informació i promoció | 8.189,00 € |")
	mustContain(t, out, "**Total general**")
	mustContain(t, out, "30.168,47 €")

	// Corrents breakdown: apartat heading, merged concept with both CPs.
	mustContain(t, out, "## Desglossament per conceptes de despeses corrents")
	mustContain(t, out, "### a.6. Despeses de fertilitzants")
	mustContain(t, out, "| Adob orgànic | CP25006, CP25007 | 13.880,00 € |")
	mustContain(t, out, "**Total a.6.**")

	// Inversions breakdown.
	mustContain(t, out, "## Desglossament per conceptes d'inversions")
	mustContain(t, out, "| Carretilla transportadora | CP25028 | 6.610,74 € |")
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing %q\n---\n%s", needle, haystack)
	}
}
