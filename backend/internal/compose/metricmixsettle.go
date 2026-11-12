// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settleMix settles the won deals' mix parts against the values bookings_won
// counts, which only this package can read.
func settleMix(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, facts []reporting.Fact) ([]reporting.Fact, error) {
	totals, err := wonDealValues(ctx, tx, frame, facts)
	if err != nil {
		return nil, err
	}
	invalidLabel := reporting.ExplanationCopy(frame).Say(reportingComponentKindLabels[salesprogress.InvalidKind])
	return salesprogress.SettleMix(facts, totals, invalidLabel), nil
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
