package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// parseForecastIDs splits a comma-separated CP-id list, trimming blanks.
func parseForecastIDs(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseMoney(s string) (model.Money, error) {
	return model.MoneyFromString(strings.TrimSpace(s))
}

// concessionForm builds the create/edit form. existing == nil means create
// (Grup field editable); otherwise edit (Grup fixed).
func (p ajutsPanel) concessionForm(existing *model.Concession) formModal {
	title := "Nova concessió"
	group, subtype, concept, demanat, concedit, forecasts := "", "", "", "0.00", "0.00", ""
	if existing != nil {
		title = "Edita concessió"
		group = existing.GroupCode()
		subtype = existing.SubtypeCode()
		concept = existing.Concept()
		demanat = existing.RequestedTotal().String()
		concedit = existing.GrantedAmount().String()
		forecasts = p.forecastIDsFor(existing.GroupCode())
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Grup", Placeholder: "A6-02", Value: group})
	}
	fields = append(fields,
		formFieldDef{Label: "Subtipus", Placeholder: "a6", Value: subtype},
		formFieldDef{Label: "Concepte", Placeholder: "Adob orgànic", Value: concept},
		formFieldDef{Label: "Demanat", Placeholder: "0.00", Value: demanat},
		formFieldDef{Label: "Concedit", Placeholder: "0.00", Value: concedit},
		formFieldDef{Label: "Previsions", Placeholder: "CP25008,CP25009", Value: forecasts},
	)

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		req, err := parseMoney(values["Demanat"])
		if err != nil {
			return nil
		}
		granted, err := parseMoney(values["Concedit"])
		if err != nil {
			return nil
		}
		gc := group
		if existing == nil {
			gc = strings.TrimSpace(values["Grup"])
		}
		in := application.ConcessionInput{
			Year: year, GroupCode: gc,
			SubtypeCode:    strings.TrimSpace(values["Subtipus"]),
			Concept:        values["Concepte"],
			RequestedTotal: req, GrantedAmount: granted,
			ForecastIDs: parseForecastIDs(values["Previsions"]),
		}
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.SaveConcession(ctx, in)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

// handleConcessionKey handles n/e/d while the Concessions view is active.
func (p ajutsPanel) handleConcessionKey(key string) (Panel, tea.Cmd) {
	switch key {
	case "n":
		return p, openModalCmd(p.concessionForm(nil))
	case "e":
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return p, nil
		}
		c := p.concessions[p.selected]
		return p, openModalCmd(p.concessionForm(&c))
	case "d":
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return p, nil
		}
		c := p.concessions[p.selected]
		gc := c.GroupCode()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.DeleteConcession(ctx, p.year, gc)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar la concessió %s?", gc), onConfirm))
	}
	return p, nil
}
