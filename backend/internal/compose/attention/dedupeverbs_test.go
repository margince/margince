// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which verbs a duplicate pair offers, and to whom.
//
// Split out of feed_test.go because it is one question asked six ways, and the
// answer is assembled from THREE reads that the card then has to agree with: is
// each side visible, could this reader change each side, and would the merge
// take this pair at all. Each read withholds something different, and every one
// of them shipped a control that refused after the press before it was asked
// here.
//
// The cases come in pairs on purpose. A withholding check is satisfied by a
// lane that has stopped offering the verb to anybody, so each is followed by
// the reader who still gets it.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnreadableSideCostsEveryVerbRatherThanLeakingTheRecord(t *testing.T) {
	hidden := ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, unreadable: hidden, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: hidden,
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("a record this reader may not see must not fail the whole day: %v", err)
	}
	if out.NeedsYou[0].Pair != nil {
		t.Fatal("a side the reader may not read was named anyway")
	}
	// NO verb, not merely no merge. Dismissing the pair writes it for the whole
	// workspace, so a reader who may not read one side has no more business
	// declaring them different than combining them.
	if len(out.NeedsYou[0].Actions) != 0 {
		t.Errorf("actions %v offered over a record the reader cannot see", out.NeedsYou[0].Actions)
	}
}

// A pair naming a record the reader may see but could not change offers no
// verb. This is the ordinary case for a rep: everyone here reads every
// colleague's customer records and can write almost none of them, so a merge
// button decided by visibility refused every press it invited.
func TestAPairTheReaderCannotChangeIsShownWithoutAVerb(t *testing.T) {
	theirs := ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, undecidable: theirs, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: theirs,
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	// The pair is still NAMED. Withholding the verb is not withholding the
	// fact: a reader who cannot merge these two still needs to know a duplicate
	// is waiting, and who it is about.
	if out.NeedsYou[0].Pair == nil {
		t.Fatal("a pair the reader may read was hidden because they cannot decide it")
	}
	if len(out.NeedsYou[0].Actions) != 0 {
		t.Errorf("actions %v offered over a record the reader cannot change, which is the press that always refused",
			out.NeedsYou[0].Actions)
	}
}

// Authority over ONE side is not authority to decide the pair: the merge
// archives one record and rewrites the other, so half the authority settles
// nothing and a verb offered on it would refuse.
func TestOwningOneSideOfAPairDoesNotOfferTheVerb(t *testing.T) {
	mine, theirs := ids.NewV7(), ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, undecidable: theirs, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "company", Confidence: 1,
			LeftID: mine, RightID: theirs,
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.NeedsYou[0].Actions) != 0 {
		t.Errorf("actions %v offered on a pair the reader owns half of", out.NeedsYou[0].Actions)
	}
}

// A reader who can change both records still gets the verb. Without this the
// two tests above would pass against a lane that had simply stopped offering
// merge to anybody.
func TestAPairTheReaderCanChangeStillOffersTheVerb(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	var offered bool
	for _, action := range out.NeedsYou[0].Actions {
		if action == "merge" {
			offered = true
		}
	}
	if !offered {
		t.Error("a steward who can change both records was offered no way to settle the pair")
	}
}

// A pair the MERGE would refuse still offers the DISMISSAL, and only that.
//
// Two companies each carrying live work do not combine (PROJ-LIFE-4). Authority
// cannot see that: it is a property of the PAIR, not of either record, so a
// data steward who may write both was handed a button that answered 409 every
// time — the same dead end the authority check above was added to remove,
// reached by another path.
//
// Withholding the merge is not withholding the decision. The other arm of
// DisposeDedupeCandidate takes write authority over both sides and applies no
// project rule, so this steward can settle the pair as a false positive — and
// when the lane withheld that verb too, the pair sat on the page as a question
// nobody could answer from here.
func TestAPairTheMergeWouldRefuseStillOffersTheDismissal(t *testing.T) {
	pairID := ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, unsettleable: pairID, pairs: []DuplicatePair{{
			ID: pairID, EntityType: "company", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	// Named, like the authority case: a steward who cannot merge these two
	// still needs to know the duplicate is waiting.
	if out.NeedsYou[0].Pair == nil {
		t.Fatal("a pair the reader may read was hidden because the merge would refuse it")
	}
	var dismissal bool
	for _, action := range out.NeedsYou[0].Actions {
		if action == "merge" {
			t.Error("merge is offered on a pair the merge itself refuses — the press " +
				"answers 409 and says nothing about what to do next")
		}
		dismissal = dismissal || action == actionDismiss
	}
	if !dismissal {
		t.Error("a steward who may write both records was offered no way to clear a " +
			"false positive the merge will never take, which leaves the pair on the " +
			"page as a question nobody can answer from here")
	}
}

// And a pair nothing blocks still gets the verb, or the check above is
// satisfied by a lane that offers merge to nobody.
func TestAnUnblockedPairStillOffersTheVerb(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "company", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	var offered bool
	for _, action := range out.NeedsYou[0].Actions {
		offered = offered || action == "merge"
	}
	if !offered {
		t.Error("a writable pair the merge would accept offers no verb — the lane has " +
			"stopped offering merge at all, which the withholding cases above cannot see")
	}
}
