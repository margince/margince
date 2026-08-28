// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which of a deal's fields a caller may set back to NOTHING.
//
// Its own file because "which fields this store can clear" is a question a
// reader asks whole, and answering it means reading one place rather than
// hunting through the writer. Applying a clear is storekit's.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// clearableDealColumns names the wire fields whose clear writes ONE column,
// with literal column names. amount_minor and currency are absent: money is
// read as one field, and a half-cleared pair states an amount in no currency.
// status and the close-date flags belong to the advance path.
//
// The partner fields are absent because their clear writes TWO columns — see
// dealClearPairs, which owns the pair.
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns
func clearableDealColumns(current crmcontracts.Deal) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"expected_close_date": {Column: "expected_close_date", Current: current.ExpectedCloseDate},
		"forecast_category":   {Column: "forecast_category", Current: current.ForecastCategory},
		"wait_until":          {Column: "wait_until", Current: current.WaitUntil},
		"owner_id":            {Column: "owner_id", Current: current.OwnerId},
		"organization_id":     {Column: "organization_id", Current: current.OrganizationId},
		"project_id":          {Column: "project_id", Current: current.ProjectId},
	}
}

// dealClearPairs names the wire fields whose clear writes BOTH halves of a
// two-column fact, keyed by every name a caller may reach it under.
//
// The partner and what that partner did are one fact:
// deal_partner_attribution_pairing rejects a row where one half survived the
// other, so there is no state in which a deal names a partner it claims nothing
// about. Both names therefore mean the same instruction — forget the partner and
// the claim together — rather than one of them being refused with advice to send
// the other. A restore reverting a partner-add names both halves as null, and
// refusing either would leave that reversal impossible to express.
//
// Routing these through clearableDealColumns instead would set a single column
// and earn a constraint violation from the database rather than a decision from
// the store.
func dealClearPairs(current crmcontracts.Deal) map[string][]storekit.Clearable {
	partner := []storekit.Clearable{
		{Column: "partner_org_id", Current: current.PartnerOrgId},
		{Column: "partner_attribution", Current: current.PartnerAttribution},
	}
	return map[string][]storekit.Clearable{
		"partner_org_id":      partner,
		"partner_attribution": partner,
	}
}

// splitDealClears applies every paired clear the caller named and returns the
// rest for storekit.ApplyClears, plus whether the partner pair was among them.
//
// The flag travels because the partner's own writer has to know: a request that
// forgets the partner while naming a claim about them has nobody left to
// attribute it to, and that is a refusal rather than a race between two writes
// to the same column.
func splitDealClears(p *storekit.Patch, fields []string, current crmcontracts.Deal) (rest []string, clearedPartner bool) {
	pairs := dealClearPairs(current)
	rest = make([]string, 0, len(fields))
	for _, field := range fields {
		pair, paired := pairs[field]
		if !paired {
			rest = append(rest, field)
			continue
		}
		clearedPartner = true
		for _, half := range pair {
			p.Set(half.Column, half.Current, nil)
		}
	}
	return rest, clearedPartner
}

// ensureClearedLinksVisible refuses to forget a link to a record the caller
// could not open.
//
// The read path withholds a deal's organization and partner from a reader whose
// row scope cannot reach them (unreadableReferences), and the write path refuses
// to SET either to a target they cannot see. Between the two sat this hole: a
// reader told "you may not see which company this is" could still detach it, and
// with the partner they could destroy the attribution a commission accrues on —
// a write about an organization they were not allowed to name.
//
// A miss reads as not-found, which is what EnsureLinkTarget already answers, so
// existence stays hidden.
//
// project_id is deliberately absent. ATTACHING a project needs write authority
// because winning the deal advances that project's phase and writes its history;
// detaching advances nothing and discloses nothing, so the asymmetry is the rule
// rather than a gap in it.
func ensureClearedLinksVisible(ctx context.Context, tx pgx.Tx, current crmcontracts.Deal, cleared []string) error {
	for _, field := range cleared {
		var target *openapi_types.UUID
		switch field {
		case filterOrganizationID:
			target = current.OrganizationId
		// Either name reaches the pair, and the permission is the partner's
		// either way: the claim is a statement about that organization.
		case filterPartnerOrgID, partnerAttributionField:
			target = current.PartnerOrgId
		default:
			continue
		}
		// Nothing there to forget, so nobody's record is being written about.
		if target == nil {
			continue
		}
		if err := auth.EnsureLinkTarget(ctx, tx, "organization", ids.UUID(*target)); err != nil {
			return err
		}
	}
	return nil
}
