// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// WHICH population a prebuilt report measures, as against which rows its reader
// may see at all.
//
// Its own file because the two are constantly confused and the difference is
// the whole defect: a deal is an identity table read by every seat, so row
// scope narrows a report by nothing, and until the population was applied a
// rep's Pipeline answered the whole installation while their Forecast answered
// their own.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// callersOwnPopulation is the population a prebuilt report measures.
//
// Empty, deliberately: `POST /reports/{report}` carries no scope parameter, so
// the caller names nothing and the resolver answers with their own lens
// default — a rep's records, a manager's teams. There is no wider population to
// forge here because there is no field to forge it in, which is why this is a
// function with a name rather than a `RequestedScope{}` literal at two call
// sites that would read as an oversight.
//
// If this endpoint ever takes a scope, this is the one place that changes, and
// both the aggregate and its drill-through change with it.
func callersOwnPopulation() RequestedScope { return RequestedScope{} }

// reportPopulationClause is the population half for the report engine's own
// WHERE builders.
//
// One function so the aggregate and its drill-through cannot resolve
// differently: they are separate statements with separate bind slices, so the
// clause has to be RENDERED per statement — passing a finished string between
// them would carry the wrong `$N`. What must not differ is the question, and
// that is what this shares.
func reportPopulationClause(
	ctx context.Context, tx pgx.Tx, requested RequestedScope, arg func(any) int,
) (string, error) {
	_, clause, err := AnalyticsPopulationClause(ctx, tx, requested, "t", arg)
	return clause, err
}
