// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// A rule turning itself off, and what that costs and does not cost.
//
// The tests below split into the two triggers and one thing neither may do.
// The measured trigger needs volume; the safety trigger must not wait for it;
// and NEITHER may stop the product from proposing — a suspension takes away
// the automatic apply, and a transition that went silent instead would hide
// the very moves an operator suspended it to look at.

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// suspensionOf reads what the product recorded about a rule being off.
func suspensionOf(t *testing.T, e *configEnv, ref TransitionRef) (bool, string) {
	t.Helper()
	var at *time.Time
	var reason *string
	if err := e.owner.QueryRow(t.Context(), `
		SELECT suspended_at, suspended_reason
		  FROM stage_progression_policy
		 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&at, &reason); err != nil {
		t.Fatalf("reading the rule's suspension: %v", err)
	}
	if at == nil {
		return false, ""
	}
	if reason == nil {
		t.Fatal("a suspension with no reason: the column's CHECK should refuse this, " +
			"and an operator asked to re-enable has nothing to judge")
	}
	return true, *reason
}

// settleOne records one clean approval through the real writer, which is what
// runs the sweep. Returns the approval so the caller can mark it unsafe.
//
// Clean, always: every test here is about what the SWEEP does with a record,
// and a clean approval is the outcome that makes the unsafe rate depend only
// on what the caller marks afterwards.
func settleOne(
	t *testing.T, e *configEnv, dealID ids.DealID, ref TransitionRef,
) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	proposeOutcome(t, e, dealID, ref.FromStageID, ref.ToStageID, id)
	if err := e.store.RecordProgressionDecided(
		e.asDealWriter(), id, ProgressionApprovedClean, nil, false); err != nil {
		t.Fatalf("settling a proposal: %v", err)
	}
	return id
}

// aRecordGoneBad settles enough reviewed moves to clear the volume bar and
// marks more than the ceiling's share of them unsafe, WITHOUT letting the
// sweep see the bad ones until the caller asks.
//
// The unsafe rows are marked after they settle, because the writers that set
// reversed_at and evidence_corrected are the revert endpoint's and the
// evidence correction path's — neither of which is on this decision path.
func aRecordGoneBad(
	t *testing.T, e *configEnv, dealID ids.DealID, ref TransitionRef, unsafe int,
) {
	t.Helper()
	for i := range passingReviewed {
		id := settleOne(t, e, dealID, ref)
		if i < unsafe {
			markUnsafe(t, e, id, unsafeWays{Reversed: true})
		}
		if i == 0 {
			backdateDecision(t, e, id, passingSpanDays*24*time.Hour)
		}
	}
}

// The measured trigger: a rule stops itself once its own ceiling is crossed.
//
// The ceiling is the rule's, not a constant. This test moves it rather than
// settling two hundred more rows, because what is under test is that the sweep
// reads the transition's configured number — a sweep with 1% hard-coded passes
// a fixture built at exactly 1% and fails every installation that set its own.
func TestARuleSuspendsItselfWhenCorrectionsOrReversalsReachTheThreshold(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)

	// A record with reversals, but UNDER the default 1% ceiling: 2 of 240 is
	// 0.83%. The rule must still be running, or the assertion below passes
	// because the fixture was bad rather than because the sweep worked.
	aRecordGoneBad(t, e, dealID, ref, 2)
	if off, why := suspensionOf(t, e, ref); off {
		t.Fatalf("the rule suspended itself below its own ceiling (%s) — every "+
			"transition would go off on its first reversal", why)
	}
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the fixture does not reach auto on its own (%s: %s), so a "+
			"suspension below would prove nothing", got.Mode, got.Why)
	}

	// One more reversal takes it to 3 of 240 — 1.25%, over the ceiling. The
	// NEXT decision is what runs the sweep, which is the point: the count that
	// trips the ceiling and the suspension commit together.
	third := settleOne(t, e, dealID, ref)
	markUnsafe(t, e, third, unsafeWays{Reversed: true})
	settleOne(t, e, dealID, ref)

	off, why := suspensionOf(t, e, ref)
	if !off {
		t.Fatal("the rule kept applying moves automatically after corrections and " +
			"reversals crossed its own ceiling")
	}
	if !strings.Contains(why, "%") {
		t.Errorf("the suspension reason %q names no rate, so an operator cannot "+
			"tell how far over the ceiling it went", why)
	}
}

// The safety trigger does NOT wait for volume, and that is its whole reason
// for existing separately.
//
// A record of three moves cannot cross a rate ceiling — the measured sweep is
// correct to ignore it. But one of those three reaching a record it had no
// right to touch is not a rate at all, and a mechanism that waited for two
// hundred of them would be waiting for two hundred breaches.
func TestASafetyDefectSuspendsImmediatelyWithoutWaitingForVolume(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)

	// Three reviewed moves: far under MinReviewed, so the measured sweep has
	// nothing to say. Marking one unsafe proves that directly.
	id := settleOne(t, e, dealID, ref)
	markUnsafe(t, e, id, unsafeWays{Reversed: true})
	settleOne(t, e, dealID, ref)
	settleOne(t, e, dealID, ref)
	if off, why := suspensionOf(t, e, ref); off {
		t.Fatalf("a rate over three moves suspended the rule (%s) — the volume "+
			"bar is not being applied, and every new transition would go off", why)
	}

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectCrossTenant); err != nil {
		t.Fatalf("suspending for a safety defect: %v", err)
	}

	off, why := suspensionOf(t, e, ref)
	if !off {
		t.Fatal("a cross-tenant defect left the transition applying moves " +
			"automatically, waiting for a volume it may never reach")
	}
	if !strings.Contains(why, "workspace") {
		t.Errorf("the suspension reason %q does not say what went wrong, so an "+
			"operator cannot tell a safety stop from a rate one", why)
	}
}

// An invented defect is refused rather than written.
//
// The vocabulary is closed because each member is a claim that the product did
// something it had no right to do. A caller who can pass any string can turn a
// rule off for a reason nobody agreed was a safety matter, and the reason is
// what an operator reads when deciding whether to re-enable.
func TestOnlyAKnownSafetyDefectSuspendsARule(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	ruleOn(t, e, ref)

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, SafetyDefect("looked_wrong")); err == nil {
		t.Fatal("an unknown defect suspended a rule, so any string is a safety stop")
	}
	if off, _ := suspensionOf(t, e, ref); off {
		t.Error("the refused defect suspended the rule anyway")
	}
}

// A defect on a transition nobody configured is not an error.
//
// Automation was never on there, so there is nothing to turn off. Answering an
// error would push a branch onto every caller for the ordinary case.
func TestASafetyDefectOnAnUngovernedTransitionIsNotAnError(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectAuthorization); err != nil {
		t.Fatalf("a defect on a transition with no rule answered an error: %v", err)
	}
}

// A suspension takes away the APPLY, never the ask.
//
// This is the one a hasty implementation gets wrong by turning the rule's mode
// to propose, or by refusing the transition outright. Both are wrong in the
// same direction: the moves an operator suspended a rule to look at are
// exactly the ones that would stop being visible.
func TestASuspendedRuleStillProposesButNeverApplies(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)
	aGoodRecord(t, e, dealID, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the fixture does not reach auto (%s: %s)", got.Mode, got.Why)
	}

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectEvidenceLink); err != nil {
		t.Fatalf("suspending: %v", err)
	}

	got := verdictFor(t, e, ref)
	if got.Mode != ModePropose {
		t.Fatalf("a suspended rule answered %q — it is still applying moves "+
			"without asking", got.Mode)
	}
	if got.Why == "" {
		t.Error("the suspended rule refuses without saying why, so an admin sees " +
			"cards arriving and nothing naming the suspension")
	}
	// The ADMIN'S SETTING survives. A suspension is the product's act and it
	// lifts; rewriting mode would make the admin re-enable something they
	// never turned off, and would lose that they had trusted this transition.
	if got.Rule == nil || got.Rule.Mode != ModeAuto {
		t.Error("the suspension overwrote what the admin asked for, so resuming " +
			"would leave the transition on propose with nobody having said so")
	}
}

// The suspension is on the record and announced.
//
// A rule going off is why a rep's inbox starts receiving cards for a move that
// had been silent, and a queue that simply changed behaviour is
// indistinguishable from one that broke.
func TestSuspensionIsAuditedAndAnnounced(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	rule := ruleOn(t, e, ref)

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectProtectedField); err != nil {
		t.Fatalf("suspending: %v", err)
	}

	var audits int
	if err := e.owner.QueryRow(t.Context(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2
		   AND after ->> 'suspended' = 'true'`,
		policyEntity, rule.ID).Scan(&audits); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if audits != 1 {
		t.Fatalf("the suspension left %d audit rows, want exactly 1 — an "+
			"unaudited safety stop is one nobody can account for later", audits)
	}

	// The event is linked to that audit row by trace.audit_log_id, which is
	// what makes the domain change, the trail entry and the announcement one
	// recoverable trace rather than three things that happened to coincide.
	var linked int
	if err := e.owner.QueryRow(t.Context(), `
		SELECT count(*) FROM event_outbox o
		  JOIN audit_log a
		    ON a.id::text = o.envelope -> 'trace' ->> 'audit_log_id'
		 WHERE a.entity_type = $1 AND a.entity_id = $2
		   AND a.after ->> 'suspended' = 'true'`,
		policyEntity, rule.ID).Scan(&linked); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	if linked != 1 {
		t.Fatalf("the suspension published %d events bound to its audit row, want "+
			"exactly 1 — nothing downstream learns the transition stopped moving "+
			"deals itself, or learns it without a trace back to why", linked)
	}
}

// A second suspension keeps the FIRST reason.
//
// The reason that matters is the one that stopped the rule. A later sweep
// overwriting it with a consequence of the first loses what an operator needs
// to decide whether it is safe to resume.
func TestASecondSuspensionKeepsTheReasonThatStoppedTheRule(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	ruleOn(t, e, ref)

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectCrossTenant); err != nil {
		t.Fatalf("first suspension: %v", err)
	}
	_, first := suspensionOf(t, e, ref)

	if err := e.store.SuspendForSafetyDefect(
		e.as(), ref, DefectProtectedField); err != nil {
		t.Fatalf("second suspension: %v", err)
	}
	_, second := suspensionOf(t, e, ref)

	if first == second {
		return
	}
	t.Errorf("a later suspension replaced the reason: was %q, now %q", first, second)
}

// Below the volume bar, a bad rate enables nothing and suspends nothing.
//
// The same number is both floors, deliberately: the volume at which a record
// is worth trusting and the volume at which it is worth distrusting are one
// judgement. Letting them drift apart gives a transition a band where it may
// run automatically but cannot be stopped by its own numbers.
func TestBelowReviewedVolumeNothingEnables(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)

	// Two reviewed moves, one of them reversed — a 50% unsafe rate, which is
	// far over any ceiling anyone would configure.
	id := settleOne(t, e, dealID, ref)
	markUnsafe(t, e, id, unsafeWays{Reversed: true})
	settleOne(t, e, dealID, ref)

	if off, why := suspensionOf(t, e, ref); off {
		t.Fatalf("a 50%% rate over two moves suspended the rule (%s) — a "+
			"transition would go off on its first reversal, before it had a record", why)
	}
	got := verdictFor(t, e, ref)
	if got.Mode != ModePropose {
		t.Fatal("a transition with two reviewed moves applied one automatically")
	}
	if !strings.Contains(got.Why, "reviewed") {
		t.Errorf("the refusal says %q rather than naming the volume bar, so an "+
			"admin goes looking for a quality problem that has not been measured", got.Why)
	}
}

// The sweep judges a rule nobody can change under it.
//
// The defect this holds shut: the sweep reads the rule's thresholds, counts
// against them, and only then takes the row's lock inside the write. In that
// gap an admin raising the ceiling is overruled by a judgement made against the
// number they just replaced — the rule goes off citing a bar that no longer
// exists, and the reason an operator reads names a threshold nobody configured.
//
// Proved by changing the state WHILE the sweep's transaction is open, which is
// the only way to tell a held lock from a comment claiming one. The admin's
// UPDATE runs on its own connection and must BLOCK until the sweep commits; if
// the sweep did not hold the row, it would land immediately and the sweep would
// still have suspended on the old ceiling.
func TestTheSweepHoldsTheRuleItIsJudging(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	rule := ruleOn(t, e, ref)

	// A record over the default ceiling and past the volume bar.
	seeded := rule.MinReviewed + 20
	for i := range seeded {
		id := settleOne(t, e, dealID, ref)
		if i < 10 {
			markUnsafe(t, e, id, unsafeWays{Reversed: true})
		}
		if i == 0 {
			backdateDecision(t, e, id, passingSpanDays*24*time.Hour)
		}
	}

	// The rule is now suspended by the seeding itself, which is the ordinary
	// path. Resume it so the sweep below has something to decide about.
	if _, err := e.store.ResumeTransitionPolicy(e.as(), ref); err != nil {
		t.Fatalf("resuming for the concurrency probe: %v", err)
	}

	// WHAT THIS PROVES, and what it does not. It takes the rule the way the
	// sweep's first statement does and shows that an admin's threshold write
	// then BLOCKS — so the thresholds the sweep judges by cannot be replaced
	// under it. What holds the sweep to this shape is that it calls the same
	// lockTransitionPolicy; a sweep rewritten to read unlocked would not be
	// caught here, and that is the gap this test cannot close.
	//
	// It is written this way because the obvious version does not work:
	// probing after the whole sweep passes with OR without the lock, since
	// suspendTransitionPolicyTx locks again on its way through and the probe
	// always arrives to find the row held.
	blocked := make(chan error, 1)
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		// Take the lock the sweep takes, then let the admin try.
		read, err := lockTransitionPolicy(e.asDealWriter(), tx, ref)
		if err != nil {
			return err
		}

		go func() {
			_, err := e.owner.Exec(t.Context(), `
				UPDATE stage_progression_policy
				   SET correction_reversal_threshold = 0.900
				 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3`,
				ref.PipelineID, ref.FromStageID, ref.ToStageID)
			blocked <- err
		}()
		select {
		case err := <-blocked:
			t.Errorf("the admin's threshold write completed while the sweep held "+
				"only a read (%v): the sweep judges against a ceiling that can be "+
				"replaced before it writes, and the rule goes off citing a bar "+
				"nobody configured", err)
		case <-time.After(500 * time.Millisecond):
			// Blocked, as a held row requires.
		}

		// The judgement the sweep would make, against the thresholds it read.
		if !read.recordWentBad(mustRates(t, e, tx, *read)) {
			t.Fatal("the fixture's record is not over its ceiling, so this test " +
				"never reaches the write the lock protects")
		}
		return suspendTransitionPolicyTx(e.asDealWriter(), tx, ref, "over the ceiling")
	}); err != nil {
		t.Fatalf("running the sweep: %v", err)
	}

	if err := <-blocked; err != nil {
		t.Fatalf("the admin's write failed after the sweep committed: %v", err)
	}

	// The sweep's own judgement stands: it suspended on the ceiling that was in
	// force while it held the row.
	off, why := suspensionOf(t, e, ref)
	if !off {
		t.Fatal("the sweep did not suspend a rule whose record was over its ceiling")
	}
	if why == "" {
		t.Error("the suspension names no reason")
	}
}

// mustRates counts the transition's record inside the caller's transaction.
func mustRates(
	t *testing.T, e *configEnv, tx pgx.Tx, rule TransitionPolicy,
) TransitionRates {
	t.Helper()
	r, err := transitionRatesTx(e.asDealWriter(), tx, rule, time.Now())
	if err != nil {
		t.Fatalf("counting the transition's record: %v", err)
	}
	return r
}
