// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What the accounts list orders by when the column it draws is not a column of
// `organization`.
//
// A column the list SHOWS is a column the list SORTS BY (Lars, 2026-08-21), and
// half of what this list shows is derived: the website comes off the primary
// domain row, and the two counts are read per page rather than stored. Each is
// spelled here as the expression that orders it, beside the rule it mirrors.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// orderByPrimaryDomain orders by the host the Website column draws.
//
// website_url is DERIVED, never stored (ADR-0085): the primary domain row is
// the canonical fact. This picks the same row attachOrgDomains renders from —
// live, primary, oldest first — so the order the reader sees is the order of
// the values on screen. An account with no primary domain shows nothing and
// orders by nothing, which is the same answer.
//
// The domain rather than the rendered URL, because the scheme is a constant
// this product adds and sorting by "https://" tells nobody anything.
func orderByPrimaryDomain(context.Context, func(any) int) (string, error) {
	return `(SELECT wd.domain FROM organization_domain wd
	          WHERE wd.organization_id = organization.id
	            AND wd.archived_at IS NULL AND wd.is_primary
	          ORDER BY wd.created_at LIMIT 1)`, nil
}

// orderByContactCount orders by how many contacts this caller may see at the
// account — the number the Contacts column prints, counted per row.
//
// A role that may not read people, or may not traverse employment edges, sees
// no count and orders by NOTHING: the column is absent for them, and a page
// ordered by a number they are refused would disclose it through the order.
func orderByContactCount(ctx context.Context, arg func(any) int) (string, error) {
	if !grantVisible(ctx, "person") || !grantVisible(ctx, "relationship") {
		return withheldSortValue, nil
	}
	from, err := countedEmploymentFrom(ctx, "rel.organization_id = organization.id", arg)
	if err != nil {
		return "", err
	}
	return "(SELECT count(*)" + from + ")", nil
}

// orderByOpenDealCount orders by the account's open pipeline, read from the
// 0065 rollup, where "open deal" is defined — so the order and the number on
// screen come from the same rows.
//
// Held by: TestEveryOpenDealCountComesFromTheRollup (backend/internal/modules/people/opendealcount_test.go)
//
// The as-of date is rollupAsOf(), which the page's own count already uses and
// which truncates to the day: the sort and the figure beside it cannot land on
// two different instants.
//
// A role without computed_field:read is shown NO count (STATE-4) and orders by
// nothing — same rule as the contacts column above, for the same reason.
func orderByOpenDealCount(ctx context.Context, arg func(any) int) (string, error) {
	if !grantVisible(ctx, "deal") || !computedFieldsVisible(ctx) {
		return withheldSortValue, nil
	}
	return storekit.SQLf(
		`(SELECT pipeline_sort.open_deal_count
		    FROM organization_open_pipeline_rollup($%d) pipeline_sort
		   WHERE pipeline_sort.organization_id = organization.id)`, arg(rollupAsOf())), nil
}

// withheldSortValue is what a column this caller may not see orders by: NULL,
// which the list's ORDER BY already puts last. Every row then sits in the tail
// together and the page falls back to its tie-breaker, so the order says
// nothing at all about the figures behind it.
//
// CAST, because Postgres reads a bare NULL in an ORDER BY as a constant and
// refuses the statement. Both columns this answers for are counts, so the type
// is the one their cursor keys already round-trip through.
const withheldSortValue = "NULL::numeric"

// Compile-time proof that each of these is the shape a sortable field takes.
var _ = []func(context.Context, func(any) int) (string, error){
	orderByPrimaryDomain, orderByContactCount, orderByOpenDealCount,
}
