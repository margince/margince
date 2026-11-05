// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal list's partner dials: whether a partner brought the deal, and
// what it did. Each answers only where the partner itself may be read.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// partnerAttributionFilterClause narrows to what a partner did for the deal,
// and carries the partner's own visibility with it.
//
// Without that second half the filter is an existence oracle for the fact the
// field mask withholds. A caller whose read of a deal masks both partner
// columns could ask for `partner_attribution=sourced`. The row's presence would
// then tell them that some partner brought it. The mask and the filter have to
// withhold the same fact, so this arm answers only for deals whose partner the
// caller could open.
func partnerAttributionFilterClause(ctx context.Context, attribution string, arg func(any) int) (string, error) {
	if err := validPartnerAttribution(attribution); err != nil {
		return "", err
	}
	// The claim travels with the partner, so a role withholding one withholds
	// both. This arm answers for the rows a mask already declined to name.
	if err := auth.RefuseUnreadableReferenceFilter(ctx, filterPartnerAttribution, "company"); err != nil {
		return "", err
	}
	if err := auth.RefuseMaskedFilter(ctx, maskObject, filterPartnerAttribution, filterPartnerAttribution); err != nil {
		return "", err
	}
	clause := storekit.SQLf("partner_attribution = $%d", arg(attribution))
	scope, err := auth.ScopeClauseFor(ctx, "company", "pref", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		return clause, nil
	}
	return clause + storekit.SQLf(
		" AND EXISTS (SELECT 1 FROM company pref WHERE pref.id = partner_company_id AND %s)", scope), nil
}

func appendPartnerFilters(ctx context.Context, where []string, in ListDealsInput, arg func(any) int) ([]string, error) {
	if in.PartnerSourced != nil {
		// Whether a partner is named at all is a reading of partner_company_id,
		// and the page's membership is the answer the mask declined to give.
		if err := auth.RefuseUnreadableReferenceFilter(ctx, filterPartnerSourced, "company"); err != nil {
			return nil, err
		}
		if err := auth.RefuseMaskedFilter(ctx, maskObject, filterPartnerSourced, filterPartnerCompanyID); err != nil {
			return nil, err
		}
		if *in.PartnerSourced {
			where = append(where, PartnerSourcedSQL(""))
		} else {
			where = append(where, "NOT "+PartnerSourcedSQL(""))
		}
	}
	if in.PartnerAttribution != nil {
		clause, err := partnerAttributionFilterClause(ctx, *in.PartnerAttribution, arg)
		if err != nil {
			return nil, err
		}
		where = append(where, clause)
	}
	return where, nil
}
