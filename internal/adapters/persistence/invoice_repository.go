package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type InvoiceRepository struct {
	q *sqlc.Queries
}

func NewInvoiceRepository(q *sqlc.Queries) *InvoiceRepository {
	return &InvoiceRepository{q: q}
}

func (r *InvoiceRepository) ListByYear(ctx context.Context, year int) ([]model.Invoice, error) {
	headers, err := r.q.ListInvoicesByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	payRows, err := r.q.ListInvoicePaymentsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	linkRows, err := r.q.ListForecastInvoicesByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	paysByInv := map[int][]model.InvoicePayment{}
	for _, pr := range payRows {
		p, err := mapper.InvoicePaymentFromRow(pr)
		if err != nil {
			return nil, err
		}
		paysByInv[p.InvoiceID()] = append(paysByInv[p.InvoiceID()], p)
	}
	linksByInv := map[int][]model.ForecastInvoice{}
	for _, lr := range linkRows {
		l, err := mapper.ForecastInvoiceFromRow(lr)
		if err != nil {
			return nil, err
		}
		linksByInv[l.InvoiceID()] = append(linksByInv[l.InvoiceID()], l)
	}
	out := make([]model.Invoice, 0, len(headers))
	for _, hr := range headers {
		h, err := mapper.InvoiceHeaderFromRow(hr)
		if err != nil {
			return nil, err
		}
		inv, err := model.NewInvoice(h.ID(), h.Year(), h.Issuer(), h.Nif(), h.Number(),
			h.IssueDate(), h.NetAmount(), h.FilePath(), h.Notes(),
			paysByInv[h.ID()], linksByInv[h.ID()])
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

// Save inserts (id==0) or updates the header, then replaces children.
func (r *InvoiceRepository) Save(ctx context.Context, inv model.Invoice) (model.Invoice, error) {
	id := int64(inv.ID())
	if inv.ID() == 0 {
		newID, err := r.q.InsertInvoice(ctx, mapper.InvoiceToInsert(inv))
		if err != nil {
			return model.Invoice{}, err
		}
		id = newID
		inv = inv.WithID(int(newID))
	} else {
		if err := r.q.UpdateInvoice(ctx, mapper.InvoiceToUpdate(inv)); err != nil {
			return model.Invoice{}, err
		}
	}
	if err := r.q.DeletePaymentsByInvoice(ctx, id); err != nil {
		return model.Invoice{}, err
	}
	if err := r.q.DeleteForecastInvoicesByInvoice(ctx, id); err != nil {
		return model.Invoice{}, err
	}
	for _, p := range inv.Payments() {
		if err := r.q.InsertInvoicePayment(ctx, sqlc.InsertInvoicePaymentParams{
			InvoiceID: id, PaidOn: mapper.FormatDate(p.PaidOn()), Amount: p.Amount().String(),
		}); err != nil {
			return model.Invoice{}, err
		}
	}
	for _, l := range inv.Links() {
		if err := r.q.InsertForecastInvoice(ctx, sqlc.InsertForecastInvoiceParams{
			ForecastID: l.ForecastID(), InvoiceID: id, Amount: l.Amount().String(),
		}); err != nil {
			return model.Invoice{}, err
		}
	}
	return inv, nil
}

func (r *InvoiceRepository) Delete(ctx context.Context, invoiceID int) error {
	return r.q.DeleteInvoice(ctx, int64(invoiceID)) // children cascade
}

func (r *InvoiceRepository) ReplaceForYear(ctx context.Context, year int, invoices []model.Invoice) error {
	if err := r.q.DeleteInvoicesByYear(ctx, int64(year)); err != nil { // children cascade
		return err
	}
	for _, inv := range invoices {
		if _, err := r.Save(ctx, inv.WithID(0)); err != nil {
			return err
		}
	}
	return nil
}
