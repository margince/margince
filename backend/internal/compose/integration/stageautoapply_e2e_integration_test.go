// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A transition that earned it moving a deal by itself.
//
// Driven through compose.SweepAutoApply — the same seam the job calls — so
// these prove the WIRING and not a re-derivation of it. A test that called the
// applier's pieces itself would pass over a sweep that never looks for stage
// progressions, which is the shape of defect that hid under WP4b's green
// module suite.
//
// Every test here is about a REFUSAL except two, and that asymmetry is the
// design. Nobody notices a card that arrived when it needn't have; everybody
// notices a deal that moved on its own when it should not have.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// governedTransition puts the fixture's transition on auto with a record good
// enough to clear every threshold, and turns the installation switch on.
//
// Returns the transition, so a test that wants to prove ONE condition withholds
// can move that one and leave the rest true.
func governedTransition(t *testing.T, e *Env, deal ids.DealID) deals.TransitionRef {
	t.Helper()
	giveTheOwnerARealRole(t, e)
	ref := transitionOfDeal(t, e, deal)
	setAutopilotSwitch(t, e, true)
	if _, err := e.Deals.SetTransitionPolicy(e.Admin(), deals.SetTransitionPolicyInput{
		TransitionRef: ref, Mode: deals.ModeAuto,
	}); err != nil {
		t.Fatalf("enabling the transition: %v", err)
	}
	seedGoodRecord(t, e, deal, ref)
	return ref
}

// giveTheOwnerARealRole grants the deal's owner the permissions the applier
// will act under.
//
// NOT decoration. The auto-applier binds the OWNER's own grants, seat and row
// scope — an automatic apply is bounded by exactly what that rep could have
// done by hand — so an owner with no role assignment cannot decide their own
// card, and the sweep correctly applies nothing. A fixture that skipped this
// would make every refusal in this file pass without the product refusing
// anything.
//
// The permissions are the ones the path actually reads: the deal to move it,
// the pipeline to read the rule that governs the move, and the installation
// settings to read the kill switch.
func giveTheOwnerARealRole(t *testing.T, e *Env) {
	t.Helper()
	const roleKey = "stage-autopilot-rep"
	e.WsExec(t, `INSERT INTO role (key, name, permissions)
		VALUES ($1, 'Stage Autopilot Rep', $2::jsonb)`,
		roleKey,
		`{"objects":{"deal":{"read":true,"update":true},`+
			`"pipeline":{"read":true},`+
			`"installation_settings":{"read":true}},"row_scope":"all"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, $1 FROM role r WHERE r.key = $2`,
		e.Rep1, roleKey)
}

// transitionOfDeal names the move the staged card proposes, read from the card
// rather than assumed: the proposer chooses the target stage, and a test that
// governed a different transition would govern nothing.
func transitionOfDeal(t *testing.T, e *Env, deal ids.DealID) deals.TransitionRef {
	t.Helper()
	var ref deals.TransitionRef
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT pipeline_id, from_stage_id, to_stage_id
		  FROM stage_progression_outcome
		 WHERE deal_id = $1 AND outcome = 'proposed'
		 ORDER BY created_at DESC LIMIT 1`, deal).
		Scan(&ref.PipelineID, &ref.FromStageID, &ref.ToStageID); err != nil {
		t.Fatalf("reading the proposed transition: %v", err)
	}
	return ref
}

// setAutopilotSwitch writes the installation kill switch.
func setAutopilotSwitch(t *testing.T, e *Env, on bool) {
	t.Helper()
	value := "false"
	if on {
		value = "true"
	}
	if _, err := e.Pool.Exec(t.Context(),
		`INSERT INTO setting (key, value) VALUES ($1, $2::jsonb)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		deals.StageAutopilotEnabled.Key(), value); err != nil {
		t.Fatalf("setting the kill switch: %v", err)
	}
}

// seedGoodRecord settles enough clean acceptances on this transition, spread
// over enough days, to clear the default bar.
//
// Written straight to the ledger. The alternative is staging and deciding two
// hundred real cards, which is what the transition's record IS — but the
// subject here is the applier, and a fixture that spent a minute proving the
// ledger writer again would test that instead.
func seedGoodRecord(t *testing.T, e *Env, deal ids.DealID, ref deals.TransitionRef) {
	t.Helper()
	if _, err := e.Pool.Exec(t.Context(), `
		INSERT INTO stage_progression_outcome (
			approval_id, deal_id, pipeline_id, from_stage_id, to_stage_id,
			outcome, decided_at)
		SELECT gen_random_uuid(), $1, $2, $3, $4, 'approved_clean',
		       now() - (CASE WHEN i = 0 THEN interval '29 days' ELSE interval '1 hour' END)
		  FROM generate_series(0, 239) AS i`,
		deal, ref.PipelineID, ref.FromStageID, ref.ToStageID); err != nil {
		t.Fatalf("seeding the transition's record: %v", err)
	}
}

// pendingCardFor answers the deal's open stage-progression card.
func pendingCardFor(t *testing.T, e *Env, deal ids.DealID) (ids.ApprovalID, bool) {
	t.Helper()
	var id ids.ApprovalID
	err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&id)
	if err != nil {
		return id, false
	}
	return id, true
}

// stageOf answers where the deal actually is.
func stageOf(t *testing.T, e *Env, deal ids.DealID) ids.StageID {
	t.Helper()
	var stage ids.StageID
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT stage_id FROM deal WHERE id = $1`, deal).Scan(&stage); err != nil {
		t.Fatalf("reading the deal's stage: %v", err)
	}
	return stage
}

// A governed transition with a clean record moves the deal itself, and the
// ledger says the product did it.
//
// The one test here that proves the feature works rather than that it
// withholds. It is also the positive control every refusal below depends on:
// if this fails, a refusal proves only that nothing was ever going to apply.
func TestAnAutoAppliedMoveIsDecidedBySystemAndLandsOnTheLedger(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	ref := governedTransition(t, e, deal)
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the auto-apply sweep failed: %v", err)
	}
	if applied == 0 {
		t.Fatal("the sweep applied nothing: a transition with a clean record over " +
			"the volume and observation bars did not move its deal, so every " +
			"refusal test in this file would pass for the wrong reason")
	}

	if got := stageOf(t, e, deal); got == before {
		t.Fatalf("the sweep reported an apply but the deal is still in %s", before)
	}
	if got := stageOf(t, e, deal); got != ref.ToStageID {
		t.Fatalf("the deal moved to %s, want the proposed %s", got, ref.ToStageID)
	}

	// The ledger is closed by the outcome consumer, fed the event the apply
	// actually wrote — the same path a human decision takes. That the consumer
	// is subscribed at all is held by TestEveryEventConsumerIsSubscribed; what
	// is under test here is that an AUTOMATIC apply reaches it carrying the
	// fact that nobody was asked.
	card, open := pendingCardFor(t, e, deal)
	if open {
		t.Fatal("the card is still pending after the sweep reported applying it")
	}
	deliverApprovalDecided(t, e, appliedCardFor(t, e, deal))
	_ = card

	// The ledger tells an automatic move from a human's clean approval. This
	// is what keeps the launch gate honest: counted as approved_clean, the
	// autopilot would be voting on whether it should be running.
	var outcome string
	var bySystem bool
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT outcome, decided_by_system FROM stage_progression_outcome
		 WHERE deal_id = $1 AND outcome <> 'approved_clean' ORDER BY decided_at DESC LIMIT 1`,
		deal).Scan(&outcome, &bySystem); err != nil {
		t.Fatalf("reading the applied move's ledger row: %v", err)
	}
	if outcome != deals.ProgressionAutoApplied {
		t.Fatalf("an automatic move reads as %q on the ledger, want %q — counted "+
			"as a human's approval it would dilute the very rate that authorized it",
			outcome, deals.ProgressionAutoApplied)
	}
	if !bySystem {
		t.Error("the ledger row does not say the system decided it, so the column " +
			"and the outcome disagree about the same move")
	}
}

// The installation kill switch stops every transition, however good its record.
func TestNothingAutoAppliesWhileTheKillSwitchIsOff(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	governedTransition(t, e, deal)
	setAutopilotSwitch(t, e, false)
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied != 0 {
		t.Fatalf("%d move(s) applied while the installation's kill switch was off", applied)
	}
	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("the deal moved to %s with the kill switch off", got)
	}
	if _, open := pendingCardFor(t, e, deal); !open {
		t.Error("the card is gone: the switch must take away the APPLY, not the ask")
	}
}

// A transition nobody configured never applies.
//
// The DEFAULT state of every transition in the product, and the one this must
// get right: a fresh installation moves no deal by itself.
func TestAnUngovernedTransitionNeverAutoApplies(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	// Everything else is true: the owner has the authority to make this move
	// by hand, the switch is on, the record is spotless. Only the RULE is
	// missing.
	//
	// The role matters as much as the record here. Without it the sweep
	// refuses for lack of permission, and this test passes against an applier
	// that ignores the governing rule entirely — which it did, until the grant
	// was added and the mutation check caught it.
	giveTheOwnerARealRole(t, e)
	setAutopilotSwitch(t, e, true)
	seedGoodRecord(t, e, deal, transitionOfDeal(t, e, deal))
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied != 0 {
		t.Fatalf("%d move(s) applied on a transition nobody configured", applied)
	}
	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("the deal moved to %s under no rule at all", got)
	}
}

// A suspended rule proposes and never applies.
func TestASuspendedRuleNeverAutoApplies(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	ref := governedTransition(t, e, deal)
	if err := e.Deals.SuspendForSafetyDefect(
		e.Admin(), ref, deals.DefectCrossTenant); err != nil {
		t.Fatalf("suspending: %v", err)
	}
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied != 0 {
		t.Fatalf("%d move(s) applied on a suspended rule", applied)
	}
	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("a suspended rule moved the deal to %s", got)
	}
	if _, open := pendingCardFor(t, e, deal); !open {
		t.Error("the suspended rule stopped asking: the moves an operator " +
			"suspended it to look at are exactly the ones that must stay visible")
	}
}

// Thresholds are read in the APPLY transaction, not at staging.
//
// The card here was staged while the record was good and the rule was on. The
// record then goes bad. Nothing re-stages, and the card is unchanged — so an
// applier that trusted what was true at staging would move the deal on numbers
// that have since failed.
func TestThresholdsAreReadInTheApplyTransactionNotAtStaging(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	governedTransition(t, e, deal)

	// The record goes bad AFTER the card was staged: enough of the settled
	// moves reversed to put the transition over its ceiling.
	if _, err := e.Pool.Exec(t.Context(), `
		UPDATE stage_progression_outcome
		   SET reversed_at = now()
		 WHERE deal_id = $1 AND outcome = 'approved_clean'
		   AND id IN (SELECT id FROM stage_progression_outcome
		               WHERE deal_id = $1 AND outcome = 'approved_clean' LIMIT 20)`,
		deal); err != nil {
		t.Fatalf("spoiling the record: %v", err)
	}
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied != 0 {
		t.Fatalf("%d move(s) applied on a record that failed after staging: the "+
			"thresholds were read when the card was raised, not when it applied", applied)
	}
	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("the deal moved to %s on a record that had already failed", got)
	}
}

// A card the product itself marked as needing judgement never applies.
//
// ConfirmFirst is a fact about THIS move — a closing stage, a skip, a criterion
// resting on something merely proposed — and no amount of history about the
// transition answers it. A rule good enough to apply must still not apply one.
func TestAConfirmFirstCardNeverAutoAppliesHoweverGoodTheRecord(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	governedTransition(t, e, deal)

	card, open := pendingCardFor(t, e, deal)
	if !open {
		t.Fatal("no card to mark")
	}
	// Mark the staged card the way the proposer marks a move that needs a
	// judgement, leaving everything else true.
	if _, err := e.Pool.Exec(t.Context(), `
		UPDATE approval
		   SET proposed_change = jsonb_set(proposed_change, '{confirm_first}', 'true')
		 WHERE id = $1`, card); err != nil {
		t.Fatalf("marking the card confirm-first: %v", err)
	}
	before := stageOf(t, e, deal)

	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied != 0 {
		t.Fatalf("%d move(s) applied on a card marked as needing judgement", applied)
	}
	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("a confirm-first move applied itself, landing the deal in %s", got)
	}
}

// An automatic move does not improve the record that authorized it.
//
// The autopilot cannot vote on whether it should be running. Its own applies
// are counted on their own line and excluded from the denominator every rate
// is taken over, so a transition running automatically reports the same clean
// acceptance rate as its volume grows.
func TestAnAutoAppliedMoveDoesNotCountTowardTheRateThatAuthorizedIt(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	ref := governedTransition(t, e, deal)

	before := reviewedCount(t, e, ref)
	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied == 0 {
		t.Fatal("nothing applied, so this proves nothing about what an automatic " +
			"apply does to the count")
	}
	// The ledger is only written when the consumer hears the decision. Without
	// this the count cannot move whatever the mapping does, and the assertion
	// below would hold against a consumer that filed the apply as a human's.
	deliverApprovalDecided(t, e, appliedCardFor(t, e, deal))
	after := reviewedCount(t, e, ref)

	if after != before {
		t.Fatalf("the reviewed count moved from %d to %d across an automatic "+
			"apply: the autopilot's own move entered the denominator of the rate "+
			"that decides whether it may keep running", before, after)
	}
}

// appliedCardFor answers the card the sweep decided.
func appliedCardFor(t *testing.T, e *Env, deal ids.DealID) ids.ApprovalID {
	t.Helper()
	var id ids.ApprovalID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'approved'
		 ORDER BY decided_at DESC LIMIT 1`,
		deals.StageProgressionKind, deal).Scan(&id); err != nil {
		t.Fatalf("reading the applied card: %v", err)
	}
	return id
}

// reviewedCount is how many proposals a CONTACT answered on this transition.
func reviewedCount(t *testing.T, e *Env, ref deals.TransitionRef) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM stage_progression_outcome
		 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3
		   AND outcome IN ('approved_clean', 'approved_edited', 'rejected')`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&n); err != nil {
		t.Fatalf("counting reviewed proposals: %v", err)
	}
	return n
}

// The rule is re-asked in the transaction that moves the deal.
//
// THE WINDOW IS THE POINT. The sweep asks whether a transition may apply, then
// decides — and between those two the rule can change: an admin flips the kill
// switch, sets the transition back to propose, or the product suspends it. An
// applier that trusted its earlier answer commits the move on a permission
// that no longer exists.
//
// So the suspension here lands AFTER the sweep's own check has passed and
// before the effect writes. Suspending before the sweep would prove nothing —
// the sweep's own check would refuse it, and the test would pass with this
// guard deleted. It did, until the mutation check caught it.
func TestAnAutomaticMoveIsRefusedWhenTheRuleChangesBeforeItLands(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	ref := governedTransition(t, e, deal)
	before := stageOf(t, e, deal)
	card, open := pendingCardFor(t, e, deal)
	if !open {
		t.Fatal("no card to apply")
	}

	// The sweep's question, asked and answered while the rule is still on.
	verdict, err := e.Deals.MayAutoApplyStageMove(
		e.Admin(), deal, ref.FromStageID, ref.ToStageID)
	if err != nil || verdict.Mode != deals.ModeAuto {
		t.Fatalf("the fixture does not reach auto (%s: %s, err=%v)",
			verdict.Mode, verdict.Why, err)
	}

	// The rule goes off HERE — after that answer, before the apply.
	if err := e.Deals.SuspendForSafetyDefect(
		e.Admin(), ref, deals.DefectAuthorization); err != nil {
		t.Fatalf("suspending: %v", err)
	}

	// Now apply as the sweep would, having already been told yes. This is the
	// call the applier makes; the only thing standing between it and a moved
	// deal is the re-check inside the effect's transaction.
	if err := applyAsTheSweepWould(t, e, card); err == nil {
		t.Fatal("the move applied on an answer taken before the rule went off: " +
			"the effect trusts what the sweep was told rather than what is true " +
			"when it writes")
	}

	if got := stageOf(t, e, deal); got != before {
		t.Fatalf("the deal moved to %s on a rule that had been suspended", got)
	}

	// The card is left APPROVED with its failure recorded, not pending. That
	// is this product's existing shape for any effect that refuses after its
	// decision commits — a human's approval behaves the same way — and the
	// mark is what keeps the row reachable: not pending, so the decision lane
	// skips it; carrying effect_failed_at, so the receipts lane does not.
	//
	// Asserted rather than wished away: a test demanding the card stay pending
	// would be describing a product that does not exist, and would fail for a
	// reason that has nothing to do with the guard above.
	var failure *string
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT effect_failure FROM approval WHERE id = $1`, card).Scan(&failure); err != nil {
		t.Fatalf("reading the card's effect failure: %v", err)
	}
	if failure == nil {
		t.Error("the refused move left no failure on the card, so nobody can see " +
			"that an approved automatic move never happened")
	}
}

// autoApplyActor is the id compose/autoapply.go binds for an automatic apply.
//
// Spelled here because the constant is unexported and exporting it for a test
// would widen the package's surface for nobody's benefit. The guard under test
// keys on this exact value, so a rename there fails this test — which is the
// coupling a reader should see rather than one hidden behind an accessor.
const autoApplyActor = "agent:auto-apply"

// applyAsTheSweepWould runs the apply under the same principal the auto-applier
// binds: an agent acting for the deal's owner, carrying that owner's authority.
//
// Built here rather than by calling the sweep, because the sweep would ask the
// policy again first and refuse before reaching the effect — which is the
// correct behaviour and the reason it cannot exercise this guard.
func applyAsTheSweepWould(t *testing.T, e *Env, card ids.ApprovalID) error {
	t.Helper()
	rbac, seat, err := identity.NewService(e.Pool).EffectiveAuthority(
		e.Admin(), e.WS, e.Rep1)
	if err != nil {
		t.Fatalf("resolving the owner's authority: %v", err)
	}
	ownerCtx := principal.WithActor(e.Admin(), principal.Principal{
		Type:        principal.PrincipalAgent,
		ID:          autoApplyActor,
		UserID:      e.Rep1,
		OnBehalfOf:  e.Rep1,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Scopes:      principal.ScopeSet{principal.ScopeWrite: struct{}{}},
		Permissions: rbac.Permissions,
	})
	// The service WITH the real effect registered. A bare approvals.NewService
	// has none, so ApplyUnderPolicy would approve the card and never run the
	// move — the test would then report the guard missing whatever the guard
	// does, because nothing it guards ever executes.
	_, err = compose.StageProgressionDecisions(e.Pool).ApplyUnderPolicy(ownerCtx, card)
	return err
}
