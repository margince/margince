// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A rule turning itself off when its own record goes bad.
//
// The governance file beside this one has the WRITER; what lives here is when
// the product calls it without being asked. Two triggers, and they are
// deliberately not the same shape:
//
//   - The MEASURED one. Corrections and reversals crossing the transition's
//     own ceiling, over enough reviewed proposals to mean something. It runs
//     on the decision path, in the transaction that records the outcome, so
//     the count that trips it INCLUDES the decision that tripped it.
//   - The SAFETY one. An authorization, cross-tenant, evidence-link or
//     protected-field defect suspends immediately, at any volume, because
//     those say the transition is doing something it was never allowed to do
//     — and waiting for two hundred of them to accumulate is waiting for two
//     hundred of them to happen.
//
// The second is not a stricter version of the first. A ceiling asks "is this
// still working well enough"; a defect asks "did this do something wrong",
// and there is no volume at which the answer to the second becomes acceptable.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// SafetyDefect is a way a stage move can be wrong that no amount of volume
// excuses.
//
// A CLOSED set rather than a free string, because each member is a claim that
// the product did something it had no right to do, and a caller who can invent
// a new one can suspend a rule for a reason nobody agreed was a safety matter.
type SafetyDefect string

const (
	// DefectAuthorization is a move applied that its principal was not allowed
	// to make.
	DefectAuthorization SafetyDefect = "authorization"
	// DefectCrossTenant is a move that touched a record outside the workspace
	// it was acting in.
	DefectCrossTenant SafetyDefect = "cross_tenant"
	// DefectEvidenceLink is a criterion whose evidence did not belong to the
	// deal it was cited for.
	DefectEvidenceLink SafetyDefect = "evidence_link"
	// DefectProtectedField is a move that wrote a field the deal's protections
	// put out of reach.
	DefectProtectedField SafetyDefect = "protected_field"
)

// safetyDefects is what each defect means to an operator reading a suspension.
//
// It is also the ADMISSION list: a defect absent from here is refused rather
// than written, so a caller cannot suspend a rule for a reason nobody agreed
// was a safety matter. That makes forgetting an entry the dangerous direction
// — a declared defect the map does not carry is one the product cannot
// suspend on at all, and it fails silently at the call site.
//
// Held by: TestEverySafetyDefectCanActuallySuspendARule
// (backend/gates/safetydefects_test.go)
var safetyDefects = map[SafetyDefect]string{
	DefectAuthorization:  "a move was applied that its actor was not authorized to make",
	DefectCrossTenant:    "a move reached a record outside its own workspace",
	DefectEvidenceLink:   "a criterion cited evidence belonging to another deal",
	DefectProtectedField: "a move wrote a field the deal's protections hold",
}

// SuspendForSafetyDefect turns a transition's automation off immediately.
//
// NO VOLUME TEST, and that is the whole point of it being a separate entry
// point. The measured sweep asks whether a transition is still working well
// enough to keep trusting; this one has already been told it did something it
// was not allowed to do, and one of those is one too many.
//
// Called by the applier when a move it made turns out to have crossed a line
// the product holds. It is safe to call for a transition with no rule at all —
// there is nothing to suspend, and refusing would make the caller carry a
// branch about whether automation happened to be configured.
func (s *Store) SuspendForSafetyDefect(
	ctx context.Context, in TransitionRef, defect SafetyDefect,
) error {
	why, known := safetyDefects[defect]
	if !known {
		return fmt.Errorf("deals: %q is not a stage automation safety defect", defect)
	}
	err := s.SuspendTransitionPolicy(ctx, in, "safety: "+why)
	if errors.Is(err, apperrors.ErrNotFound) {
		// No rule on this transition. Nothing was automatic, so nothing needs
		// turning off — and the defect is still recorded by whoever detected
		// it. Answering an error here would push a branch onto every caller
		// for the ordinary case where a transition was never configured.
		return nil
	}
	return err
}

// suspendIfRecordWentBadTx turns a rule off when corrections and reversals have
// crossed its own ceiling.
//
// IN THE DECISION'S TRANSACTION, deliberately. Run as a later sweep, the
// window between the outcome that crossed the ceiling and the suspension is a
// window in which the rule is still on and still applying moves on a record
// that has already failed. Here, the decision and the suspension it justifies
// commit together or not at all.
//
// It LOCKS THE RULE BEFORE READING ITS THRESHOLDS, and the order matters. Read
// unlocked, an admin raising the ceiling between the count and the write would
// be overruled by a judgement made against the number they had just replaced —
// the rule goes off citing a threshold that no longer exists, and the reason an
// operator reads names a bar nobody configured.
//
// The threshold is BOTH directions of one question: enough reviewed proposals
// for the rate to mean anything, AND the rate above the ceiling. A transition
// with three reviews and one reversal is at 33%, which is not a bad record —
// it is no record at all, and suspending on it would turn automation off for
// every transition on its third day.
func suspendIfRecordWentBadTx(
	ctx context.Context, tx pgx.Tx, in TransitionRef, now time.Time,
) error {
	rule, err := lockTransitionPolicy(ctx, tx, in)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// No rule configured. Nothing to suspend, and the decision that
			// brought us here is an ordinary proposal being answered.
			return nil
		}
		return err
	}
	if rule.Suspended() {
		// Already off. Re-counting would spend a query to reach the branch
		// that keeps the first reason anyway.
		return nil
	}
	rates, err := transitionRatesTx(ctx, tx, *rule, now)
	if err != nil {
		return err
	}
	if !rule.recordWentBad(rates) {
		return nil
	}
	return suspendTransitionPolicyTx(ctx, tx, in, fmt.Sprintf(
		"%.1f%% of %d reviewed moves on this transition were undone or corrected, "+
			"above the %.1f%% ceiling",
		rates.UnsafeRate()*100, rates.Reviewed, rule.CorrectionReversalThreshold*100))
}

// recordWentBad answers whether this rule's own numbers say to stop.
//
// A METHOD ON THE RULE, not a package constant, because every threshold here
// is the transition's own: an installation that set a stricter ceiling on one
// transition means it for that transition, and a sweep applying a hard-coded
// 1% would quietly ignore what an admin configured.
//
// MinReviewed does double duty — it is the same number that gates turning
// automation on. That is intended: the volume at which a record is worth
// trusting and the volume at which it is worth distrusting are one judgement,
// and letting them drift apart gives a transition a band where it may run
// automatically but may not be stopped by its own numbers.
func (p TransitionPolicy) recordWentBad(r TransitionRates) bool {
	if r.Reviewed < p.MinReviewed {
		return false
	}
	return r.UnsafeRate() > p.CorrectionReversalThreshold
}
