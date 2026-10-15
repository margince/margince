// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// What a transition has earned, and what each number is allowed to mean.
//
// These are arithmetic tests, and the arithmetic is the product: the rates
// here decide whether a transition may eventually move deals without asking.
// A denominator that quietly includes proposals nobody read, or a safety
// number that counts one bad move twice, is not a rounding difference — it is
// the launch gate answering a question nobody asked.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reportWindow is wide enough that nothing in these fixtures falls out of it,
// so a test that fails is failing on the arithmetic rather than the clock.
const reportWindow = 90 * 24 * time.Hour

// settle records a proposal and then answers it, which is what every fixture
// below is made of.
//
// Each call takes its OWN target stage id, because a second proposal for one
// target supersedes the first (RecordProgressionProposed closes it). That is
// right for the product and wrong for a fixture trying to build a population,
// so the tests hand out distinct targets and the report groups them.
func settle(
	t *testing.T, e *configEnv, dealID ids.DealID, fromID, toID ids.StageID, outcome string,
) ids.UUID {
	t.Helper()
	approvalID := ids.NewV7()
	proposeOutcome(t, e, dealID, fromID, toID, approvalID)
	if outcome == ProgressionProposed {
		return approvalID
	}
	if err := e.store.RecordProgressionDecided(
		e.asDealWriter(), approvalID, outcome, nil, false); err != nil {
		t.Fatalf("recording %s: %v", outcome, err)
	}
	return approvalID
}

// reportFor reads one pipeline's record.
func reportFor(t *testing.T, e *configEnv, fromID ids.StageID) []TransitionRates {
	t.Helper()
	got, err := e.store.ReadStageAutomationReport(
		e.as(), pipelineOf(t, e, fromID), reportWindow)
	if err != nil {
		t.Fatalf("reading the report: %v", err)
	}
	return got
}

// oneTransition answers the single transition a fixture built, and fails
// rather than returning a zero value: a report that came back empty would
// otherwise pass every "the rate is 0" assertion below.
func oneTransition(t *testing.T, got []TransitionRates) TransitionRates {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("the report carries %d transitions, want exactly 1 — an empty report "+
			"satisfies every rate assertion in this file by accident", len(got))
	}
	return got[0]
}

// A rate is counted per transition, and the same rows cut again by what the
// moves rested on. Two transitions in one pipeline do not pool.
func TestRatesAreCountedByTransitionAndEvidenceKind(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	other := extraStage(t, e, fromID, "Negotiation", 2)

	settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	settle(t, e, dealID, fromID, other, ProgressionRejected)

	got := reportFor(t, e, fromID)
	if len(got) != 2 {
		t.Fatalf("two targets produced %d transitions, want 2 — the report is pooling "+
			"moves that go to different stages", len(got))
	}
	byTarget := map[ids.StageID]TransitionRates{}
	for _, r := range got {
		byTarget[r.ToStageID] = r
	}
	if rate := byTarget[toID].CleanAcceptanceRate(); rate != 1 {
		t.Errorf("the accepted transition reads %.2f clean acceptance, want 1", rate)
	}
	if rate := byTarget[other].CleanAcceptanceRate(); rate != 0 {
		t.Errorf("the rejected transition reads %.2f clean acceptance, want 0", rate)
	}
	// The same rows, cut by the criterion kind they rested on.
	kinds := byTarget[toID].EvidenceKinds
	if len(kinds) != 1 || kinds[0].Kind != string(CriterionBuyerConfirmed) {
		t.Fatalf("the accepted transition's evidence cut is %+v, want one %s row",
			kinds, CriterionBuyerConfirmed)
	}
	if kinds[0].AcceptedClean != 1 || kinds[0].Reviewed != 1 {
		t.Errorf("the %s cut reads %d clean of %d reviewed, want 1 of 1",
			kinds[0].Kind, kinds[0].AcceptedClean, kinds[0].Reviewed)
	}
}

// An EDITED approval is not a clean one.
//
// The rate answers "is this proposal right as it stands", and a card a human
// had to fix before releasing is one they did not find right as it stood.
// Folding edits in would let a transition that is always nearly right read as
// one that is always right, which is the difference the launch gate turns on.
func TestCleanAcceptanceExcludesEditedApprovals(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	third := extraStage(t, e, fromID, "Negotiation", 2)
	fourth := extraStage(t, e, fromID, "Contract", 3)

	settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	settle(t, e, dealID, fromID, third, ProgressionApprovedEdited)
	settle(t, e, dealID, fromID, fourth, ProgressionApprovedEdited)

	var clean, edited, reviewed int
	for _, r := range reportFor(t, e, fromID) {
		clean += r.AcceptedClean
		edited += r.AcceptedEdited
		reviewed += r.Reviewed
	}
	if clean != 1 || edited != 2 || reviewed != 3 {
		t.Fatalf("counted %d clean / %d edited of %d reviewed, want 1 / 2 of 3",
			clean, edited, reviewed)
	}
	// And on the transition that was accepted cleanly, the rate is whole.
	for _, r := range reportFor(t, e, fromID) {
		if r.ToStageID == toID && r.CleanAcceptanceRate() != 1 {
			t.Errorf("the cleanly accepted transition reads %.2f, want 1",
				r.CleanAcceptanceRate())
		}
		if r.ToStageID == third && r.CleanAcceptanceRate() != 0 {
			t.Errorf("an EDITED approval counted toward clean acceptance: %.2f, want 0",
				r.CleanAcceptanceRate())
		}
	}
}

// One bad move counts ONCE, however many ways it went wrong.
//
// A move that was reversed AND whose evidence a human corrected is a single
// "this should not have happened". Adding a reversal rate to a correction rate
// would report it as two, and push a transition past a ceiling it had not
// actually crossed — which is the direction that matters, because this number
// is what STOPS automation rather than what earns it.
func TestCorrectionAndReversalShareTheSafetyNumeratorWithoutDoubleCountingOneOutcome(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	third := extraStage(t, e, fromID, "Negotiation", 2)
	fourth := extraStage(t, e, fromID, "Contract", 3)
	fifth := extraStage(t, e, fromID, "Signed", 4)

	// One move that went wrong BOTH ways.
	both := settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	markUnsafe(t, e, both, unsafeWays{Reversed: true, Corrected: true})
	// One reversed only, one corrected only, one clean.
	reversedOnly := settle(t, e, dealID, fromID, third, ProgressionApprovedClean)
	markUnsafe(t, e, reversedOnly, unsafeWays{Reversed: true})
	correctedOnly := settle(t, e, dealID, fromID, fourth, ProgressionApprovedClean)
	markUnsafe(t, e, correctedOnly, unsafeWays{Corrected: true})
	settle(t, e, dealID, fromID, fifth, ProgressionApprovedClean)

	var unsafe, reviewed int
	for _, r := range reportFor(t, e, fromID) {
		unsafe += r.Unsafe
		reviewed += r.Reviewed
	}
	if reviewed != 4 {
		t.Fatalf("counted %d reviewed, want 4", reviewed)
	}
	if unsafe != 3 {
		t.Fatalf("counted %d unsafe of 4 reviewed, want 3 — the move that was both "+
			"reversed and corrected is ONE mistake and must not count twice", unsafe)
	}
}

// The window closing is not a rejection.
//
// Both are refusals in the approvals module's vocabulary, and the product's
// answer to each is opposite: a rejected transition needs a better proposal, an
// expired one needs somebody to look. Counting expiry as rejection would make a
// transition nobody has time to read indistinguishable from one contacts actively
// disagree with — and neither belongs in the reviewed denominator, because
// nobody answered.
func TestAnExpiredProposalCountsAsExpiredNotRejected(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	third := extraStage(t, e, fromID, "Negotiation", 2)

	settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	settle(t, e, dealID, fromID, third, ProgressionExpired)

	var expired, rejected, reviewed int
	for _, r := range reportFor(t, e, fromID) {
		expired += r.Expired
		rejected += r.Rejected
		reviewed += r.Reviewed
	}
	if expired != 1 {
		t.Errorf("counted %d expired, want 1", expired)
	}
	if rejected != 0 {
		t.Errorf("counted %d rejected, want 0 — an expiry is a refusal by NOBODY, and "+
			"reading it as disagreement sends the reader to fix the wrong thing", rejected)
	}
	if reviewed != 1 {
		t.Fatalf("counted %d reviewed, want 1 — a proposal nobody answered is not "+
			"evidence that anyone agreed with it, so it cannot sit in the denominator",
			reviewed)
	}
	// And the surviving rate is whole rather than halved by the unanswered one.
	for _, r := range reportFor(t, e, fromID) {
		if r.ToStageID == toID && r.CleanAcceptanceRate() != 1 {
			t.Errorf("clean acceptance reads %.2f, want 1 — the expired proposal is in "+
				"the denominator", r.CleanAcceptanceRate())
		}
	}
}

// A proposal still standing is counted, and counted apart.
//
// Reported because its absence would make the reviewed count look like the
// whole story: a transition whose cards are mostly ignored has an acceptance
// rate computed over the few somebody happened to open, and a reader needs to
// see that before trusting the rate.
func TestAnOpenProposalIsReportedButNotReviewed(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	third := extraStage(t, e, fromID, "Negotiation", 2)

	settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	settle(t, e, dealID, fromID, third, ProgressionProposed)

	var open, reviewed int
	for _, r := range reportFor(t, e, fromID) {
		open += r.Proposed
		reviewed += r.Reviewed
	}
	if open != 1 {
		t.Errorf("counted %d still open, want 1", open)
	}
	if reviewed != 1 {
		t.Errorf("counted %d reviewed, want 1 — an unanswered card is not an answer", reviewed)
	}
}

// A transition with nothing answered has NO rate, and the zero it reports must
// not be read as one.
//
// Reviewed is the guard: a caller that showed 0% here would be saying contacts
// refuse this move every time, when nobody has looked at it once.
func TestATransitionNobodyAnsweredReportsZeroReviewed(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	settle(t, e, dealID, fromID, toID, ProgressionProposed)

	r := oneTransition(t, reportFor(t, e, fromID))
	if r.Reviewed != 0 {
		t.Fatalf("an unanswered proposal counted as %d reviewed, want 0", r.Reviewed)
	}
	if r.Proposed != 1 {
		t.Fatalf("the open card is reported as %d, want 1 — its absence would leave the "+
			"zero rate looking like disagreement rather than silence", r.Proposed)
	}
	// The rate is zero because the DENOMINATOR is zero, and the caller is told
	// so by Reviewed rather than by the rate.
	if r.CleanAcceptanceRate() != 0 {
		t.Fatalf("clean acceptance over no answers reads %.2f, want 0",
			r.CleanAcceptanceRate())
	}
}

// Observation days measure the span actually WATCHED, not the window asked for.
//
// A transition with a fine acceptance rate earned entirely on one afternoon has
// not been observed, however many proposals it saw — nobody has had time to
// notice a problem. Rounded DOWN, so twenty-seven days and twenty-three hours
// does not clear a twenty-eight day bar by an hour.
func TestObservationDaysMeasureTheSpanWatchedNotTheWindow(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	third := extraStage(t, e, fromID, "Negotiation", 2)

	first := settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	second := settle(t, e, dealID, fromID, third, ProgressionApprovedClean)
	// Ten days and twenty-three hours apart: a bar of eleven must not clear.
	backdateDecision(t, e, first, 10*24*time.Hour+23*time.Hour)

	var days int
	for _, r := range reportFor(t, e, fromID) {
		if r.ObservationDays > days {
			days = r.ObservationDays
		}
	}
	_ = second
	if days != 0 {
		// Each transition here holds ONE decision, so its own span is zero.
		t.Fatalf("a transition with a single decision reports %d observation days, want 0", days)
	}
	// Both decisions on ONE transition is where a span exists.
	fourth := extraStage(t, e, fromID, "Contract", 3)
	a := settle(t, e, dealID, fromID, fourth, ProgressionApprovedClean)
	backdateDecision(t, e, a, 10*24*time.Hour+23*time.Hour)
	b := settle(t, e, dealID, fromID, fourth, ProgressionApprovedClean)
	_ = b
	for _, r := range reportFor(t, e, fromID) {
		if r.ToStageID != fourth {
			continue
		}
		if r.ObservationDays != 10 {
			t.Fatalf("ten days and twenty-three hours reads as %d observation days, want 10 "+
				"— rounding up would clear an eleven-day bar by an hour", r.ObservationDays)
		}
	}
}

// A move recorded as reversed is unsafe even with no timestamp against it.
//
// reversed_at is nullable, and the table's constraint only forbids a CONTACT
// with no instant — so a row standing at `reversed` with a null timestamp is
// legal. Reading only the timestamp would count that undone move as a safe
// one, and under-counting is the single direction this number must never fail
// in: it is the ceiling that STOPS automation, so a miss reads as permission.
func TestAReversedOutcomeIsUnsafeWithoutATimestamp(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	approvalID := settle(t, e, dealID, fromID, toID, ProgressionReversed)

	// Deliberately no reversed_at: the outcome alone is the record.
	var stamped *time.Time
	if err := e.owner.QueryRow(t.Context(),
		`SELECT reversed_at FROM stage_progression_outcome WHERE approval_id = $1`,
		approvalID).Scan(&stamped); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if stamped != nil {
		t.Fatalf("the fixture carries a reversed_at (%v), so it does not test the "+
			"timestamp-less case it is named for", stamped)
	}

	r := oneTransition(t, reportFor(t, e, fromID))
	if r.Unsafe != 1 {
		t.Fatalf("a reversed move with no timestamp counted as %d unsafe, want 1 — "+
			"under-counting here reads as permission to automate", r.Unsafe)
	}
}

// The autopilot does not vote on whether it should be running.
//
// An `auto_applied` move is the automation agreeing with itself. In the
// denominator it dilutes every rate the launch gate reads, so a transition
// already running on automatic would report a FALLING clean-acceptance rate as
// its own volume grew — and the gate would read that as contacts losing
// confidence, when every contact who looked agreed.
func TestAnAutomaticMoveDoesNotDiluteTheHumanRates(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)

	// One human approval, then three the system applied on the same transition.
	settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)
	for range 3 {
		auto := ids.NewV7()
		proposeOutcome(t, e, dealID, fromID, toID, auto)
		if err := e.store.RecordProgressionDecided(
			e.asDealWriter(), auto, ProgressionAutoApplied, nil, false); err != nil {
			t.Fatalf("recording the automatic move: %v", err)
		}
	}

	r := oneTransition(t, reportFor(t, e, fromID))
	if r.AutoApplied != 3 {
		t.Fatalf("counted %d automatic moves, want 3", r.AutoApplied)
	}
	if r.Reviewed != 1 {
		t.Fatalf("counted %d reviewed, want 1 — an automatic move is the autopilot "+
			"agreeing with itself and cannot sit in the denominator that decides "+
			"whether it may keep running", r.Reviewed)
	}
	if rate := r.CleanAcceptanceRate(); rate != 1 {
		t.Fatalf("clean acceptance reads %.2f, want 1 — every CONTACT who looked agreed, "+
			"and the autopilot's own volume must not read as them losing confidence", rate)
	}
}

// A decision made today is in today's window, however old the card was.
//
// The window is about DECISIONS. A transition whose cards sit for weeks before
// anybody answers — which is exactly the slow, high-value transition an
// installation most wants measured — would otherwise lose its most recent
// answers to a filter on when the card was raised.
func TestADecisionIsInTheWindowEvenWhenItsCardIsOlderThanIt(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	approvalID := settle(t, e, dealID, fromID, toID, ProgressionApprovedClean)

	// The card was raised well outside a 30-day window; the answer came today.
	backdateCreation(t, e, approvalID, 60*24*time.Hour)

	got, err := e.store.ReadStageAutomationReport(
		e.as(), pipelineOf(t, e, fromID), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("reading the report: %v", err)
	}
	r := oneTransition(t, got)
	if r.Reviewed != 1 {
		t.Fatalf("a decision made TODAY on a card raised 60 days ago counted as %d "+
			"reviewed, want 1 — windowing on the card's age drops the answers a "+
			"slow transition is measured by", r.Reviewed)
	}
}

// backdateCreation ages one row's created_at without touching when it was
// answered, so a test can build the card-older-than-its-decision case.
func backdateCreation(t *testing.T, e *configEnv, approvalID ids.UUID, ago time.Duration) {
	t.Helper()
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(),
			`UPDATE stage_progression_outcome SET created_at = now() - $2::interval
			  WHERE approval_id = $1`, approvalID, ago.String())
		return err
	}); err != nil {
		t.Fatalf("backdating the proposal: %v", err)
	}
}

// pipelineOf answers the pipeline a stage belongs to.
func pipelineOf(t *testing.T, e *configEnv, stageID ids.StageID) ids.PipelineID {
	t.Helper()
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(),
		`SELECT pipeline_id FROM stage WHERE id = $1`, stageID).Scan(&pipelineID); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	return pipelineID
}

// extraStage adds one more open stage to the fixture's pipeline, so a test can
// build several distinct transitions out of one deal.
func extraStage(t *testing.T, e *configEnv, sibling ids.StageID, name string, pos int) ids.StageID {
	t.Helper()
	var pipelineID ids.PipelineID
	if err := e.owner.QueryRow(t.Context(),
		`SELECT pipeline_id FROM stage WHERE id = $1`, sibling).Scan(&pipelineID); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	open := 0
	stage, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: name, Position: pos,
		Semantic: string(SemanticOpen), WinProbability: &open,
	})
	if err != nil {
		t.Fatalf("seeding the stage %q: %v", name, err)
	}
	return ids.From[ids.StageKind](ids.UUID(stage.Id))
}

// unsafeWays says which way a settled move went wrong, so a call site reads as
// what happened rather than as two anonymous booleans.
type unsafeWays struct {
	Reversed  bool
	Corrected bool
}

// markUnsafe records that a settled move was undone, or rested on evidence a
// human went back and marked wrong, or both.
//
// Written straight to the columns because the writers that set them are WP6's
// (the revert endpoint) and the evidence correction path's. What is under test
// here is the arithmetic over those columns, and a fixture that waited for
// both writers would test nothing until both shipped.
func markUnsafe(t *testing.T, e *configEnv, approvalID ids.UUID, ways unsafeWays) {
	t.Helper()
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `
			UPDATE stage_progression_outcome
			   SET reversed_at = CASE WHEN $2 THEN now() ELSE reversed_at END,
			       evidence_corrected = $3
			 WHERE approval_id = $1`, approvalID, ways.Reversed, ways.Corrected)
		return err
	}); err != nil {
		t.Fatalf("marking the move unsafe: %v", err)
	}
}

// backdateDecision moves one settled row's decided_at into the past, so a test
// can build an observation span without waiting for one.
func backdateDecision(t *testing.T, e *configEnv, approvalID ids.UUID, ago time.Duration) {
	t.Helper()
	if err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(),
			`UPDATE stage_progression_outcome SET decided_at = now() - $2::interval
			  WHERE approval_id = $1`, approvalID, ago.String())
		return err
	}); err != nil {
		t.Fatalf("backdating the decision: %v", err)
	}
}
