// Package services — reconciliation.go is the Phase 2 pure algorithm that
// turns the year's Concession + Invoice data into a per-forecast
// AssignedSubsidy snapshot. It has zero I/O; orchestration lives in
// internal/application/reconciliation_service.go.
package services

import (
	"sort"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/shopspring/decimal"
)

// ReconciliationInput is everything ComputeReconciliation needs to compute
// per-forecast subsidies for a single year. The application service builds
// this from ports.RepoSet reads inside a TxManager.WithinTx.
type ReconciliationInput struct {
	Year        int
	Forecasts   []model.ExpenseForecast // ALL year's forecasts; algorithm filters Enabled==true
	Concessions []model.Concession
	Links       []model.ConcessionForecast // membership (year, groupCode, forecastID)
	Invoices    []model.Invoice            // aggregate: payments + links included
	Subtypes    []model.ExpenseSubtype     // year-scoped
	Types       []model.ExpenseType        // year-scoped (subtype→type→category lookup)
	Partners    []model.Partner
}

// ReconciliationData is the JSON-serialisable snapshot produced by
// ComputeReconciliation. Categories are ordered CURRENT then INVESTMENT.
// Empty categories/subtypes/concessions are omitted.
type ReconciliationData struct {
	Year       int                      `json:"year"`
	Categories []CategoryReconciliation `json:"categories"`
}

type CategoryReconciliation struct {
	Category     model.ExpenseCategory   `json:"category"`
	Requested    model.Money             `json:"requested"`
	Granted      model.Money             `json:"granted"`
	Executed     model.Money             `json:"executed"`
	Assigned     model.Money             `json:"assigned"`
	NetDeviation model.Money             `json:"netDeviation"` // Σ Subtype.Deviation
	Subtypes     []SubtypeReconciliation `json:"subtypes"`
}

type SubtypeReconciliation struct {
	Code        string                     `json:"code"`
	Label       string                     `json:"label"`
	Requested   model.Money                `json:"requested"`
	Granted     model.Money                `json:"granted"`
	Executed    model.Money                `json:"executed"`
	Assigned    model.Money                `json:"assigned"`
	Deviation   model.Money                `json:"deviation"` // Granted − Executed (raw)
	Concessions []ConcessionReconciliation `json:"concessions"`
}

type ConcessionReconciliation struct {
	GroupCode  string                    `json:"groupCode"`
	Concept    string                    `json:"concept"`
	Requested  model.Money               `json:"requested"`
	Granted    model.Money               `json:"granted"`
	Executed   model.Money               `json:"executed"`
	Assigned   model.Money               `json:"assigned"`
	Difference model.Money               `json:"difference"` // Granted − Executed
	Forecasts  []ForecastReconciliation  `json:"forecasts"`
}

type ForecastReconciliation struct {
	ForecastID     string                `json:"forecastId"`
	PartnerID      int                   `json:"partnerId"`
	Concept        string                `json:"concept"`
	GrossAmount    model.Money           `json:"grossAmount"`
	ApprovedAmount model.Money           `json:"approvedAmount"`
	Executed       model.Money           `json:"executed"`
	Pending        model.Money           `json:"pending"`
	Assigned       model.Money           `json:"assigned"`
	Status         ForecastReconStatus   `json:"status"`
	Invoices       []InvoiceContribution `json:"invoices"`
}

type InvoiceContribution struct {
	InvoiceID    int         `json:"invoiceId"`
	Issuer       string      `json:"issuer"`
	Number       string      `json:"number"`
	IssueDate    time.Time   `json:"issueDate"`
	LinkedAmount model.Money `json:"linkedAmount"`
	FullyPaid    bool        `json:"fullyPaid"`
	PaidOn       *time.Time  `json:"paidOn,omitempty"`
}

// ForecastReconStatus flags each forecast's reconciliation state. Precedence
// (first-match wins as applied by the algorithm): NoInvoice, PaymentPending,
// OverExecuted, PartiallyJustified, FullyJustified.
type ForecastReconStatus int

const (
	StatusFullyJustified ForecastReconStatus = iota
	StatusPartiallyJustified
	StatusOverExecuted
	StatusPaymentPending
	StatusNoInvoice
)

// ComputeReconciliation is the pure entry point. Given the year's forecasts,
// concessions, invoices, taxonomy, and partners, it returns the snapshot tree
// described by the Phase 2 spec. Skeleton in Task 1; filled in Tasks 2-5.
func ComputeReconciliation(in ReconciliationInput) (ReconciliationData, error) {
	return ReconciliationData{Year: in.Year}, nil
}

// forecastExec bundles the per-forecast paid/pending totals with the list of
// invoice contributions (paid AND unpaid). It's the shared intermediate the
// downstream stages of ComputeReconciliation consume.
type forecastExec struct {
	Executed model.Money
	Pending  model.Money
	Invoices []InvoiceContribution
}

// executedAndPending walks the year's invoices and produces per-forecast
// paid/pending totals + audit contributions. Invoices are classified as
// fully paid iff Σ payments ≥ netAmount − 0.01. Enabled==false forecasts are
// skipped: their forecastExec is not populated (they don't appear in the map).
func executedAndPending(in ReconciliationInput) map[string]forecastExec {
	// Set of enabled forecast IDs (unknown IDs are ignored — data hygiene is
	// Phase 1's job; here we just don't produce output rows for them).
	enabled := make(map[string]bool, len(in.Forecasts))
	for _, f := range in.Forecasts {
		if f.Enabled() {
			enabled[f.ID()] = true
		}
	}
	out := make(map[string]forecastExec, len(enabled))
	for id := range enabled {
		out[id] = forecastExec{Executed: model.ZeroMoney(), Pending: model.ZeroMoney()}
	}

	for _, inv := range in.Invoices {
		paidTotal := inv.PaidTotal()
		fullyPaid := invoiceFullyPaid(paidTotal, inv.NetAmount())
		paidOn := latestPaidOn(inv, fullyPaid)
		for _, link := range inv.Links() {
			id := link.ForecastID()
			if !enabled[id] {
				continue
			}
			cur := out[id]
			contrib := InvoiceContribution{
				InvoiceID:    inv.ID(),
				Issuer:       inv.Issuer(),
				Number:       inv.Number(),
				IssueDate:    inv.IssueDate(),
				LinkedAmount: link.Amount(),
				FullyPaid:    fullyPaid,
				PaidOn:       paidOn,
			}
			if fullyPaid {
				cur.Executed = cur.Executed.Plus(link.Amount())
			} else {
				cur.Pending = cur.Pending.Plus(link.Amount())
			}
			cur.Invoices = append(cur.Invoices, contrib)
			out[id] = cur
		}
	}
	// Deterministic ordering for each forecast's invoice list.
	for id, fx := range out {
		sort.Slice(fx.Invoices, func(i, j int) bool {
			if !fx.Invoices[i].IssueDate.Equal(fx.Invoices[j].IssueDate) {
				return fx.Invoices[i].IssueDate.Before(fx.Invoices[j].IssueDate)
			}
			return fx.Invoices[i].Number < fx.Invoices[j].Number
		})
		out[id] = fx
	}
	return out
}

// invoiceFullyPaid = Σ payments ≥ netAmount − 0.01 (all-or-nothing rule).
func invoiceFullyPaid(paidTotal, netAmount model.Money) bool {
	// paidTotal ≥ netAmount − 0.01  ⇔  paidTotal + 0.01 ≥ netAmount
	// Using cent-level compare via Money.Cmp.
	oneCent, _ := model.MoneyFromString("0.01")
	return paidTotal.Plus(oneCent).Cmp(netAmount) >= 0
}

// latestPaidOn returns the latest payment date if fully paid, else nil.
func latestPaidOn(inv model.Invoice, fullyPaid bool) *time.Time {
	if !fullyPaid || len(inv.Payments()) == 0 {
		return nil
	}
	latest := inv.Payments()[0].PaidOn()
	for _, p := range inv.Payments()[1:] {
		if p.PaidOn().After(latest) {
			latest = p.PaidOn()
		}
	}
	return &latest
}

// groupResult carries a Concessió group's Base (=min(Granted, Executed_g)) and
// its Assigned total (equals Base — kept as a separate field so the roll-ups
// task in Task 5 can just sum without recomputing).
type groupResult struct {
	Base     model.Money
	Assigned model.Money
	Executed model.Money // Σ Executed_i for forecasts in group (used later)
	Granted  model.Money // Concession.GrantedAmount, needed by status precedence
}

// assignForGroups computes Base_g = min(Granted_g, Executed_g) for every
// Concessió, then prorates Base_g across the group's forecasts by each
// forecast's share of Executed_g. Uses largest-remainder to close the cent so
// Σ Assigned_i = Base_g exactly. Forecasts not in any group (or in a group
// with Executed_g == 0) get Assigned = 0.
func assignForGroups(in ReconciliationInput, exec map[string]forecastExec) (map[string]groupResult, map[string]model.Money) {
	// groupCode → []forecastID
	groupForecasts := make(map[string][]string, len(in.Concessions))
	for _, l := range in.Links {
		groupForecasts[l.GroupCode()] = append(groupForecasts[l.GroupCode()], l.ForecastID())
	}

	groups := make(map[string]groupResult, len(in.Concessions))
	assigned := make(map[string]model.Money, len(exec))
	for id := range exec {
		assigned[id] = model.ZeroMoney()
	}

	for _, c := range in.Concessions {
		ids := groupForecasts[c.GroupCode()]
		// Σ Executed_g across the group's forecasts (only enabled ones survive
		// in the exec map; unknown ids are skipped).
		execG := model.ZeroMoney()
		for _, id := range ids {
			if fx, ok := exec[id]; ok {
				execG = execG.Plus(fx.Executed)
			}
		}
		var base model.Money
		if execG.Cmp(c.GrantedAmount()) < 0 {
			base = execG
		} else {
			base = c.GrantedAmount()
		}
		groups[c.GroupCode()] = groupResult{Base: base, Assigned: base, Executed: execG, Granted: c.GrantedAmount()}

		if execG.IsZero() {
			continue // all Assigned_i stay at 0
		}
		// Largest-remainder: compute each forecast's fractional Assigned as
		// Base * Executed_i / Executed_g, take the floor at cent precision,
		// then distribute the remaining cents to the largest fractional parts.
		type share struct {
			id       string
			floor    model.Money
			fraction decimal.Decimal // fractional cents lost to floor
		}
		shares := make([]share, 0, len(ids))
		baseCents := base.Decimal().Mul(decimal.NewFromInt(100)) // ×100 → cent scale
		execGDec := execG.Decimal()
		var floorSumCents decimal.Decimal
		for _, id := range ids {
			fx, ok := exec[id]
			if !ok {
				continue
			}
			// exact_i (in cents) = Base * Executed_i / Executed_g * 100
			exactCents := baseCents.Mul(fx.Executed.Decimal()).Div(execGDec)
			floorCents := exactCents.Floor()
			frac := exactCents.Sub(floorCents)
			floor := model.MoneyFromDecimalCents(floorCents)
			shares = append(shares, share{id: id, floor: floor, fraction: frac})
			floorSumCents = floorSumCents.Add(floorCents)
		}
		// Distribute remainder cents (base_cents − Σ floor_cents) to the largest fractions.
		remaining := baseCents.Sub(floorSumCents).IntPart()
		// Stable sort by fraction desc; tie-break by id asc.
		sort.SliceStable(shares, func(i, j int) bool {
			if !shares[i].fraction.Equal(shares[j].fraction) {
				return shares[i].fraction.GreaterThan(shares[j].fraction)
			}
			return shares[i].id < shares[j].id
		})
		oneCent, _ := model.MoneyFromString("0.01")
		for i := range shares {
			assign := shares[i].floor
			if int64(i) < remaining {
				assign = assign.Plus(oneCent)
			}
			assigned[shares[i].id] = assign
		}
	}
	return groups, assigned
}

// statusFor applies the precedence rule from the Phase 2 spec:
// 1. NoInvoice      — zero links total.
// 2. PaymentPending — has any unpaid link (Pending > 0).
// 3. OverExecuted   — paid Executed_i > GrossAmount_i.
// 4. PartiallyJustified — group Executed < Granted.
// 5. FullyJustified — group Executed ≥ Granted.
// A forecast not attached to any group (data hygiene issue) is treated as
// NoInvoice.
func statusFor(f model.ExpenseForecast, fx forecastExec, g groupResult, hasGroup bool) ForecastReconStatus {
	if len(fx.Invoices) == 0 || !hasGroup {
		return StatusNoInvoice
	}
	if !fx.Pending.IsZero() {
		return StatusPaymentPending
	}
	if fx.Executed.Cmp(f.GrossAmount()) > 0 {
		return StatusOverExecuted
	}
	if g.Executed.Cmp(g.Granted) < 0 {
		return StatusPartiallyJustified
	}
	return StatusFullyJustified
}

