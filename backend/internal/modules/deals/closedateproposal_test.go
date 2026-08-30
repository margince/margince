// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// When two close-date corrections are the same question.
//
// This is the whole of the rejection memory's judgment, and both of its terms
// were arrived at by watching one fail: keying on a date remembers a refusal
// for a single night, because the sweep recomputes its guess against "today";
// dropping the second term ends close-date hygiene on a deal for good.

import (
	"testing"
	"time"
)

func onDate(t *testing.T, day string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, day)
	if err != nil {
		t.Fatalf("parsing %q: %v", day, err)
	}
	return &parsed
}

func TestWhenTwoCloseDateCorrectionsAreTheSameQuestion(t *testing.T) {
	t.Parallel()
	// Last night the sweep offered to move a three-stages-out deal off Aug 19
	// to Sep 13, wrote that date on provisionally, and the rep said no.
	held := "2026-08-19"
	refused := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-13", PreviousCloseDate: &held, RemainingOpenStages: "3",
		Asking: AskingIsThisDateRight,
	}
	stillThere := onDate(t, "2026-09-13")

	// Tonight's pass over the same untouched deal proposes a LATER date from the
	// same reasoning, and the rep has answered the reasoning already.
	tonight := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}
	if !ProbeFor(tonight, stillThere).SameQuestionAs(refused) {
		t.Error("the same judgment raised again reads as a new question; the rep is " +
			"asked what they already answered")
	}

	// The deal advanced: the guess is drawn from a genuinely different distance,
	// so it is worth asking again.
	nearer := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "1", Asking: AskingIsThisDateRight,
	}
	if ProbeFor(nearer, stillThere).SameQuestionAs(refused) {
		t.Error("a deal that advanced a stage is treated as already answered, so " +
			"nobody is told its date went stale")
	}

	// The rep set their own date, and it has gone stale in its turn. The deal no
	// longer stands where the refusal left it, so the refusal no longer
	// describes it — without this one "no" ends close-date hygiene for good.
	theirOwn := onDate(t, "2026-08-26")
	if ProbeFor(tonight, theirOwn).SameQuestionAs(refused) {
		t.Error("a date the rep set themselves is treated as the refused guess, so " +
			"they are never told it went stale")
	}

	// A deal holding no date at all is not standing where this refusal left it:
	// that one was about a deal holding Aug 19.
	if ProbeFor(tonight, nil).SameQuestionAs(refused) {
		t.Error("a deal with no close date matches a refusal about a deal that held one")
	}
}

// The gone-quiet arm does not re-date a deal whose date is still ahead of it, so
// such a deal ends the night exactly where it started. Recognising only the
// PROPOSED date would forget every refusal on that arm by morning — the rep
// would be asked whether the deal is still alive every single day.
func TestARefusalOnADealThatWasNotReDatedIsStillRemembered(t *testing.T) {
	t.Parallel()
	held := "2026-11-30"
	refused := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-13", PreviousCloseDate: &held, RemainingOpenStages: "3",
		Asking: AskingIsThisDateRight,
	}
	// Tonight's guess has moved, and the deal is still on its own future date.
	tonight := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}
	if !ProbeFor(tonight, onDate(t, held)).SameQuestionAs(refused) {
		t.Error("a deal the sweep did not re-date reads as a new question every night")
	}
	// And a rep who then moves it somewhere of their own is asked again.
	if ProbeFor(tonight, onDate(t, "2027-02-01")).SameQuestionAs(refused) {
		t.Error("a date the rep chose is treated as the one they refused")
	}
}

// Saying a date is fine is not saying the deal is alive.
//
// The two cards reach a rep as one approval kind, and they can meet at the same
// deal, the same stage count and the same date. Without this term a refusal of
// either buries the other — a rep who turned down a date correction would never
// be told the deal had since gone quiet.
func TestRefusingOneQuestionDoesNotAnswerTheOther(t *testing.T) {
	t.Parallel()
	held := "2026-08-19"
	refusedTheDate := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-13", PreviousCloseDate: &held,
		RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}
	stillThere := onDate(t, "2026-09-13")

	isItAlive := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDealAlive,
	}
	if ProbeFor(isItAlive, stillThere).SameQuestionAs(refusedTheDate) {
		t.Error("refusing a date correction silences the review asking whether the " +
			"deal is still alive")
	}
	// And the same the other way round.
	isTheDateRight := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}
	refusedAlive := refusedTheDate
	refusedAlive.Asking = AskingIsThisDealAlive
	if ProbeFor(isTheDateRight, stillThere).SameQuestionAs(refusedAlive) {
		t.Error("refusing a gone-quiet review silences the date correction")
	}
	// The two labels have to be tellable apart, or this test proves nothing.
	if AskingIsThisDateRight == AskingIsThisDealAlive {
		t.Fatal("both questions carry the same label")
	}
}

// A refusal about a deal holding NO date is remembered while it holds none, and
// forgotten once somebody gives it one. The sentinel is what makes the first
// half work: an absent key would match nothing at all.
func TestARefusalAboutADatelessDealIsRememberedWhileItStaysDateless(t *testing.T) {
	t.Parallel()
	refused := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-13", PreviousCloseDate: nil, RemainingOpenStages: "3",
		Asking: AskingIsThisDateRight,
	}
	tonight := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}
	if !ProbeFor(tonight, nil).SameQuestionAs(refused) {
		t.Error("a deal that still has no date is asked about again every night")
	}
	if ProbeFor(tonight, onDate(t, "2026-12-01")).SameQuestionAs(refused) {
		t.Error("a deal that has since been given a date is treated as still dateless")
	}
}

func TestAPayloadWithNoStageCountMatchesNothing(t *testing.T) {
	t.Parallel()
	// A correction staged before this key existed. Two unknowns must not read as
	// one, or the oldest refusal in the queue silences every deal that meets it.
	older := CloseDateCorrection{ExpectedCloseDate: "2026-09-13"}
	standing := onDate(t, "2026-09-13")
	live := CloseDateCorrection{ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3"}

	if ProbeFor(live, standing).SameQuestionAs(older) {
		t.Error("a live probe matches a payload that carries no stage count")
	}
	if ProbeFor(older, standing).SameQuestionAs(older) {
		t.Error("two payloads with no stage count match each other")
	}
}

// The probe compares TONIGHT's standing date against the EARLIER proposal, and
// this is the one relation in the memory that is easy to write the wrong way
// round. Comparing two standing dates, or two proposed dates, holds a refusal
// for exactly one night — which looks identical to no memory at all from the
// second night on.
func TestTheProbeComparesWhereTheDealIsAgainstWhatWasRefused(t *testing.T) {
	t.Parallel()
	held := "2026-08-19"
	refused := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-13", PreviousCloseDate: &held, RemainingOpenStages: "3",
		Asking: AskingIsThisDateRight,
	}
	tonight := CloseDateCorrection{
		ExpectedCloseDate: "2026-09-14", RemainingOpenStages: "3", Asking: AskingIsThisDateRight,
	}

	// Standing where the refusal put it: still the same question, even though
	// neither proposed date matches the other.
	if !ProbeFor(tonight, onDate(t, "2026-09-13")).SameQuestionAs(refused) {
		t.Error("the memory is comparing the two PROPOSED dates, which move nightly")
	}
	// Standing on tonight's own proposal would be the wrong comparison: nothing
	// has written that date yet when the probe runs.
	if ProbeFor(tonight, onDate(t, "2026-09-14")).SameQuestionAs(refused) {
		t.Error("the memory matches a deal standing on tonight's proposal, so it is " +
			"comparing the probe against itself")
	}
}

// The memory's key and the guessed date must clamp the stage count the same
// way. A deal in its last open stage can count zero or one remaining depending
// on how the pipeline is shaped, and both are offered the identical date — so
// both are one question, and two spellings of the clamp would forget a refusal
// the moment the count crossed between them.
func TestTheStageCountIsClampedTheSameWayTheDateIs(t *testing.T) {
	t.Parallel()
	if StagesRemaining(0) != StagesRemaining(1) {
		t.Errorf("zero stages keys as %q and one stage as %q, but both propose the "+
			"same date — a refusal of one is forgotten when the count reads the other",
			StagesRemaining(0), StagesRemaining(1))
	}
	if got := StagesRemaining(4); got != "4" {
		t.Errorf("four stages remaining keys as %q, want \"4\"", got)
	}
	if StagesToGo(0) != 1 {
		t.Errorf("a deal with no counted stages ahead is paced over %d stages, want 1",
			StagesToGo(0))
	}
}
