// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ClosedYear is one seller's closed revenue for each month of a calendar
// year, in minor units of the reporting currency. A nil month has not ended
// or is not the reader's to see.
type ClosedYear struct {
	Year     int
	Currency string
	Months   [12]*int64
}

// ClosedByMonth reads the months Progress shows for one seller's year, under
// the reader's own lens, and keeps the ones that have ended.
func (h Reading) ClosedByMonth(ctx context.Context, tx pgx.Tx, owner ids.UUID, year int) (ClosedYear, error) {
	frame, err := h.frame(ctx, tx, progressPeriod{kind: "year", year: year})
	if err != nil {
		return ClosedYear{}, err
	}
	out := ClosedYear{Year: year, Currency: frame.currency}
	id := openapi_types.UUID(owner)
	scope, err := h.Reports.GoalScope(ctx, tx, crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindOwner, Id: &id})
	if err != nil {
		return out, err
	}
	evaluation, err := h.yearEvaluation(ctx, tx, scope, frame)
	if err != nil {
		return out, err
	}
	var table crmcontracts.SalesProgress
	fillMonths(frame, evaluation, reporting.OwnerGoals{}, reporting.GoalSellers{}, &table)
	for index, month := range table.Months {
		if month.State == crmcontracts.SalesProgressMonthStateEnded && month.Closed.Status == crmcontracts.SalesProgressAmountStatusOk {
			out.Months[index] = month.Closed.AmountMinor
		}
	}
	return out, nil
}
