// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Choosing and writing ONE deal's close-date correction.
//
// The pass that finds the deals lives in closedatesweep.go; this file is what
// happens to a single one once it is flagged — which tier applies, which fields
// that tier is allowed to move, and the two guards that keep an unchanged value
// out of the audit trail.
//
// The split is by concept rather than by size: a reader asking "what does the
// sweep do to a deal" reads this file, and one asking "which deals does it
// reach" reads the other.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The §7 forecast categories, as the deal column spells them.
//
// Named here rather than imported: forecasting declares the same four words for
// its own readings, and a module never imports a sibling. Two spellings of a
// database enum is the price of that rule, and the column is what holds them
// together — a change to either would fail the other's tests against it.
// correctionFlagsKey names the hygiene findings inside a correction's
// deal.updated payload — one spelling for all three tiers.
const correctionFlagsKey = "flags"

const (
	forecastCommit   = "commit"
	forecastBestCase = "best_case"
	forecastPipeline = "pipeline"
	forecastOmitted  = "omitted"
)

// effectiveForecastCategory is the §7 reading: the rep's explicit
// override wins; otherwise the stage probability derives the default
// (commit ≥ 90, best-case ≥ 50).
func effectiveForecastCategory(override *string, winProbability int) string {
	if override != nil {
		return *override
	}
	switch {
	case winProbability >= forecastCommitMinProb:
		return forecastCommit
	case winProbability >= lateStageMinProb:
		return forecastBestCase
	default:
		return forecastPipeline
	}
}

// forecastDowngrade is the 🔻 notch: Commit→Best-case→Pipeline→Omitted,
// never below Omitted.
func forecastDowngrade(category string) string {
	switch category {
	case forecastCommit:
		return forecastBestCase
	case forecastBestCase:
		return forecastPipeline
	default:
		return forecastOmitted
	}
}

// setCloseDate assigns the sweep's proposed date only where it differs from the
// one the deal already claims.
//
// The sweep re-flags a deal every night it stays quiet, and its proposal is
// derived from stage velocity rather than from the calendar — so it frequently
// recomputes the date the deal already has. storekit.Patch records an assignment
// without comparing it, so an unconditional Set would put that date in the audit
// diff and in deal_forecast_history on every pass, and a reconstruction would
// read a forecast moving nightly while standing still.
func setCloseDate(p *storekit.Patch, before *time.Time, proposed time.Time) {
	if before != nil && before.Equal(proposed) {
		return
	}
	p.SetDate(closeDateField, before, &proposed)
}

// setForecastCategory assigns the notched category only where it differs from
// the one the deal effectively carries.
//
// The same reasoning as setCloseDate, and the branch it guards used to lack it.
// forecastDowngrade floors at "omitted", so a deal already sitting there was
// assigned "omitted" again every night it stayed quiet — a patch that is not
// Empty(), so an audit row and a deal.updated event were written for a change
// that did not happen, forever.
//
// The comparison is against the EFFECTIVE category rather than the stored
// column, because the column is nullable and the two disagree: a deal with no
// override carries NULL while reading as its probability-derived default. The
// notch is computed from the effective reading, so the guard has to ask the
// same question — comparing against a raw NULL would find every value
// different and write on every pass, which is the bug it exists to stop.
func setForecastCategory(p *storekit.Patch, stored *string, effective, notched string) {
	if effective == notched {
		return
	}
	p.Set("forecast_category", stored, notched)
}

// correct applies one deal's A6 tier. The write runs in its own audited
// transaction; the 🟡 staging follows it (Stage opens its own) — if the
// staging fails the provisional row simply re-enters the next sweep.
func (c *CloseDateCorrector) correct(ctx context.Context, cand closeDateCandidate, hygiene CloseDateHygiene, category string, now time.Time, loc *time.Location) error {
	if !hygiene.Flagged {
		if cand.provisional {
			// The date itself is clean (the sweep set it), but the human
			// has not confirmed it yet: keep the 🟡 surface alive if the
			// previous staging expired undecided.
			//
			return c.ensureStaged(ctx, cand, 0, CloseDateCorrection{
				DealID:              cand.id,
				ExpectedCloseDate:   cand.expectedClose.Format(time.DateOnly),
				PreviousCloseDate:   dateString(cand.expectedClose),
				RemainingOpenStages: StagesRemaining(cand.remainingOpen),
				Asking:              AskingIsThisDateRight,
				Basis:               quietHoldingBasis,
			})
		}
		return nil
	}

	proposal := CloseDateCorrection{
		DealID:              cand.id,
		ExpectedCloseDate:   hygiene.ProposedClose.Format(time.DateOnly),
		PreviousCloseDate:   dateString(cand.expectedClose),
		RemainingOpenStages: StagesRemaining(cand.remainingOpen),
		Asking:              AskingIsThisDateRight,
		Flags:               hygiene.Flags,
		Basis:               pacedBasis(StagesToGo(cand.remainingOpen)),
	}

	switch hygiene.Action {
	// 🟢 and 🟡 write the SAME THING, and sharing the branch is the point.
	//
	// Both replace the date with proposedCloseDate's stage-velocity estimate,
	// mark it provisional, and raise the confirm. 🟢 used to clear
	// close_date_provisional instead, which made a guess indistinguishable from
	// a date a customer had given: it counted toward supported forecast claims,
	// and the rep saw no sign that nobody had confirmed it. Neither path reads
	// a buyer's message, so neither may call its replacement final — that needs
	// attributable evidence or a person's answer.
	//
	// What still separates the tiers is the AUDIT LABEL, which records the
	// policy that admitted the deal: auto_apply for a clear-overdue early-stage
	// deal the sweep corrects without waiting, provisional_confirm for
	// everything else. One write, two names for why it happened.
	case CloseDateActionAutoApply, CloseDateActionProvisionalConfirm:
		label := "provisional_confirm"
		if hygiene.Action == CloseDateActionAutoApply {
			label = "auto_apply"
		}
		version, err := c.apply(ctx, cand, label, func(p *storekit.Patch) {
			setCloseDate(p, cand.expectedClose, *hygiene.ProposedClose)
			if !cand.provisional {
				p.Set("close_date_provisional", false, true)
			}
		}, map[string]any{correctionFlagsKey: hygiene.Flags, "basis": proposal.Basis})
		if err != nil {
			return err
		}
		return c.ensureStaged(ctx, cand, version, proposal)

	case CloseDateActionDowngradeAndReview:
		notched := forecastDowngrade(category)
		version, err := c.apply(ctx, cand, "downgrade_and_review", func(p *storekit.Patch) {
			setForecastCategory(p, cand.forecastCat, category, notched)
			if hygiene.Provisional {
				// Only the invariant forces a date onto a quiet deal —
				// never an optimistic re-date on top of the downgrade.
				setCloseDate(p, cand.expectedClose, *hygiene.ProposedClose)
				if !cand.provisional {
					p.Set("close_date_provisional", false, true)
				}
			}
		}, map[string]any{correctionFlagsKey: hygiene.Flags, "at_risk": true})
		if err != nil {
			return err
		}
		// The 🟡 review: gone quiet — still alive? The proposal keeps the
		// stage-velocity date the assessment computed, on BOTH branches. It
		// used to be overwritten here with the deal's CURRENT date whenever the
		// invariant did not force a re-date, which asked a human to confirm the
		// date the deal already had — a card with nothing in it to approve.
		review := proposal
		// A different question from the 🟡 confirm, and the memory keys on which:
		// a rep who said this date is fine has not said the deal is still alive.
		review.Asking = AskingIsThisDealAlive
		review.Basis = c.quietBasis(ctx, cand.id, now, loc)
		return c.ensureStaged(ctx, cand, version, review)
	}
	return fmt.Errorf("close-date sweep: no executor for action %q", hygiene.Action)
}

// quietBasis is the reason the quiet review shows: which way the silence runs,
// who is on the far end of it, and how long it has lasted.
//
// A failure to READ the correspondence is not a reason to fail the sweep — the
// downgrade has already committed and the review is what is left to raise. So a
// read error degrades to the generic sentence and is logged, rather than
// aborting a pass over every other deal in the workspace.
func (c *CloseDateCorrector) quietBasis(ctx context.Context, dealID ids.DealID, now time.Time, loc *time.Location) string {
	facts, names, err := c.reviewer.ReadForOwner(ctx, dealID)
	if err != nil {
		c.log.WarnContext(ctx, "close-date quiet review fell back to a generic reason",
			"deal_id", dealID, "error", err)
		return quietFallbackBasis
	}
	return quietReason(facts, names, now, loc)
}
