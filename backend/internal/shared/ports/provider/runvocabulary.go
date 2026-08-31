// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provider

// The run's own closed vocabularies: what caused it, where it got to, and why
// it sent nothing. Split from the seam's two interfaces because these are what
// a stored row and a rendered page speak — the schema's CHECK constraints and
// the contract's enums both mirror them, and all three have to agree.

// Trigger is what caused a run to be queued (PI-DDL-2).
type Trigger string

// The closed set of things that queue a run. Every one but TriggerManual is
// Automatic, and an automatic run buys only the categories that cost nothing.
const (
	TriggerAutomaticCreate Trigger = "automatic_create"
	TriggerAutomaticImport Trigger = "automatic_import"
	// TriggerAutomaticBackfill is the catch-up sweep reaching a contact no run
	// has covered — one that existed before the provider was connected, or
	// that arrived while the posture was off. Automatic, so it buys only what
	// costs nothing.
	TriggerAutomaticBackfill Trigger = "automatic_backfill"
	TriggerScheduledRefresh  Trigger = "scheduled_refresh"
	// TriggerManual is a human asking explicitly. It is never fenced by the
	// duplicate or freshness checks: the person looking at the record knows
	// something the timestamps do not.
	TriggerManual Trigger = "manual"
)

// Automatic reports whether this trigger is subject to the fences that only
// apply to work nobody asked for directly.
func (t Trigger) Automatic() bool { return t != TriggerManual }

// RunState is the closed run-state machine (PI-STATE-2).
type RunState string

// The states a run passes through, and the four it can end in. Terminal is
// terminal: nothing moves a run out of completed, no_match, skipped, failed or
// cancelled.
const (
	RunQueued     RunState = "queued"
	RunSubmitting RunState = "submitting"
	RunInProgress RunState = "in_progress"

	RunCompleted RunState = "completed"
	RunNoMatch   RunState = "no_match"
	RunSkipped   RunState = "skipped"
	// RunSubmissionUnknown is terminal and never retried, and it still
	// occupies the live-run index: a run that may have been paid for keeps
	// blocking an identical retry until a human resolves it.
	RunSubmissionUnknown RunState = "submission_unknown"
	RunFailed            RunState = "failed"
	RunCancelled         RunState = "cancelled"
)

// Terminal reports whether the run has left the machine.
func (s RunState) Terminal() bool {
	switch s {
	case RunQueued, RunSubmitting, RunInProgress:
		return false
	default:
		return true
	}
}

// SkipReason says why a skipped run sent nothing (PI-DDL-2). Each is a
// distinct product fact: a customer must never be told their budget stopped
// something when nothing was wrong.
type SkipReason string

// Why a run sent nothing. Each is a distinct product fact a customer may be
// shown, which is why none of them collapses into a general "declined".
const (
	SkipBudgetExhausted           SkipReason = "budget_exhausted"
	SkipLowBalance                SkipReason = "low_balance"
	SkipSuppressed                SkipReason = "suppressed"
	SkipNotEligible               SkipReason = "not_eligible"
	SkipDuplicateSubjectCandidate SkipReason = "duplicate_subject_candidate"
	SkipRateLimited               SkipReason = "rate_limited"
	// SkipAlreadyFresh means a completed run is newer than the refresh
	// window, so an automatic trigger declined to buy the same data twice.
	SkipAlreadyFresh SkipReason = "already_fresh"
	// SkipNoIdentifiers means the subject carries nothing the provider can
	// match on — no profile link, and no name with a company. Declining is
	// cheaper than spending a call that can only answer "no match", and it is
	// a different fact from not_eligible: nothing forbids this purchase, there
	// is simply nothing to ask with. A human pressing the button may still try.
	SkipNoIdentifiers SkipReason = "no_identifiers"
)
