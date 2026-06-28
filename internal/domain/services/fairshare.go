package services

import (
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

const (
	maxIterations        = 100
	convergenceThreshold = "0.01"
)

type fairShareResult struct {
	allocations    []report.PartnerAllocation
	finalRemainder model.Money
}

// distribute runs the iterative fair-share distribution: partners requesting at
// most the per-head mean keep their request; the rest are capped at the mean.
// Ported verbatim from espigol-java FairShareAllocator.
func distribute(remainder model.Money, partnerTotals map[int]model.Money, partnerNames map[int]string) fairShareResult {
	if len(partnerTotals) == 0 {
		return fairShareResult{allocations: []report.PartnerAllocation{}, finalRemainder: remainder}
	}

	ids := sortedIDs(partnerTotals)
	requested := map[int]model.Money{}
	allocated := map[int]model.Money{}
	fixed := map[int]bool{}
	for _, id := range ids {
		requested[id] = partnerTotals[id]
		allocated[id] = partnerTotals[id]
		fixed[id] = false
	}

	totalRequested := sumMoney(values(requested, ids))

	// Case 1: no excess — everyone gets their full request.
	if totalRequested.Cmp(remainder) <= 0 {
		return fairShareResult{
			allocations:    buildAllocations(ids, requested, requested, partnerNames),
			finalRemainder: remainder.Minus(totalRequested),
		}
	}

	// Case 2: iterative fair share.
	threshold := mustMoney(convergenceThreshold)
	budgetLeft := remainder
	for iter := 0; iter < maxIterations; iter++ {
		nUnfixed := 0
		for _, id := range ids {
			if !fixed[id] {
				nUnfixed++
			}
		}
		if nUnfixed == 0 {
			break
		}
		mean := budgetLeft.DividedBy(nUnfixed)

		newlyFixed := false
		for _, id := range ids {
			if fixed[id] {
				continue
			}
			if allocated[id].Cmp(mean) <= 0 {
				fixed[id] = true
				budgetLeft = budgetLeft.Minus(allocated[id])
				newlyFixed = true
			}
		}
		if !newlyFixed {
			// All remaining requests exceed the mean — cap them at the mean.
			for _, id := range ids {
				if !fixed[id] {
					allocated[id] = mean
					fixed[id] = true
				}
			}
			break
		}
		diff := absDecimal(remainder.Decimal().Sub(sumMoney(values(allocated, ids)).Decimal()))
		if diff.Cmp(threshold.Decimal()) < 0 {
			break
		}
	}

	return fairShareResult{
		allocations:    buildAllocations(ids, requested, allocated, partnerNames),
		finalRemainder: remainder.Minus(sumMoney(values(allocated, ids))),
	}
}

func buildAllocations(ids []int, requested, allocated map[int]model.Money, nameByID map[int]string) []report.PartnerAllocation {
	out := make([]report.PartnerAllocation, 0, len(ids))
	for _, id := range ids {
		out = append(out, report.PartnerAllocation{
			PartnerID:   id,
			PartnerName: nameByID[id],
			Requested:   requested[id],
			Allocated:   allocated[id],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PartnerName < out[j].PartnerName })
	return out
}

func sortedIDs(m map[int]model.Money) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func values(m map[int]model.Money, ids []int) []model.Money {
	out := make([]model.Money, 0, len(ids))
	for _, id := range ids {
		out = append(out, m[id])
	}
	return out
}

func sumMoney(ms []model.Money) model.Money {
	total := model.ZeroMoney()
	for _, m := range ms {
		total = total.Plus(m)
	}
	return total
}
