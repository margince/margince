// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The two roll-ups every company row shows (PO-EXT-10, AC-companies-2/3):
// how many people work here, and how many deals are open. Attached to a
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
// same row-scope predicate the person list applies: owner-only capture
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
	// Whether each company is this caller's to change, one statement for the
	// page, stamped at the seam the list and the single read already share.
	if _, err := auth.StampWritable(ctx, tx, "company", companies,
		func(o crmcontracts.Company) ids.UUID { return ids.UUID(o.Id) },
		func(o *crmcontracts.Company, may bool) { o.Writable = &may }); err != nil {
		return err
	}
	idx := make(map[openapi_types.UUID]*crmcontracts.Company, len(companies))
	companyIDs := make([]ids.UUID, len(companies))
	// The object grant comes first, as it does on the two lists themselves: a
	// role that may not read people gets no contact count, and one that may
	// not read deals gets no deal count — absent, not zero.
	//
	// The contact count needs the EDGE grant beside the person one, because the
	// number is a fact about the employment pairs and not about either end:
	// "how many people work at Acme" is precisely what relationship.read
	// governs, and a count that answered without it would be a counting oracle
	// over edges the role is refused on every other surface. Absent, again —
	// zero would be a wrong number on screen rather than a withheld one.
	contactsVisible := grantVisible(ctx, "person") && grantVisible(ctx, "relationship")
	dealsVisible := grantVisible(ctx, "deal") && computedFieldsVisible(ctx)
	for i := range companies {
		idx[companies[i].Id] = &companies[i]
		companyIDs[i] = ids.UUID(companies[i].Id)
		if contactsVisible {
			zero := 0
			companies[i].ContactCount = &zero
		}
		if dealsVisible {
			zeroDeals := 0
			companies[i].OpenDealCount = &zeroDeals
		}
	}
	if contactsVisible {
		if err := fillContactCounts(ctx, tx, idx, companyIDs); err != nil {
			return err
		}
	}
	if !dealsVisible {
		return nil
	}
	return fillOpenDealCounts(ctx, tx, idx, companyIDs, asOf)
}

// grantVisible is the object-grant half of a read, answered from the
// principal's merged permissions the way computedFieldsVisible answers its
// own: no query, system principal trusted, no actor fails closed.
func grantVisible(ctx context.Context, object string) bool {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if actor.Type == principal.PrincipalSystem {
		return true
	}
	return actor.Permissions.Allows(object, principal.ActionRead)
}

// fillContactCounts counts current-primary employment edges to live people
// the caller may see. The company end needs no predicate of its own:
// every account on the page already passed the list's row scope.
func fillContactCounts(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Company, companyIDs []ids.UUID) error {
	args := []any{companyIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// attachCompanyCounts admitted the object; this bounds WHICH edges are counted,
	// so a count never includes an edge whose endpoint the caller cannot reach.
	edgeBound, err := auth.RelationshipEndpointScope(ctx, "rel", arg)
	if err != nil {
		return err
	}
	if edgeBound != "" {
		edgeBound = " AND " + edgeBound
	}
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	return fillCount(ctx, tx, idx,
		`SELECT rel.company_id, count(*)
		 FROM relationship rel
		 JOIN person p ON p.id = rel.person_id AND p.archived_at IS NULL
		 WHERE rel.company_id = ANY($1)
		   AND rel.kind = 'employment'
		   AND `+employment.CurrentPrimarySQL("rel")+`
		   AND rel.archived_at IS NULL`+edgeBound+scope+`
		 GROUP BY rel.company_id`, args,
		func(o *crmcontracts.Company, n int) { o.ContactCount = &n })
}

// fillOpenDealCounts reads the 0065 company_open_pipeline_rollup view
// for the page — the ONE spelling of "open deal" this module reads, so the
// list's count and the company page's open-pipeline tile derive from the
// same rows and cannot disagree.
// Held by: TestEveryOpenDealCountComesFromTheRollup (backend/internal/modules/people/opendealcount_test.go)
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
