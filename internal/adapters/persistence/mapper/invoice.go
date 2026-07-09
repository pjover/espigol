package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func stringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func InvoiceHeaderFromRow(r sqlc.Invoice) (model.Invoice, error) {
	net, err := model.MoneyFromString(r.NetAmount)
	if err != nil {
		return model.Invoice{}, err
	}
	issued, err := ParseDate(r.IssueDate)
	if err != nil {
		return model.Invoice{}, err
	}
	return model.NewInvoice(int(r.ID), int(r.Year), r.Issuer, r.Nif, r.Number, issued,
		net, stringPtr(r.FilePath), stringPtr(r.Notes), nil, nil)
}

func InvoicePaymentFromRow(r sqlc.InvoicePayment) (model.InvoicePayment, error) {
	amt, err := model.MoneyFromString(r.Amount)
	if err != nil {
		return model.InvoicePayment{}, err
	}
	paid, err := ParseDate(r.PaidOn)
	if err != nil {
		return model.InvoicePayment{}, err
	}
	return model.NewInvoicePayment(int(r.ID), int(r.InvoiceID), paid, amt), nil
}

func ForecastInvoiceFromRow(r sqlc.ForecastInvoice) (model.ForecastInvoice, error) {
	amt, err := model.MoneyFromString(r.Amount)
	if err != nil {
		return model.ForecastInvoice{}, err
	}
	return model.NewForecastInvoice(r.ForecastID, int(r.InvoiceID), amt)
}

func InvoiceToInsert(inv model.Invoice) sqlc.InsertInvoiceParams {
	return sqlc.InsertInvoiceParams{
		Year:      int64(inv.Year()),
		Issuer:    inv.Issuer(),
		Nif:       inv.Nif(),
		Number:    inv.Number(),
		IssueDate: FormatDate(inv.IssueDate()),
		NetAmount: inv.NetAmount().String(),
		FilePath:  nullString(inv.FilePath()),
		Notes:     nullString(inv.Notes()),
	}
}

func InvoiceToUpdate(inv model.Invoice) sqlc.UpdateInvoiceParams {
	return sqlc.UpdateInvoiceParams{
		Year:      int64(inv.Year()),
		Issuer:    inv.Issuer(),
		Nif:       inv.Nif(),
		Number:    inv.Number(),
		IssueDate: FormatDate(inv.IssueDate()),
		NetAmount: inv.NetAmount().String(),
		FilePath:  nullString(inv.FilePath()),
		Notes:     nullString(inv.Notes()),
		ID:        int64(inv.ID()),
	}
}
