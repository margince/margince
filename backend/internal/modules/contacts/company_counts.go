// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two roll-ups every company row shows (PO-EXT-10, AC-companies-2/3):
// how many contacts work here, and how many deals are open. Attached to a
// page in one query each, the same batch shape attachCompanyDomains uses, so a
// list of fifty accounts costs two statements rather than a hundred.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attachCompanyCounts fills contact_count and open_deal_count for a page.
//
// contact_count is a count of the contacts the CALLER may see, under the
// same row-scope predicate the contact list applies: owner-only capture
// privacy holds for a count as for a name, since a number that moves when a
// colleague captures a private contact discloses that contact.
//
// open_deal_count counts the WHOLE workspace — the same population the
// account's computed_fields open-pipeline row sums — by founder decision
// (PO-EXT-10, 2026-08-18): a pipeline figure on an account is a fact about
// the account, not about who may open each deal. What it does follow is that
// row's visibility gate (STATE-4): a role without computed_field:read is
// shown no count of a pipeline it may not see. Absent means withheld; every
// visible account carries a number, zero included, so a reader can tell
// "none" from "not yours to know".
func attachCompanyCounts(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company, asOf time.Time) error {
	if len(companies) == 0 {
		return nil
	}
	idx := make(map[openapi_types.UUID]*crmcontracts.Company, len(companies))
	companyIDs := make([]ids.UUID, len(companies))
	dealsVisible := pipelineCountsVisible(ctx)
	for i := range companies {
		idx[companies[i].Id] = &companies[i]
		companyIDs[i] = ids.UUID(companies[i].Id)
		if dealsVisible {
			zero := 0
			companies[i].OpenDealCount = &zero
		}
	}
	if err := fillContactCounts(ctx, tx, idx, companyIDs); err != nil {
		return err
	}
	if !dealsVisible {
		return nil
	}
	return fillOpenDealCounts(ctx, tx, idx, companyIDs, asOf)
}

// contactCountVisible decides whether contact_count and its sort show. The
// count needs relationship.read beside contact.read, because it is a fact
// about employment edges. Without both it stays absent, so the count never
// reveals edges the role is refused elsewhere.
func contactCountVisible(ctx context.Context) bool {
	return auth.ReadGranted(ctx, "contact") && auth.ReadGranted(ctx, "relationship")
}

// pipelineCountsVisible decides whether open_deal_count, the computed fields
// and the open-deal sort show (STATE-4). A role without deal.read or
// computed_field.read sees none of them. Absent, not zero.
func pipelineCountsVisible(ctx context.Context) bool {
	return auth.ReadGranted(ctx, "deal") && auth.ReadGranted(ctx, "computed_field")
}

// fillContactCounts counts distinct current employees to live contacts the
// caller may see. It checks contactCountVisible itself and sets the zero, so
// the list and the single read gate the count alike.
func fillContactCounts(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Company, companyIDs []ids.UUID) error {
	if len(companyIDs) == 0 || !contactCountVisible(ctx) {
		return nil
	}
	for _, company := range idx {
		zero := 0
		company.ContactCount = &zero
	}
	args := []any{companyIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The helper bounds which edges and contacts are counted. A count never
	// includes an edge whose endpoint the caller cannot reach.
	from, err := countedEmploymentFrom(ctx, "rel.company_id = ANY($1)", arg)
	if err != nil {
		return err
	}
	return fillCount(ctx, tx, idx,
		`SELECT rel.company_id, count(DISTINCT p.id)`+from+`
		 GROUP BY rel.company_id`, args,
		func(o *crmcontracts.Company, n int) { o.ContactCount = &n })
}

// countedEmploymentFrom is the FROM and WHERE of "which contacts work here that
// this caller may see", with the company binding left to the caller.
//
// Both the count the page PRINTS and the expression it ORDERS BY build their
// statement from here — the page counts the whole set at once, the sort counts
// per row — so a reader sees the number the list was arranged by rather than a
// second reading of the same question.
//
// It takes the gates itself rather than being handed them: which employment
// EDGES this caller may traverse bounds the count as much as which CONTACTS they
// may see, and a caller that had to remember to pass either could forget one
// and get a counting oracle over edges it is refused everywhere else.
func countedEmploymentFrom(ctx context.Context, companyBinding string, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return "", err
	}
	edgeBound, err := auth.EdgeReadScope(ctx, "rel", arg)
	if err != nil {
		return "", err
	}
	if edgeBound != "" {
		edgeBound = " AND " + edgeBound
	}
	scope, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return "", err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	return `
		 FROM relationship rel
		 JOIN contact p ON p.id = rel.contact_id AND p.archived_at IS NULL
		 WHERE ` + companyBinding + `
		   AND ` + employment.CurrentMembershipSQL("rel") + edgeBound + scope, nil
}

// fillOpenDealCounts reads the 0065 company_open_pipeline_rollup view
// for the page — the ONE spelling of "open deal" this module reads, so the
// list's count and the company page's open-pipeline tile derive from the
// same rows and cannot disagree.
// Held by: TestEveryOpenDealCountComesFromTheRollup (backend/internal/modules/contacts/opendealcount_test.go)
func fillOpenDealCounts(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Company, companyIDs []ids.UUID, asOf time.Time) error {
	return fillCount(ctx, tx, idx,
		`SELECT company_id, open_deal_count
		 FROM company_open_pipeline_rollup($2)
		 WHERE company_id = ANY($1)`, []any{companyIDs, asOf},
		func(o *crmcontracts.Company, n int) { o.OpenDealCount = &n })
}

// fillCount runs one (company_id, count) query and hands each row's
// count to set on the matching company.
func fillCount(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Company,
	query string, args []any, set func(*crmcontracts.Company, int),
) error {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID ids.UUID
		var n int
		if err := rows.Scan(&companyID, &n); err != nil {
			return err
		}
		if o, ok := idx[openapi_types.UUID(companyID)]; ok {
			set(o, n)
		}
	}
	return rows.Err()
}
