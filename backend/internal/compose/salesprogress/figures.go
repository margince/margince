// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// metricAmount reads one figure of an evaluation as a progress amount. A
// measured zero is a known amount. A partly priced figure is partial, so no
// completion is claimed on it. Any other reading is withheld.
func metricAmount(evaluation reporting.Evaluation, metric crmcontracts.ReportingMetricID) crmcontracts.SalesProgressAmount {
	for _, reading := range evaluation.Result.Metrics {
		if reading.Id != metric {
			continue
		}
		if reading.Coverage.Withheld || reading.Value == nil {
			return crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusWithheld}
		}
		amount := int64(*reading.Value)
		switch reading.Coverage.Status {
		case crmcontracts.ReportingStatusOk, crmcontracts.ReportingStatusNoData:
			return crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusOk, AmountMinor: &amount}
		case crmcontracts.ReportingStatusPartial, crmcontracts.ReportingStatusInsufficientSample:
			return crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusPartial, AmountMinor: &amount}
		}
	}
	return crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusWithheld}
}

func fillFigures(frame progressFrame, headline reporting.Evaluation, goals reporting.OwnerGoals, sellers reporting.GoalSellers, out *crmcontracts.SalesProgress) {
	out.Goal = goals.Over(sellers.Owners, frame.months(), frame.period.yearly())
	out.Planned = metricAmount(headline, crmcontracts.ReportingMetricIDOpenPipeline)
	out.Closed = crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusNotStarted}
	out.Categories = crmcontracts.SalesProgressCategories{Status: crmcontracts.SalesProgressCategoriesStatusNotStarted, ProductItems: []string{}, ServiceItems: []string{}}
	if !frame.started() {
		return
	}
	out.Closed = metricAmount(headline, crmcontracts.ReportingMetricIDBookingsWon)
	out.Completion, out.DeltaMinor = compared(out.Closed, out.Goal)
	out.Categories.Status = crmcontracts.SalesProgressCategoriesStatus(out.Closed.Status)
	if out.Closed.AmountMinor == nil {
		return
	}
	split := map[string]int64{}
	invalid := map[ids.UUID]bool{}
	for _, fact := range headline.Facts {
		if fact.Metric != crmcontracts.ReportingMetricIDBookingsMix || fact.ContextID != contextInterval || fact.Money == nil {
			continue
		}
		split[Category(fact.GroupKey)] += *fact.Money
		if strings.TrimPrefix(fact.GroupKey, "kind:") == InvalidKind {
			invalid[fact.SourceID] = true
		}
	}
	// The split is settled to each deal's value, so a gap is a deal the mix
	// could not read. It is unresolved, never a product or a service.
	if gap := *out.Closed.AmountMinor - split[Product] - split[Service] - split[Unresolved]; gap > 0 {
		split[Unresolved] += gap
	}
	out.Categories.ProductMinor, out.Categories.ServiceMinor, out.Categories.UnresolvedMinor = split[Product], split[Service], split[Unresolved]
	out.Categories.InvalidDeals = len(invalid)
}

// fillMonths adds the twelve months of the selected year from the year's
// reading, each month's actuals placed by the close date's local month.
func fillMonths(frame progressFrame, year reporting.Evaluation, goals reporting.OwnerGoals, sellers reporting.GoalSellers, out *crmcontracts.SalesProgress) {
	closed := map[int]int64{}
	status := metricAmount(year, crmcontracts.ReportingMetricIDBookingsWon).Status
	for _, fact := range year.Facts {
		if fact.Metric == crmcontracts.ReportingMetricIDBookingsWon && fact.ContextID == contextInterval && fact.Money != nil && fact.At() != nil {
			closed[int(fact.At().In(frame.zone).Month())] += *fact.Money
		}
	}
	for month := 1; month <= 12; month++ {
		start := frame.year.StartAt.AddDate(0, month-1, 0)
		row := crmcontracts.SalesProgressMonth{
			Month: month, Window: crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)},
			State: crmcontracts.SalesProgressMonthStateEnded, Goal: goals.Over(sellers.Owners, []int{month}, false),
			Closed: crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusNotStarted},
		}
		switch {
		case !start.Before(frame.now):
			row.State = crmcontracts.SalesProgressMonthStateNotStarted
		case frame.now.Before(row.Window.EndAt):
			row.State = crmcontracts.SalesProgressMonthStateCurrent
		}
		if row.State != crmcontracts.SalesProgressMonthStateNotStarted {
			amount := closed[month]
			row.Closed = crmcontracts.SalesProgressAmount{Status: status, AmountMinor: &amount}
			if status == crmcontracts.SalesProgressAmountStatusWithheld {
				row.Closed.AmountMinor = nil
			}
			_, row.DeltaMinor = compared(row.Closed, row.Goal)
		}
		out.Months = append(out.Months, row)
	}
}

// fillMembers lists a team's members one by one, each with the facts the
// headline counted for them, in name order.
func (h Reading) fillMembers(ctx context.Context, tx pgx.Tx, frame progressFrame, headline reporting.Evaluation, goals reporting.OwnerGoals, sellers reporting.GoalSellers, out *crmcontracts.SalesProgress) error {
	out.MembersWithheld = sellers.Team && sellers.Withheld
	if !sellers.Team || len(sellers.Owners) == 0 {
		return nil
	}
	names, err := seatNames(ctx, tx, sellers.Owners)
	if err != nil {
		return err
	}
	closed, planned := map[ids.UUID]int64{}, map[ids.UUID]int64{}
	for _, fact := range headline.Facts {
		if fact.Money == nil || fact.OwnerWithheld {
			continue
		}
		switch {
		case fact.Metric == crmcontracts.ReportingMetricIDBookingsWon && fact.ContextID == contextInterval:
			closed[fact.OwnerID] += *fact.Money
		case fact.Metric == crmcontracts.ReportingMetricIDOpenPipeline && fact.ContextID == contextState:
			planned[fact.OwnerID] += *fact.Money
		}
	}
	for _, owner := range sellers.Owners {
		member := crmcontracts.SalesProgressMember{
			OwnerId: openapi_types.UUID(owner), Name: names[owner],
			Goal:   goals.Over([]ids.UUID{owner}, frame.months(), frame.period.yearly()),
			Closed: memberAmount(out.Closed.Status, closed[owner]), Planned: memberAmount(out.Planned.Status, planned[owner]),
		}
		_, member.DeltaMinor = compared(member.Closed, member.Goal)
		out.Members = append(out.Members, member)
	}
	slices.SortStableFunc(out.Members, func(a, b crmcontracts.SalesProgressMember) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.OwnerId.String(), b.OwnerId.String()))
	})
	return nil
}

func memberAmount(status crmcontracts.SalesProgressAmountStatus, amount int64) crmcontracts.SalesProgressAmount {
	if status == crmcontracts.SalesProgressAmountStatusWithheld || status == crmcontracts.SalesProgressAmountStatusNotStarted {
		return crmcontracts.SalesProgressAmount{Status: status}
	}
	return crmcontracts.SalesProgressAmount{Status: status, AmountMinor: &amount}
}

func seatNames(ctx context.Context, tx pgx.Tx, owners []ids.UUID) (map[ids.UUID]string, error) {
	rows, err := tx.Query(ctx, "SELECT id,COALESCE(display_name,'') FROM app_user WHERE id=ANY($1)", owners)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID]string{}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// fillItems names what was sold in each category, most revenue first, from
// the product each settled line carries.
func (h Reading) fillItems(ctx context.Context, tx pgx.Tx, headline reporting.Evaluation, out *crmcontracts.SalesProgress) error {
	revenue := map[string]map[ids.UUID]int64{Product: {}, Service: {}}
	var products []ids.UUID
	for _, fact := range headline.Facts {
		category := Category(fact.GroupKey)
		if fact.Metric != crmcontracts.ReportingMetricIDBookingsMix || fact.ContextID != contextInterval || fact.Money == nil || category == Unresolved {
			continue
		}
		_, part, _ := strings.Cut(fact.Row.Key, ":")
		product, err := ids.Parse(part)
		if err != nil {
			continue
		}
		revenue[category][product] += *fact.Money
		products = append(products, product)
	}
	if len(products) == 0 {
		return nil
	}
	names, err := productNames(ctx, tx, products)
	if err != nil {
		return err
	}
	out.Categories.ProductItems = rankedNames(revenue[Product], names)
	out.Categories.ServiceItems = rankedNames(revenue[Service], names)
	return nil
}

func rankedNames(revenue map[ids.UUID]int64, names map[ids.UUID]string) []string {
	sold := make([]ids.UUID, 0, len(revenue))
	for product := range revenue {
		sold = append(sold, product)
	}
	slices.SortFunc(sold, func(a, b ids.UUID) int {
		return cmp.Or(cmp.Compare(revenue[b], revenue[a]), cmp.Compare(names[a], names[b]), cmp.Compare(a.String(), b.String()))
	})
	out := make([]string, 0, min(len(sold), 50))
	for _, product := range sold[:min(len(sold), 50)] {
		out = append(out, names[product])
	}
	return out
}

func productNames(ctx context.Context, tx pgx.Tx, products []ids.UUID) (map[ids.UUID]string, error) {
	rows, err := tx.Query(ctx, "SELECT id,name FROM product WHERE id=ANY($1)", products)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID]string{}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}
