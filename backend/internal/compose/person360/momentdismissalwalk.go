// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// Walking the ladder past what a reader has already put away.
//
// A dismissal silences one CARD. The record still has whatever else it had, so
// the walk has to keep going — and a rung that speaks for a set has to be asked
// again rather than passed over, or dismissing one promise would hide the two
// beside it.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// rungPast asks one rung for the best card its reader has NOT dismissed.
//
// A rung answers over a set and returns its top pick, so a dismissal has to be
// resolved inside the rung rather than around it. This hides the dismissed
// card from the page the rung reads and asks again, until the rung either
// names something the reader has not put away or has nothing left to name.
//
// Bounded by the promises the page carries: each pass removes the card it just
// rejected, so the walk is as long as the record's own promise list and no
// longer.
func rungPast(
	ctx context.Context, now time.Time, page *crmcontracts.Person360,
	rung func(context.Context, time.Time, *crmcontracts.Person360) (crmcontracts.PersonMoment, bool),
	dismissed func(crmcontracts.PersonMoment) bool,
) (crmcontracts.PersonMoment, bool) {
	seen := map[string]bool{}
	trimmed := *page
	for {
		moment, ok := rung(ctx, now, &trimmed)
		if !ok || seen[moment.ClaimKey] {
			return crmcontracts.PersonMoment{}, false
		}
		if !dismissed(moment) {
			return moment, true
		}
		seen[moment.ClaimKey] = true
		trimmed = withoutMoment(trimmed, moment)
	}
}

// withoutMoment is the page with the rows one moment spoke for removed, so the
// rung that produced it names its next-best card instead.
//
// Only the promise sources are trimmed: those are the rungs that rank over a
// set a reader dismisses one member of at a time. A rung reading a single fact
// — the next meeting, the last inbound message — would return the same card
// again from the same row, so trimming nothing leaves it to the caller's
// seen-key check and its moment is passed over.
func withoutMoment(page crmcontracts.Person360, moment crmcontracts.PersonMoment) crmcontracts.Person360 {
	if len(moment.Evidence) == 0 || moment.Evidence[0].Id == nil {
		return page
	}
	// The evidence names the row the card was built from: the source message
	// for a claim, the task itself for a task. Matching on it rather than on
	// the claim key keeps this independent of how a key is spelled.
	spoken := *moment.Evidence[0].Id
	if page.Claims != nil {
		kept := make([]crmcontracts.ConversationClaim, 0, len(*page.Claims))
		for _, claim := range *page.Claims {
			if claim.SourceActivityId != spoken || claim.Body != moment.Evidence[0].Label {
				kept = append(kept, claim)
			}
		}
		page.Claims = &kept
	}
	if page.NextSteps != nil {
		steps := *page.NextSteps
		kept := make([]crmcontracts.Activity, 0, len(steps.Data))
		for _, task := range steps.Data {
			if task.Id != spoken {
				kept = append(kept, task)
			}
		}
		steps.Data = kept
		page.NextSteps = &steps
	}
	return page
}
