// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithEmailRowFacts fills in what a page's email rows say beyond the message
// itself: how many files came with it, what happened when it was sent, and
// whether everyone here can read it.
//
// ONE entry point rather than a pass per fact, because they are one
// obligation. Both are content, both are read once for the whole page, both
// are refused on a withheld row — and a page that picked up one and not the
// other would show a parked message as though it had gone.
//
// No statements at all when the page carries no readable email.
func WithEmailRowFacts(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	emailIDs := emailIDsOf(page)
	if len(emailIDs) == 0 {
		return nil
	}
	counts, err := AttachmentCountsFor(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyAttachmentCounts(page, counts)

	states, err := EmailStatesFor(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyEmailStates(page, states)

	// The badge starts at the narrow word and is widened only here, where the
	// question can actually be asked. A projection with no transaction cannot
	// know whether a workspace audience is narrowed by the records it is filed
	// against, and a privacy label that guesses must guess DOWN: overstating
	// the audience makes a reader more careful than they need to be, and
	// understating it makes them less.
	reach, err := auth.ActivitiesReachingEverySeat(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyEverySeatReach(page, reach)
	return nil
}

// applyEverySeatReach widens `team` to `workspace` on the rows a stranger can
// reach. Only that word is touched: participants, selected and withheld are
// narrower answers that a workspace-wide filing does not change.
func applyEverySeatReach(page []crmcontracts.Activity, reach map[ids.UUID]bool) {
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil || summary.DisplayStatus != crmcontracts.EmailAccessStatusTeam {
			continue
		}
		if reach[ids.UUID(summary.ActivityId)] {
			summary.DisplayStatus = crmcontracts.EmailAccessStatusWorkspace
		}
	}
}
