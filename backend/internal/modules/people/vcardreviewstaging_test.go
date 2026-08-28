// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The import's review results reach the staging port: each refused card, with
// its own entry and its own candidate, and nothing else.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyRefusedCardsReachTheReviewStager(t *testing.T) {
	candidate := ids.New[ids.PersonKind]()
	entries := []VCardEntry{
		{FullName: "Created Fine"},
		{FullName: "Anna Weber"},
		{FullName: "Also Created"},
	}
	results := []VCardResult{
		{Index: 0, Outcome: VCardCreated},
		{Index: 1, Outcome: VCardNeedsReview, PersonID: &candidate},
		{Index: 2, Outcome: VCardCreated},
	}
	var staged []VCardEntry
	var candidates []*ids.PersonID
	h := Handlers{}.WithVCardReviewStager(func(_ context.Context, entry VCardEntry, c *ids.PersonID) error {
		staged = append(staged, entry)
		candidates = append(candidates, c)
		return nil
	})

	h.stageVCardReviews(context.Background(), entries, results)

	if len(staged) != 1 || staged[0].FullName != "Anna Weber" {
		t.Fatalf("staged %+v, want exactly the refused card", staged)
	}
	if candidates[0] == nil || *candidates[0] != candidate {
		t.Errorf("the stager got candidate %v, want the result's own", candidates[0])
	}
}

// A staging fault must not un-tell the reader what the import already did:
// the loop carries on past it, and a role with no stager wired stages
// nothing without failing.
func TestAStagingFaultDoesNotStopTheOtherCards(t *testing.T) {
	entries := []VCardEntry{{FullName: "First"}, {FullName: "Second"}}
	results := []VCardResult{
		{Index: 0, Outcome: VCardNeedsReview},
		{Index: 1, Outcome: VCardNeedsReview},
	}
	var staged int
	h := Handlers{}.WithVCardReviewStager(func(context.Context, VCardEntry, *ids.PersonID) error {
		staged++
		if staged == 1 {
			return errors.New("the approvals queue is unreachable")
		}
		return nil
	})
	h.stageVCardReviews(context.Background(), entries, results)
	if staged != 2 {
		t.Errorf("the loop stopped after the fault: %d of 2 cards offered", staged)
	}

	Handlers{}.stageVCardReviews(context.Background(), entries, results)
}
