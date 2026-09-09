// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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

// TestAFailedStageDoesNotCostItsSiblingsTheirOwnReview — margince#3410's
// remaining half: a mailed message carrying two near-matches used to lose
// the second card's review to the first one's staging fault, and the whole
// job with it. Both cards must get their own attempt regardless of order.
func TestAFailedStageDoesNotCostItsSiblingsTheirOwnReview(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Broken Card"}, {FullName: "Fine Card"}}
	results := []people.VCardResult{
		{Index: 0, Outcome: people.VCardNeedsReview},
		{Index: 1, Outcome: people.VCardNeedsReview},
	}
	var staged []int
	failing := errors.New("a staging conflict a test forced")
	stage := func(_ context.Context, entry people.VCardEntry, _ *ids.PersonID) error {
		staged = append(staged, len(staged))
		if entry.FullName == "Broken Card" {
			return failing
		}
		return nil
	}

	err := stageReviewsWith(context.Background(), quietTestLogger(), stage, entries, results)
	if err == nil {
		t.Fatal("a batch with one failed card returned no error — nothing would tell River to retry it")
	}
	if len(staged) != 2 {
		t.Fatalf("stage was called %d times, want 2 — the second card must get its own attempt regardless of the first's outcome", len(staged))
	}
}

// TestStageReviewsSucceedsWhenEveryCardStages — the ordinary case still
// answers cleanly, so the aggregate-error path above is additive rather
// than a permanent fault where none existed before.
func TestStageReviewsSucceedsWhenEveryCardStages(t *testing.T) {
	entries := []people.VCardEntry{{FullName: "Fine Card"}}
	results := []people.VCardResult{{Index: 0, Outcome: people.VCardNeedsReview}}
	stage := func(context.Context, people.VCardEntry, *ids.PersonID) error { return nil }

	if err := stageReviewsWith(context.Background(), quietTestLogger(), stage, entries, results); err != nil {
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

	if err := stageReviewsWith(context.Background(), quietTestLogger(), stage, entries, results); err != nil {
		t.Fatalf("no eligible card, want no error, got: %v", err)
	}
	if called {
		t.Error("stage was called for a card that does not need review")
	}
}
