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

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// populationRule says WHOSE records a surface measures.
//
// measureCallersOwn is the zero value on purpose. A report added to the
// catalog that says nothing about its population measures the caller's own,
// so the narrow answer is the one you get by omission and widening is a thing
// somebody had to write down.
type populationRule int

const (
	measureCallersOwn populationRule = iota
	// measureEveryReadableRow measures every row the caller may READ, with no
	// narrowing of its own.
	//
	// For a surface with no "my work" framing: a tool asking "how many
	// projects are in delivery" is asking about the installation, not about
	// the asker, and an aggregate that narrowed it to the caller's team would
	// tell a delivery lead their department has no projects running. Row
	// scope still applies — this widens the POPULATION, never the reader's
	// authority.
	measureEveryReadableRow
)

func callersOwnPopulation() RequestedScope { return RequestedScope{} }

func requestedReportScope(scope *RequestedScope) RequestedScope {
	if scope == nil {
		return callersOwnPopulation()
	}
	return *scope
}

func checkReportScope(spec reportSpec, scope *RequestedScope) error {
	if scope == nil {
		return nil
	}
	if !measuresOwners(spec) || scope.Kind == "" {
		return apperrors.ErrInvalidArgument
	}
	return nil
}

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
	_, clause, err := AnalyticsPopulationClause(ctx, tx, requested, "t", arg, unownedIsPartOfDefault)
	return clause, err
}
