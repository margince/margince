// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noopRecordFailure is the failure-notice port for every test above that is
// not itself testing the notice — it must never be nil, since a real worker
// never hands stageReviewsWith one.
func noopRecordFailure(context.Context, ids.UUID, int) error { return nil }

// The insert carries the declared constant, and that constant is itself a
// sane bound — an unset MaxAttempts is silently River's own default with no
// other symptom, which is what the second check would catch even if the
// first held by coincidence.
func TestVCardIngestDeclaresABoundedRetryLadder(t *testing.T) {
	got := vcardIngestInsertOpts().MaxAttempts
	if got != vcardIngestMaxAttempts {
		t.Fatalf("enqueued MaxAttempts = %d, want the declared ladder %d", got, vcardIngestMaxAttempts)
	}
	if got <= 0 || got >= river.MaxAttemptsDefault {
		t.Fatalf("MaxAttempts = %d, want a positive bound well below River's own default of %d", got, river.MaxAttemptsDefault)
	}
}

// TestAFailedStageDoesNotCostItsSiblingsTheirOwnReview: a mailed message
// carrying two near-matches used to lose the second card's review to the
// first one's staging fault, and the whole job with it. Both cards must get
// their own attempt regardless of which one fails.
func TestAFailedStageDoesNotCostItsSiblingsTheirOwnReview(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Broken Card"}, {FullName: "Fine Card"}}
	results := []people.VCardResult{
		{Index: 0, Outcome: people.VCardNeedsReview},
		{Index: 1, Outcome: people.VCardNeedsReview},
	}
	var attempts int
	failing := errors.New("a staging conflict a test forced")
	stage := func(_ context.Context, entry people.VCardEntry, _ *ids.PersonID) error {
		attempts++
		if entry.FullName == "Broken Card" {
			return failing
		}
		return nil
	}

	err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, noopRecordFailure, entries, results)
	if err == nil {
		t.Fatal("a batch with one failed card returned no error — nothing would tell River to retry it")
	}
	if !errors.Is(err, failing) {
		t.Fatalf("aggregate error does not wrap the card's own error: %v — Work classifies retryability by errors.Is", err)
	}
	if attempts != 2 {
		t.Fatalf("stage was called %d times, want 2 — the second card must get its own attempt regardless of the first's outcome", attempts)
	}
}

// TestAFailedStageDoesNotCostItsSiblingsTheirOwnReview's mirror: the failure
// lands on the SECOND card, so a bug that only kept going after a successful
// attempt (rather than after any attempt) would still pass the first test.
func TestALaterCardsFailureDoesNotSkipAnEarlierCardsAttempt(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Fine Card"}, {FullName: "Broken Card"}}
	results := []people.VCardResult{
		{Index: 0, Outcome: people.VCardNeedsReview},
		{Index: 1, Outcome: people.VCardNeedsReview},
	}
	var attempts int
	stage := func(_ context.Context, entry people.VCardEntry, _ *ids.PersonID) error {
		attempts++
		if entry.FullName == "Broken Card" {
			return errors.New("a staging conflict a test forced")
		}
		return nil
	}

	if err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, noopRecordFailure, entries, results); err == nil {
		t.Fatal("a batch with one failed card returned no error — nothing would tell River to retry it")
	}
	if attempts != 2 {
		t.Fatalf("stage was called %d times, want 2", attempts)
	}
}

// TestEveryCardFailingNamesTheFullCount — the aggregate error's count must
// track every failure, not just whether at least one occurred.
func TestEveryCardFailingNamesTheFullCount(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "First"}, {FullName: "Second"}}
	results := []people.VCardResult{
		{Index: 0, Outcome: people.VCardNeedsReview},
		{Index: 1, Outcome: people.VCardNeedsReview},
	}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error {
		return errors.New("a staging conflict a test forced")
	}

	err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, noopRecordFailure, entries, results)
	if err == nil || !strings.Contains(err.Error(), "staging 2 of 2") {
		t.Fatalf("got %v, want an error naming both cards failed", err)
	}
}

// TestStageReviewsSucceedsWhenEveryCardStages — the ordinary case still
// answers cleanly, so the aggregate-error path above is additive rather
// than a permanent fault where none existed before.
func TestStageReviewsSucceedsWhenEveryCardStages(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Fine Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardNeedsReview}}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error { return nil }

	if err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, noopRecordFailure, entries, results); err != nil {
		t.Fatalf("every card staged cleanly, want no error, got: %v", err)
	}
}

// TestStageReviewsSkipsCardsThatDoNotNeedReview — a created, updated or
// skipped card (any outcome but VCardNeedsReview) is not a staging
// candidate at all, and must never reach the stager.
func TestStageReviewsSkipsCardsThatDoNotNeedReview(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Created Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardCreated}}
	called := false
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error {
		called = true
		return nil
	}

	if err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, noopRecordFailure, entries, results); err != nil {
		t.Fatalf("no eligible card, want no error, got: %v", err)
	}
	if called {
		t.Error("stage was called for a card that does not need review")
	}
}

// TestAFailedStageNotifiesTheImporter: the headline gap this file exists to
// close. A mailed card that fails to stage used to leave nothing a person
// would ever read; the notice port must fire once per failed card, naming
// the same activity the failure came from and that card's own 1-based
// position (the same numbering the log line beside it already uses).
func TestAFailedStageNotifiesTheImporter(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Broken Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardNeedsReview}}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error {
		return errors.New("a staging conflict a test forced")
	}
	activity := ids.NewV7()
	type call struct {
		activity ids.UUID
		card     int
	}
	var notified []call
	recordFailure := func(_ context.Context, a ids.UUID, card int) error {
		notified = append(notified, call{a, card})
		return nil
	}

	if err := stageReviewsWith(context.Background(), quietTestLogger(), activity, stage, recordFailure, entries, results); err == nil {
		t.Fatal("a failed card returned no error — nothing would tell River to retry it")
	}
	if len(notified) != 1 || notified[0] != (call{activity, 1}) {
		t.Fatalf("recordFailure calls = %+v, want exactly one call naming activity %s, card 1", notified, activity)
	}
}

// TestAFailedStageNotifiesForEveryFailedCard — two failed cards in one
// message must each raise their own notice, with their own distinct card
// number: a person checking after the fact should not learn about only one
// of two cards that silently failed, or be unable to tell the two apart.
func TestAFailedStageNotifiesForEveryFailedCard(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Broken One"}, {FullName: "Broken Two"}}
	results := []people.VCardResult{
		{Index: 0, Outcome: people.VCardNeedsReview},
		{Index: 1, Outcome: people.VCardNeedsReview},
	}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error {
		return errors.New("a staging conflict a test forced")
	}
	var cards []int
	recordFailure := func(_ context.Context, _ ids.UUID, card int) error {
		cards = append(cards, card)
		return nil
	}

	if err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, recordFailure, entries, results); err == nil {
		t.Fatal("both cards failed and returned no error")
	}
	if len(cards) != 2 || cards[0] != 1 || cards[1] != 2 {
		t.Fatalf("recordFailure was called with cards %v, want [1 2] — each failed card notified once, under its own number", cards)
	}
}

// TestAStagedCardNeverTriggersAFailureNotice — the mirror of the two tests
// above: a card that stages cleanly must never be noted as failed, or a
// person would be told about a review that in fact landed.
func TestAStagedCardNeverTriggersAFailureNotice(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Fine Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardNeedsReview}}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error { return nil }
	called := false
	recordFailure := func(context.Context, ids.UUID, int) error {
		called = true
		return nil
	}

	if err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, recordFailure, entries, results); err != nil {
		t.Fatalf("every card staged cleanly, want no error, got: %v", err)
	}
	if called {
		t.Error("recordFailure was called for a card that staged successfully")
	}
}

// TestANotesOwnFailureDoesNotChangeTheCardsOutcome — the notice is best
// effort: if the write that raises it fails itself, that must not turn a
// card's own staging error into a DIFFERENT error, or Work's errors.Is
// classification (ErrPermissionDenied/ErrNotFound mean "not a fault") could
// silently stop matching.
func TestANotesOwnFailureDoesNotChangeTheCardsOutcome(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Broken Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardNeedsReview}}
	stageFailure := errors.New("a staging conflict a test forced")
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error { return stageFailure }
	recordFailure := func(context.Context, ids.UUID, int) error {
		return errors.New("the notice write itself failed, in a test")
	}

	err := stageReviewsWith(context.Background(), quietTestLogger(), ids.UUID{}, stage, recordFailure, entries, results)
	if !errors.Is(err, stageFailure) {
		t.Fatalf("aggregate error does not wrap the card's own staging error: %v — "+
			"a failed notice write must not mask or replace it", err)
	}
}

// TestRecordStagingFailureRefusesAnUnboundActor: recordStagingFailure reads
// its recipient from the SAME actor context stageReviewsWith already runs
// under (asMailboxGrantor's own), never from a parameter — so a context that
// somehow reached this without one must refuse rather than notify under a
// zero user id, which would address the Worklist line to nobody. The
// worker's own pool is left nil: the guard must return before ever touching
// it.
func TestRecordStagingFailureRefusesAnUnboundActor(t *testing.T) {
	w := &vcardIngestWorker{}
	if err := w.recordStagingFailure(context.Background(), ids.NewV7(), 1); !errors.Is(err, errNoMailboxGrantorBound) {
		t.Fatalf("recordStagingFailure with no actor bound = %v, want errNoMailboxGrantorBound", err)
	}
}
