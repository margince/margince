// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What the deals list orders by when the column it draws is not a column of
// `deal`.
//
// A column the list SHOWS is a column the list SORTS BY (Lars, 2026-08-21), and
// three of the eight it draws are references: a stage and the two
// companies. Each is spelled here as the expression that orders it, beside
// the rule it mirrors.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// companyGrantVisible answers whether this caller holds the OBJECT half of
// company.read. A system principal holds every object grant by
// construction; a seat holds what its role was given.
func companyGrantVisible(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if actor.Type == principal.PrincipalSystem {
		return true
	}
	return actor.Permissions.Allows("company", principal.ActionRead)
}

// orderByStagePosition orders by a stage's place in its PIPELINE, not by its
// name.
//
// Alphabetical is almost never what somebody sorting by stage means: they want
// the funnel, and "Discovery, Negotiation, Proposal" is the funnel shuffled.
// `stage` is workspace configuration and carries no row scope, so the position
// is the same number for every reader.
func orderByStagePosition(context.Context, func(any) int) (string, error) {
	return "(SELECT stage_sort.position FROM stage stage_sort WHERE stage_sort.id = deal.stage_id)", nil
}

// orderByPriorityRank orders by the generated rank beside `priority`, so the
// order is the business one — High, Medium, Low, then the deals nobody has
// prioritised — rather than the alphabetical one the text column would give
// ('high' < 'low' < 'medium' interleaves them meaninglessly).
//
// The rank is a STORED generated column rather than a CASE rendered here, so
// the index on it can serve this ordering and there is no writer that could
// set one without the other.
func orderByPriorityRank(context.Context, func(any) int) (string, error) {
	return "deal.priority_rank", nil
}

// orderByReadableCompanyName orders by the referenced company's name, and by
// NOTHING for a reference this caller may not read.
//
// Ordering by a value is reading it — the rule refuseMaskedSort already applies
// to masked amounts — so BOTH halves of RBAC bound it.
//
// The object grant first: auth.ScopeClauseFor answers row visibility and never
// asks whether this caller may read companies at all, so a seat holding
// deal.read and no company.read would otherwise have its page arranged by
// company names it is refused on every other surface.
//
// Then the row scope, INSIDE the subquery rather than beside it: a reference
// outside the caller's scope answers NULL, which the ORDER BY already puts
// last, so those deals land in the tail together and the order says nothing
// about which company they name. That is the same answer the row itself gives,
// where the reference is withheld.
func orderByReadableCompanyName(column string) func(context.Context, func(any) int) (string, error) {
	return func(ctx context.Context, arg func(any) int) (string, error) {
		if !companyGrantVisible(ctx) {
			// Ordered by nothing: every row sits in the tail and the page
			// falls back to its tie-breaker.
			return "NULL::text", nil
		}
		scope, err := auth.ScopeClauseFor(ctx, "company", "company_sort", arg)
		if err != nil {
			return "", err
		}
		if scope != "" {
			scope = " AND " + scope
		}
		// The column is one of this map's own keys, never a caller's string.
		return storekit.SQLf(
			"(SELECT company_sort.display_name FROM company company_sort WHERE company_sort.id = deal.%s%s)",
			column, scope), nil
	}
}
