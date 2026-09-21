// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// What a transition must clear before it moves a deal by itself.
//
// Every test here is about a REFUSAL, and that asymmetry is the design: this
// engine's failure mode is not "automation did not happen when it should" —
// somebody notices a card in their inbox. It is "automation happened when it
// should not", which nobody notices until a deal is in the wrong stage. So the
// bar is asked four ways and each one is proven to withhold on its own.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A record good enough to clear every default threshold, so a test that wants
// to prove ONE condition withholds can move that one and leave the rest true.
const (
	passingReviewed = 240
	// INSIDE the default 30-day window, deliberately. The observation span is
	// measured over rows the window admits, so a decision backdated PAST it is
	// not an older record — it is an absent one, and the span collapses to
	// zero. 29 clears the 28-day bar with a day to spare.
	passingSpanDays = 29
)

// autopilotOn writes the installation kill switch.
//
// Straight to the `setting` row rather than through settings.Store, because
// this env has no store — and the row IS the shape the reader reads
// (settings.ApplyTx selects value by key), so what is under test still sees
// what production would.
func autopilotOn(t *testing.T, e *configEnv, on bool) {
	t.Helper()
	value := "false"
	if on {
		value = "true"
	}
	if _, err := e.owner.Exec(t.Context(),
		`INSERT INTO setting (key, value) VALUES ($1, $2::jsonb)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		StageAutopilotEnabled.Key(), value); err != nil {
		t.Fatalf("setting the kill switch: %v", err)
	}
}

// ruleOn puts a transition on auto with the product's default thresholds.
func ruleOn(t *testing.T, e *configEnv, ref TransitionRef) TransitionPolicy {
	t.Helper()
	saved, err := e.store.SetTransitionPolicy(e.as(), SetTransitionPolicyInput{
		TransitionRef: ref, Mode: ModeAuto,
	})
	if err != nil {
		t.Fatalf("enabling the transition: %v", err)
	}
	return saved
}

// aGoodRecord settles enough clean acceptances, spread over enough days, to
// clear the default bar.
func aGoodRecord(t *testing.T, e *configEnv, dealID ids.DealID, ref TransitionRef) {
	t.Helper()
	for i := range passingReviewed {
		id := ids.NewV7()
		proposeOutcome(t, e, dealID, ref.FromStageID, ref.ToStageID, id)
		if err := e.store.RecordProgressionDecided(
			e.asDealWriter(), id, ProgressionApprovedClean, nil, false); err != nil {
			t.Fatalf("settling proposal %d: %v", i, err)
		}
		if i == 0 {
			// One decision pushed back, so the observation SPAN is real rather
			// than a burst: the bar asks how long contacts have had to notice a
			// problem, not how many they answered in an afternoon.
			backdateDecision(t, e, id, passingSpanDays*24*time.Hour)
		}
	}
}

// verdictFor asks the engine, in the transaction shape production uses.
func verdictFor(t *testing.T, e *configEnv, ref TransitionRef) AutopilotVerdict {
	t.Helper()
	var got AutopilotVerdict
	if err := e.store.Tx(e.as(), func(tx pgx.Tx) error {
		var err error
		got, err = StageAutopilotModeTx(e.as(), tx, ref, time.Now())
		return err
	}); err != nil {
		t.Fatalf("asking the autopilot: %v", err)
	}
	return got
}

// transitionOf names the fixture's own transition.
func transitionOf(t *testing.T, e *configEnv, from, to ids.StageID) TransitionRef {
	t.Helper()
	return TransitionRef{
		PipelineID: pipelineOf(t, e, from), FromStageID: from, ToStageID: to,
	}
}

// The kill switch answers before anything else, and answers for everything.
//
// It is the control an admin reaches for when something is wrong and they do
// not yet know which transition caused it. A switch that only applied to
// transitions whose rules were also off would be no switch at all.
func TestNothingAutoAppliesWhileTheKillSwitchIsOff(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)

	// Everything else says yes: the rule is on and the record is spotless.
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)
	aGoodRecord(t, e, dealID, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the fixture does not reach auto on its own (%s: %s) — every "+
			"refusal below would then pass for the wrong reason", got.Mode, got.Why)
	}

	autopilotOn(t, e, false)
	got := verdictFor(t, e, ref)
	if got.Mode != ModePropose {
		t.Fatal("a transition applied automatically while the installation's kill " +
			"switch was off")
	}
	if got.Why == "" {
		t.Error("the refusal names no reason, so an admin sees cards arriving and " +
			"nothing saying which control stopped the automation")
	}
}

// Volume and time are separate bars, and each withholds alone.
//
// They answer different questions: enough proposals says the rate is measured
// rather than a coincidence; enough days says contacts have had a chance to
// notice a problem. A transition that cleared one and not the other has not
// earned anything.
func TestAutoApplyNeedsTwoHundredReviewsAndTwentyEightObservationDays(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)

	// Volume alone, with no span: every decision lands in this instant.
	for range passingReviewed {
		id := ids.NewV7()
		proposeOutcome(t, e, dealID, fromID, toID, id)
		if err := e.store.RecordProgressionDecided(
			e.asDealWriter(), id, ProgressionApprovedClean, nil, false); err != nil {
			t.Fatalf("settling: %v", err)
		}
	}
	ruleOn(t, e, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModePropose {
		t.Fatal("240 acceptances earned in one instant enabled automation — the " +
			"observation window is not being asked")
	}

	// Now the span, and it clears.
	var first ids.UUID
	if err := e.owner.QueryRow(t.Context(), `
		SELECT approval_id FROM stage_progression_outcome
		 WHERE from_stage_id = $1 AND to_stage_id = $2
		 ORDER BY decided_at LIMIT 1`, fromID, toID).Scan(&first); err != nil {
		t.Fatalf("finding the earliest decision: %v", err)
	}
	backdateDecision(t, e, first, passingSpanDays*24*time.Hour)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("volume and span both hold and automation is still withheld: %s", got.Why)
	}
}

// The two RATE bars, each proven to withhold on its own.
//
// One is a floor and one is a ceiling, and they fail in opposite directions —
// which is why a single "the numbers are fine" check would let one of them
// through.
func TestAutoApplyNeedsNinetyFivePercentCleanAcceptanceAndUnderOnePercentCorrectionOrReversal(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	aGoodRecord(t, e, dealID, ref)
	ruleOn(t, e, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the clean fixture does not reach auto (%s), so neither case below "+
			"proves anything", got.Why)
	}

	// A run of rejections drags acceptance under the floor: 24 refusals in 264
	// answered is 91% clean, below the 95% the rule asks for.
	for range 24 {
		id := ids.NewV7()
		proposeOutcome(t, e, dealID, fromID, toID, id)
		if err := e.store.RecordProgressionDecided(
			e.asDealWriter(), id, ProgressionRejected, nil, false); err != nil {
			t.Fatalf("settling a rejection: %v", err)
		}
	}
	if got := verdictFor(t, e, ref); got.Mode != ModePropose {
		t.Fatal("automation stayed on with clean acceptance below the floor")
	}

	// Back over the floor, then break the SAFETY ceiling instead: three undone
	// moves in ~264 is above 1%.
	e2 := setupConfigEnv(t)
	dealID2, from2, to2 := twoStagePipeline(t, e2)
	ref2 := transitionOf(t, e2, from2, to2)
	autopilotOn(t, e2, true)
	aGoodRecord(t, e2, dealID2, ref2)
	ruleOn(t, e2, ref2)
	var undone []ids.UUID
	rows, err := e2.owner.Query(t.Context(), `
		SELECT approval_id FROM stage_progression_outcome
		 WHERE from_stage_id = $1 AND to_stage_id = $2 LIMIT 3`, from2, to2)
	if err != nil {
		t.Fatalf("finding moves to undo: %v", err)
	}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		undone = append(undone, id)
	}
	rows.Close()
	for _, id := range undone {
		markUnsafe(t, e2, id, unsafeWays{Reversed: true})
	}
	got := verdictFor(t, e2, ref2)
	if got.Mode != ModePropose {
		t.Fatal("automation stayed on with the undone-or-corrected rate above its ceiling")
	}
	if got.Why == "" {
		t.Error("the safety refusal names no reason")
	}
}

// A suspended rule still PROPOSES. It does not go silent, and it does not lose
// what the admin asked for.
//
// Both halves matter. Going silent would take the transition's cards away at
// the moment its record most needs watching; forgetting the admin's mode would
// make them re-decide a question they already answered, on evidence that has
// not changed.
func TestASuspendedRuleStillProposesAndKeepsWhatTheAdminAsked(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	aGoodRecord(t, e, dealID, ref)
	ruleOn(t, e, ref)

	if err := e.store.SuspendTransitionPolicy(e.as(), ref,
		"reversals crossed the ceiling"); err != nil {
		t.Fatalf("suspending: %v", err)
	}
	got := verdictFor(t, e, ref)
	if got.Mode != ModePropose {
		t.Fatal("a suspended rule kept applying moves automatically")
	}
	if got.Rule == nil || got.Rule.Mode != ModeAuto {
		t.Fatal("the suspension overwrote the admin's own setting — when it lifts " +
			"they would find automation off and nothing saying who turned it off")
	}
	if got.Why == "" || got.Rule.SuspendedReason == nil {
		t.Error("a suspension with no reason tells an admin their automation stopped " +
			"and nothing about why")
	}

	// And resuming brings it back, because the record beneath it is still good.
	if _, err := e.store.ResumeTransitionPolicy(e.as(), ref); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("a resumed rule on a good record did not come back: %s", got.Why)
	}
}

// An unconfigured transition proposes. This is the DEFAULT STATE of every
// transition in the product, and getting it wrong would auto-apply everything.
func TestATransitionWithNoRuleProposes(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	aGoodRecord(t, e, dealID, ref)

	got := verdictFor(t, e, ref)
	if got.Mode != ModePropose {
		t.Fatal("a transition nobody configured applied moves automatically — an " +
			"unconfigured stage is the default state of every stage in the product")
	}
	if got.Rule != nil {
		t.Error("a verdict carried a rule for a transition that has none")
	}
}

// Enabling names who did it, and keeps naming them.
//
// Who first trusted a transition is what an auditor asks for, and it is a
// different fact from who last nudged a threshold. An upsert that restamped it
// would answer the second question to somebody asking the first.
func TestTheFirstEnablingIsTheOneRecorded(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)

	first := ruleOn(t, e, ref)
	if first.EnabledBy == nil || first.EnabledAt == nil {
		t.Fatal("turning a transition on recorded nobody")
	}
	// A later save that only moves a threshold.
	window := 45
	again, err := e.store.SetTransitionPolicy(e.as(), SetTransitionPolicyInput{
		TransitionRef: ref, Mode: ModeAuto, WindowDays: &window,
	})
	if err != nil {
		t.Fatalf("adjusting the window: %v", err)
	}
	if again.EnabledAt == nil || !again.EnabledAt.Equal(*first.EnabledAt) {
		t.Fatal("adjusting a threshold restamped who enabled the transition")
	}
	if again.WindowDays != window {
		t.Fatalf("the window reads %d, want %d", again.WindowDays, window)
	}
	// And the thresholds nobody named are untouched.
	if again.MinReviewed != first.MinReviewed {
		t.Fatalf("an unnamed threshold moved from %d to %d — a caller changing one "+
			"value silently reset another", first.MinReviewed, again.MinReviewed)
	}
}

// A rule written straight to `propose` must not carry an enabler.
//
// The INSERT passed the actor unconditionally while stamping enabled_at only
// for auto, so a propose row would have named somebody with no instant and
// failed the enabling_is_timed CHECK. It did not fire only because the caller
// happened to pass nil — SQL that is right by accident is one refactor from
// being wrong.
func TestAProposeRuleCarriesNoEnabler(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)

	saved, err := e.store.SetTransitionPolicy(e.as(), SetTransitionPolicyInput{
		TransitionRef: ref, Mode: ModePropose,
	})
	if err != nil {
		t.Fatalf("writing a propose rule: %v", err)
	}
	if saved.EnabledBy != nil || saved.EnabledAt != nil {
		t.Fatalf("a rule set to propose named an enabler (%v at %v)",
			saved.EnabledBy, saved.EnabledAt)
	}

	// And the SQL says so on its own terms, not because the caller happened to
	// pass nil. Written straight to the statement with a real actor and
	// mode=propose: without the mode condition on enabled_by this inserts a
	// contact with no instant and the enabling_is_timed CHECK refuses it, which
	// is the failure a caller refactor would otherwise discover in production.
	_, direct := e.owner.Exec(t.Context(), `
		INSERT INTO stage_progression_policy (
			pipeline_id, from_stage_id, to_stage_id, mode, enabled_by, enabled_at)
		VALUES ($1, $2, $3, 'propose',
			CASE WHEN 'propose' = 'auto' THEN $4::uuid END,
			CASE WHEN 'propose' = 'auto' THEN now() END)
		ON CONFLICT (pipeline_id, from_stage_id, to_stage_id) DO NOTHING`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID, e.admin)
	if direct != nil {
		t.Fatalf("the propose-row insert is refused by the table: %v", direct)
	}
	// The control: the UNGUARDED shape — enabled_by passed straight through
	// while enabled_at stays conditional — must be refused. Without this the
	// assertion above would pass against a table that admits anything, and the
	// mode condition it is defending would be untested.
	_, unguarded := e.owner.Exec(t.Context(), `
		INSERT INTO stage_progression_policy (
			pipeline_id, from_stage_id, to_stage_id, mode, enabled_by, enabled_at)
		VALUES ($1, $2, $3, 'propose', $4,
			CASE WHEN 'propose' = 'auto' THEN now() END)`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID, e.admin)
	if unguarded == nil {
		t.Fatal("the table admitted an enabler with no enabling instant, so the " +
			"mode condition on enabled_by is defending nothing")
	}
}

// Deleting the contact who enabled a rule must not fail on a CHECK.
//
// enabled_by is ON DELETE SET NULL, and a paired-nullability CHECK would turn
// that declared SET NULL into a constraint violation that refuses the delete —
// which is exactly the bug an earlier table in this feature shipped. The
// constraint here is one-directional (enabled_by IS NULL OR enabled_at IS NOT
// NULL) so the erasure satisfies it, and the instant survives as the fact the
// audit trail still names a contact for.
func TestErasingTheEnablerDoesNotFightTheCheck(t *testing.T) {
	e := setupConfigEnv(t)
	_, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	saved := ruleOn(t, e, ref)
	if saved.EnabledBy == nil {
		t.Fatal("the fixture recorded no enabler, so this proves nothing")
	}

	if _, err := e.owner.Exec(t.Context(),
		`DELETE FROM app_user WHERE id = $1`, *saved.EnabledBy); err != nil {
		t.Fatalf("deleting the enabler was refused — the CHECK is fighting the "+
			"declared ON DELETE SET NULL: %v", err)
	}
	var by *string
	var at *string
	if err := e.owner.QueryRow(t.Context(),
		`SELECT enabled_by::text, enabled_at::text FROM stage_progression_policy
		  WHERE id = $1`, saved.ID).Scan(&by, &at); err != nil {
		t.Fatalf("reading the rule back: %v", err)
	}
	if by != nil {
		t.Error("the erased contact is still named on the rule")
	}
	if at == nil {
		t.Error("the enabling instant went with them — when it happened is still " +
			"true, and the audit trail is what still names who")
	}
}

// A transition with NO measured record clears no threshold.
//
// transitionRatesTx answers a zero TransitionRates for a transition nobody has
// proposed on, and 0/0 is 0 — which is under any CEILING. If the safety
// ceiling were asked first, a rule turned on before anything was proposed would
// apply immediately on an empty record. The volume bar is what catches it, and
// this holds that ordering.
func TestATransitionWithNoRecordClearsNoThreshold(t *testing.T) {
	rule := TransitionPolicy{
		CleanAcceptanceThreshold:    0.95,
		CorrectionReversalThreshold: 0.01,
		MinReviewed:                 200,
		MinObservationDays:          28,
		WindowDays:                  30,
	}
	if why := rule.withholds(TransitionRates{}); why == "" {
		t.Fatal("a transition with no measured record cleared every threshold — a " +
			"rule turned on before anything was proposed would apply at once")
	}
	// The positive control: a spotless record is NOT withheld, so the assertion
	// above is about the empty record rather than about withholds refusing
	// everything.
	spotless := TransitionRates{Reviewed: 300, AcceptedClean: 300, ObservationDays: 40}
	if why := rule.withholds(spotless); why != "" {
		t.Fatalf("a spotless record was withheld (%s), so the refusal above proves "+
			"nothing about the empty one", why)
	}
}

// Every error path answers PROPOSE by name, not a zero value.
//
// The zero AutopilotVerdict's Mode is "", which is safe against a caller asking
// `== ModeAuto` and not against one asking `== ModePropose` — it would match
// neither branch and fall through. A judgement whose safety depends on how the
// caller spells the question is one waiting for a caller who spells it the
// other way.
func TestARefusedVerdictNamesProposeRatherThanNothing(t *testing.T) {
	got := refused()
	if got.Mode != ModePropose {
		t.Fatalf("a refused verdict reads %q, want %q — a caller testing for propose "+
			"falls through to neither branch", got.Mode, ModePropose)
	}
}
