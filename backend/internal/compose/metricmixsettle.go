// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"context"
	"math/big"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var saleCategoryOrder = []string{salesprogress.Product, salesprogress.Service, salesprogress.Unresolved}

// settleShares splits total over weights by largest remainder, in minor units,
// so the shares add up to total. A tie goes to the earlier weight.
func settleShares(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	var sum int64
	for _, weight := range weights {
		sum += weight
	}
	if sum <= 0 {
		return out
	}
	type remainder struct {
		index int
		rest  *big.Int
	}
	rests := make([]remainder, len(weights))
	var given int64
	for index, weight := range weights {
		quotient, rest := new(big.Int).QuoRem(new(big.Int).Mul(big.NewInt(total), big.NewInt(weight)), big.NewInt(sum), new(big.Int))
		out[index] = quotient.Int64()
		given += out[index]
		rests[index] = remainder{index: index, rest: rest}
	}
	slices.SortStableFunc(rests, func(a, b remainder) int { return b.rest.Cmp(a.rest) })
	for step := int64(0); step < total-given; step++ {
		out[rests[step].index]++
	}
	return out
}

// settleMix makes each won deal's commercial-mix parts add up to the converted
// value bookings_won counts. So the split of closed revenue and its evidence
// agree to the minor unit. The deal value is
// split over the categories first, then each category over its lines, which
// are ordered by their product id.
func settleMix(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, facts []reporting.Fact) ([]reporting.Fact, error) {
	totals, err := wonDealValues(ctx, tx, frame, facts)
	if err != nil {
		return nil, err
	}
	// Each reading context holds its own copy of a deal's parts, so a deal is
	// settled once per context; the copies settle alike.
	type group struct {
		deal    ids.UUID
		context string
	}
	var order []group
	byDeal := map[group][]int{}
	for index, fact := range facts {
		key := group{fact.SourceID, fact.ContextID}
		if _, seen := byDeal[key]; !seen {
			order = append(order, key)
		}
		byDeal[key] = append(byDeal[key], index)
	}
	invalidLabel := reporting.ExplanationCopy(frame).Say(reportingComponentKindLabels[salesprogress.InvalidKind])
	out := make([]reporting.Fact, 0, len(facts))
	for _, key := range order {
		out = append(out, settleDeal(facts, byDeal[key], totals[key.deal], invalidLabel)...)
	}
	return out, nil
}

// settleDeal settles one deal's parts. A deal whose value is unknown keeps its
// parts as read; one whose lines exceed it becomes a single invalid part.
func settleDeal(facts []reporting.Fact, indexes []int, total *int64, invalidLabel string) []reporting.Fact {
	parts := make([]reporting.Fact, 0, len(indexes))
	for _, index := range indexes {
		parts = append(parts, facts[index])
	}
	if total == nil || slices.ContainsFunc(parts, func(f reporting.Fact) bool { return f.Money == nil }) {
		return slices.DeleteFunc(parts, func(f reporting.Fact) bool { return f.Money != nil && *f.Money < 0 })
	}
	if slices.ContainsFunc(parts, func(f reporting.Fact) bool { return *f.Money < 0 }) {
		invalid := parts[0]
		invalid.GroupKey, invalid.GroupLabel = "kind:"+salesprogress.InvalidKind, invalidLabel
		invalid.Row.Key = strings.SplitN(invalid.Row.Key, ":", 2)[0] + ":" + salesprogress.InvalidKind
		setFactMoney(&invalid, *total)
		return []reporting.Fact{invalid}
	}
	slices.SortStableFunc(parts, func(a, b reporting.Fact) int {
		if order := cmp.Compare(slices.Index(saleCategoryOrder, salesprogress.Category(a.GroupKey)), slices.Index(saleCategoryOrder, salesprogress.Category(b.GroupKey))); order != 0 {
			return order
		}
		return cmp.Compare(a.Row.Key, b.Row.Key)
	})
	weights := make([]int64, len(saleCategoryOrder))
	for _, part := range parts {
		weights[slices.Index(saleCategoryOrder, salesprogress.Category(part.GroupKey))] += *part.Money
	}
	shares := settleShares(*total, weights)
	for category, share := range shares {
		var lines []int
		var lineWeights []int64
		for index, part := range parts {
			if salesprogress.Category(part.GroupKey) == saleCategoryOrder[category] {
				lines, lineWeights = append(lines, index), append(lineWeights, *part.Money)
			}
		}
		for position, amount := range settleShares(share, lineWeights) {
			setFactMoney(&parts[lines[position]], amount)
		}
	}
	return parts
}

func setFactMoney(fact *reporting.Fact, amount int64) {
	value := float64(amount)
	fact.Money, fact.Row.Value = &amount, &value
}

// wonDealValues reads each deal's value as bookings_won counts it: converted at
// its close and behind the same field mask.
func wonDealValues(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, facts []reporting.Fact) (map[ids.UUID]*int64, error) {
	deals := make([]ids.UUID, 0, len(facts))
	for _, fact := range facts {
		deals = append(deals, fact.SourceID)
	}
	var b reportingBindings
	value, err := reportingFactValue(ctx, frame, closingMetricQuery(true), &b)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT t.id,"+value+" FROM deal t WHERE t.id=ANY("+b.add(deals)+")", b.values...)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID]*int64{}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var amount *int64
		if err := rows.Scan(&id, &amount); err != nil {
			return nil, err
		}
		out[id] = amount
	}
	return out, rows.Err()
}

// dropOverAllocation keeps a filtered mix as it read before settling existed:
// the negative remainder of an over-allocated deal is no product's.
func dropOverAllocation(facts []reporting.Fact) []reporting.Fact {
	return slices.DeleteFunc(facts, func(f reporting.Fact) bool { return f.Money != nil && *f.Money < 0 })
}
