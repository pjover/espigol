package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

const ajutsDateLayout = "2006-01-02"

// parsePayments parses "YYYY-MM-DD:amount;YYYY-MM-DD:amount".
func parsePayments(s string) ([]application.PaymentInput, error) {
	var out []application.PaymentInput
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		date, amtStr, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("pagament %q: format esperat DATA:IMPORT", part)
		}
		d, err := time.Parse(ajutsDateLayout, strings.TrimSpace(date))
		if err != nil {
			return nil, fmt.Errorf("pagament %q: data invàlida: %w", part, err)
		}
		amt, err := model.MoneyFromString(strings.TrimSpace(amtStr))
		if err != nil {
			return nil, fmt.Errorf("pagament %q: import invàlid: %w", part, err)
		}
		out = append(out, application.PaymentInput{PaidOn: d, Amount: amt})
	}
	return out, nil
}

// parseLinks parses "CPid:amount;CPid:amount".
func parseLinks(s string) ([]application.LinkInput, error) {
	var out []application.LinkInput
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, amtStr, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("enllaç %q: format esperat CPID:IMPORT", part)
		}
		amt, err := model.MoneyFromString(strings.TrimSpace(amtStr))
		if err != nil {
			return nil, fmt.Errorf("enllaç %q: import invàlid: %w", part, err)
		}
		out = append(out, application.LinkInput{ForecastID: strings.TrimSpace(id), Amount: amt})
	}
	return out, nil
}

func formatPayments(pays []model.InvoicePayment) string {
	var parts []string
	for _, p := range pays {
		parts = append(parts, fmt.Sprintf("%s:%s", p.PaidOn().Format(ajutsDateLayout), p.Amount()))
	}
	return strings.Join(parts, ";")
}

func formatLinks(links []model.ForecastInvoice) string {
	var parts []string
	for _, l := range links {
		parts = append(parts, fmt.Sprintf("%s:%s", l.ForecastID(), l.Amount()))
	}
	return strings.Join(parts, ";")
}

func (p ajutsPanel) invoiceForm(existing *model.Invoice) formModal {
	title := "Nova factura"
	id := 0
	issuer, nif, number, date, net, file, notes, pays, links := "", "", "", "", "0.00", "", "", "", ""
	if existing != nil {
		title = "Edita factura"
		id = existing.ID()
		issuer = existing.Issuer()
		nif = existing.Nif()
		number = existing.Number()
		date = existing.IssueDate().Format(ajutsDateLayout)
		net = existing.NetAmount().String()
		if existing.FilePath() != nil {
			file = *existing.FilePath()
		}
		if existing.Notes() != nil {
			notes = *existing.Notes()
		}
		pays = formatPayments(existing.Payments())
		links = formatLinks(existing.Links())
	}

	fields := []formFieldDef{
		{Label: "Proveïdor", Placeholder: "Ribot", Value: issuer},
		{Label: "NIF", Placeholder: "B12345678", Value: nif},
		{Label: "Núm", Placeholder: "FD-39521", Value: number},
		{Label: "Data", Placeholder: "2025-03-14", Value: date},
		{Label: "Import", Placeholder: "0.00", Value: net},
		{Label: "Arxiu", Placeholder: "factura.pdf", Value: file},
		{Label: "Notes", Placeholder: "", Value: notes},
		{Label: "Pagaments", Placeholder: "2025-04-01:500.00", Value: pays},
		{Label: "Enllaços", Placeholder: "CP25030:500.00", Value: links},
	}

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		net, err := parseMoney(values["Import"])
		if err != nil {
			return nil
		}
		issued, err := time.Parse(ajutsDateLayout, strings.TrimSpace(values["Data"]))
		if err != nil {
			return nil
		}
		pays, err := parsePayments(values["Pagaments"])
		if err != nil {
			return nil
		}
		links, err := parseLinks(values["Enllaços"])
		if err != nil {
			return nil
		}
		in := application.InvoiceInput{
			ID: id, Year: year,
			Issuer: values["Proveïdor"], Nif: strings.TrimSpace(values["NIF"]),
			Number: strings.TrimSpace(values["Núm"]), IssueDate: issued, NetAmount: net,
			FilePath: strings.TrimSpace(values["Arxiu"]), Notes: values["Notes"],
			Payments: pays, Links: links,
		}
		return p.reloadCmd(func(ctx context.Context) error {
			_, err := p.deps.Reconciliation.SaveInvoice(ctx, in)
			return err
		})
	}
	return newFormModal(title, fields, onSubmit)
}

func (p ajutsPanel) handleInvoiceKey(key string) (Panel, tea.Cmd) {
	switch key {
	case "n":
		return p, openModalCmd(p.invoiceForm(nil))
	case "e":
		if p.selected < 0 || p.selected >= len(p.invoices) {
			return p, nil
		}
		inv := p.invoices[p.selected]
		return p, openModalCmd(p.invoiceForm(&inv))
	case "d":
		if p.selected < 0 || p.selected >= len(p.invoices) {
			return p, nil
		}
		inv := p.invoices[p.selected]
		id, num := inv.ID(), inv.Number()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.DeleteInvoice(ctx, id)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar la factura %s?", num), onConfirm))
	}
	return p, nil
}
