// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Deal Scout: a pass over one workspace that finds companies with no open deal
// where the evidence says commercial motion is under way, and hands each one
// to deals as a suggestion.
//
// A verified buyer request and a commitment establish the commercial matter.
// So does one explicit request for a quotation from a verified buyer.
// Meetings and documents corroborate it without assigning a buyer of their own.
//
// Only evidence the whole workspace may read is used: an activity with the
// workspace audience on a thread nobody holds, and a signal visible to the
// workspace whose cited messages are the same. A suggestion built from private
// evidence would tell colleagues, by its mere existence, about mail they may
// not read. An explicit request is the one exception: it opens a deal from
// private mail too. The opener decides what of that mail the deal shows.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
)

const (
	// dealScoutWindow is how far back evidence counts. Older motion that never
	// became a deal is not a suggestion anybody wants today.
	dealScoutWindow = 90 * 24 * time.Hour
	// dealScoutCompanyCap bounds one pass. It chooses among companies whose
	// evidence already qualifies, so a flood of lone signals cannot crowd out
	// a company with a held meeting; a company that is suggested leaves the
	// candidate set, and the next pass reaches the ones this one did not.
	dealScoutCompanyCap = 200
)

// DealScoutPass is what one pass did, for the log line.
type DealScoutPass struct {
	Superseded, Considered, Raised int
}

// RunDealScout runs one pass inside the caller's transaction, as the system
// principal the job binds.
func RunDealScout(ctx context.Context, tx pgx.Tx, now time.Time) (DealScoutPass, error) {
	return scoutPass(ctx, tx, now, dealScoutCompanyCap)
}

func scoutPass(ctx context.Context, tx pgx.Tx, now time.Time, companyCap int) (DealScoutPass, error) {
	var pass DealScoutPass
	var err error
	if pass.Superseded, err = deals.SupersedeStaleSuggestionsTx(ctx, tx, func(arg func(any) int) string {
		return dealscout.VerifiedSuggestionBuyer("s.id", "s.company_id", fmt.Sprintf("$%d::timestamptz", arg(now.Add(-dealScoutWindow))), fmt.Sprintf("$%d::timestamptz", arg(now)))
	}); err != nil {
		return pass, err
	}
	pairs, err := contacts.OpenDuplicateCompanyPairsTx(ctx, tx)
	if err != nil {
		return pass, err
	}
	raisedTwice, err := deals.SupersedeDuplicateSuggestionsTx(ctx, tx, pairs)
	if err != nil {
		return pass, err
	}
	pass.Superseded += raisedTwice
	items, err := dealscout.ReadEvidence(ctx, tx, now.Add(-dealScoutWindow), now, companyCap)
	if err != nil {
		return pass, err
	}
	base, err := identity.BaseCurrencyAppliedTx(ctx, tx)
	if err != nil {
		return pass, err
	}
	today, err := installationToday(ctx, tx, now)
	if err != nil {
		return pass, err
	}
	for _, company := range dealscout.ByCompany(items) {
		pass.Considered++
		draft := dealscout.Draft(company, base, today)
		if draft.DuplicateOf, err = contacts.OpenDuplicateCompaniesTx(ctx, tx, draft.CompanyID); err != nil {
			return pass, err
		}
		raised, err := deals.RecordSuggestionTx(ctx, tx, draft)
		if err != nil {
			return pass, err
		}
		if raised {
			pass.Raised++
		}
	}
	return pass, nil
}
