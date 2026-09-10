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

// basisOwnerKey names the seat whose permissions composed a correction's basis
// sentence, inside the same evidence map.
//
// Stored because the sentence cannot be re-derived: it may carry a contact's
// name and a correspondence date read under that seat's grants, and it is text
// by the time anybody reads it back. The receipt compares this against the
// deal's owner at read time — a deal that changed hands shows the change and
// withholds the reason, rather than disclosing a name the new owner was never
// entitled to see.
const basisOwnerKey = "basis_owner"

// ownerString renders a deal's owner for the evidence map. An unowned deal
// records the empty string, which no user id equals, so a receipt on a deal
// that has since gained an owner withholds the basis rather than matching by
// accident.
func ownerString(owner *ids.UUID) string {
	if owner == nil {
		return ""
	}
	return owner.String()
}

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

// correct applies one deal's A6 tier and reports WHAT IT DID, not which tier it
// chose. The write runs in its own audited transaction; the 🟡 staging follows
// it (Stage opens its own) — if the staging fails the provisional row simply
// re-enters the next sweep.
//
// The returned outcome is the member ledger's entry, and it has to describe the
// effect because the run's counters are built from it. A tier can decide to
// correct and then write nothing — maintenance switched off mid-pass, or the
// deal already holding everything proposed — and a ledger that recorded the
// INTENTION would let the receipt claim corrections that never reached a deal.
// That is the same "machine work that changed nothing" defect the no-op guard
// exists to stop, one layer up.
func (c *CloseDateCorrector) correct(ctx context.Context, cand closeDateCandidate, hygiene CloseDateHygiene, category string, now time.Time, loc *time.Location, runID ids.UUID) (string, error) {
	// Nothing to correct. A provisional date the sweep set and nobody has
	// changed since is simply the date this deal has — there is no card to keep
	// alive, because the correction was applied when it was made and the
	// receipt said so that morning.
	if !hygiene.Flagged {
		return closeDateMemberChecked, nil
	}

	// Whose deal it is decides whether the sweep may write it.
	//
	// Asked BEFORE the reversal check and before any write, because a rep who
	// switched this off has not asked to be told what the sweep would have
	// done — they asked for their deals to be left alone, and a run that read
	// their correspondence to build a reason it would never use would be doing
	// work they declined.
	var owner ids.UUID
	if cand.ownerID != nil {
		owner = *cand.ownerID
	}
	mayCorrect, err := c.policy.CorrectsWithoutAsking(ctx, owner)
	if err != nil {
		return "", err
	}
	if !mayCorrect {
		return closeDateMemberChecked, nil
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

	takenBack, err := c.answeredByAReversal(ctx, cand, hygiene, proposal)
	if err != nil {
		return "", err
	}
	if takenBack {
		return closeDateMemberChecked, nil
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
		_, wrote, err := c.apply(ctx, cand, label, EvidenceOf(proposal, cand.expectedClose), runID, func(p *storekit.Patch) {
			setCloseDate(p, cand.expectedClose, *hygiene.ProposedClose)
			if !cand.provisional {
				p.Set("close_date_provisional", false, true)
			}
		}, map[string]any{
			correctionFlagsKey: hygiene.Flags,
			"basis":            proposal.Basis,
			// Stamped on this tier too, though pacedBasis names nobody: the
			// receipt applies ONE rule to every correction rather than knowing
			// which tier wrote which sentence, and a tier that omitted the key
			// would have its reason withheld for the wrong reason.
			basisOwnerKey: ownerString(cand.ownerID),
		})
		if err != nil {
			return "", err
		}
		// APPLIED, not asked. The correction is already on the deal and the
		// morning's receipt carries it with an Undo — a card asking a rep to
		// confirm a date the sweep had already written was a question whose
		// answer changed nothing, and one that expired into silence when
		// nobody answered it.
		return closeDateEffect{wrote: wrote}.outcome(), nil

	case CloseDateActionDowngradeAndReview:
		return c.downgradeAndReview(ctx, cand, hygiene, category, proposal, now, loc, runID)
	}
	return "", fmt.Errorf("close-date sweep: no executor for action %q", hygiene.Action)
}

// downgradeAndReview is the 🔻 tier: a deal nobody has touched drops a forecast
// notch and its owner is asked whether it is still real.
func (c *CloseDateCorrector) downgradeAndReview(
	ctx context.Context, cand closeDateCandidate, hygiene CloseDateHygiene, category string,
	proposal CloseDateCorrection, now time.Time, loc *time.Location, runID ids.UUID,
) (string, error) {
	notched := forecastDowngrade(category)
	// The 🟡 review: gone quiet — still alive? The proposal keeps the
	// stage-velocity date the assessment computed, on BOTH branches. It
	// used to be overwritten here with the deal's CURRENT date whenever the
	// invariant did not force a re-date, which asked a human to confirm the
	// date the deal already had — a card with nothing in it to approve.
	//
	// Built BEFORE the write, because the correction records the question
	// it is answering and this branch asks a different one from the confirm
	// above: a rep who said the date is fine has not said the deal is real.
	// Recording the confirm's identity here would let one answer suppress
	// the other.
	review := proposal
	review.Asking = AskingIsThisDealAlive
	// The reason, read BEFORE the write so the receipt can carry it.
	//
	// It ran after the write while a card followed the correction: the card was
	// a second transaction and a failed read could be allowed to cost the
	// sentence rather than the downgrade. The receipt is the only telling now,
	// and quietBasis answers a fallback rather than an error, so reading it
	// first costs the same and lands the reason on the row that reports the
	// change.
	review.Basis = c.quietBasis(ctx, cand.id, now, loc)
	_, wrote, err := c.apply(ctx, cand, "downgrade_and_review", EvidenceOf(review, cand.expectedClose), runID, func(p *storekit.Patch) {
		setForecastCategory(p, cand.forecastCat, category, notched)
		// The date moves on this tier too, which is the change: a deal nobody
		// has touched carries a date nobody believes, and leaving it while
		// notching the forecast corrected the number and left the calendar
		// lying. Both are the sweep's estimate, both are marked provisional,
		// and both are on one Undo.
		setCloseDate(p, cand.expectedClose, *hygiene.ProposedClose)
		if !cand.provisional {
			p.Set("close_date_provisional", false, true)
		}
	}, map[string]any{
		correctionFlagsKey: hygiene.Flags,
		"at_risk":          true,
		// The same key the paced tiers record. The receipt reads one basis
		// whichever tier corrected the deal, so a tier that omits it renders a
		// change with no stated reason.
		"basis": review.Basis,
		// WHOSE grants composed that sentence.
		//
		// The basis can name a contact and a correspondence date, resolved
		// under the owner's own permissions on the night it was written. A deal
		// handed to somebody else later would otherwise show that sentence to a
		// rep who may hold neither person:read nor activity:read — permissions
		// nothing re-checks, because the text is already stored. The receipt
		// reader compares this against the deal's owner NOW and withholds the
		// sentence when they differ.
		basisOwnerKey: ownerString(cand.ownerID),
	})
	if err != nil {
		return "", err
	}
	return closeDateEffect{wrote: wrote}.outcome(), nil
}

// answeredByAReversal reports whether somebody has already taken back the
// correction this tier is about to make.
//
// Asked BEFORE any write, not only before the card. Every tier re-dates the deal
// first and stages second, so a check living only in ensureStaged would let the
// sweep rewrite the exact value a person had just undone and merely decline to
// ask about it — the undo would appear to work and be gone by morning.
//
// The downgrade branch asks a different question of the same deal ("is this deal
// still alive" rather than "is this date right"), and the memory keys on which:
// a rep who said the date was fine has not said the deal is real.
func (c *CloseDateCorrector) answeredByAReversal(
	ctx context.Context, cand closeDateCandidate, hygiene CloseDateHygiene, proposal CloseDateCorrection,
) (bool, error) {
	asking := EvidenceOf(proposal, cand.expectedClose)
	if hygiene.Action == CloseDateActionDowngradeAndReview {
		asking.Asking = AskingIsThisDealAlive
	}
	return c.reversedSameQuestion(ctx, cand.id, asking)
}

// closeDateEffect is what one member's turn actually produced.
//
// One field, since the sweep stopped staging cards: a tier either moved the
// deal's own row or it did not. The ledger used to carry a third outcome for a
// question newly put to a human, and nothing puts one any more.
type closeDateEffect struct {
	// wrote is a committed domain change: the deal's own row moved.
	wrote bool
}

// outcome names the member ledger's entry for this turn.
//
// What this must never do is report a change when none happened — the tier
// decided to act, and then the write found nothing to do or the maintenance
// switch was off. That turn is checked, not changed.
func (e closeDateEffect) outcome() string {
	if e.wrote {
		return closeDateMemberChanged
	}
	return closeDateMemberChecked
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
