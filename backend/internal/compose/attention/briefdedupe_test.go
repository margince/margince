// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One deal, one row — and the fold that gets there without moving anything else.
//
// The morning it exists for showed the same deal twice: once because the night
// ranked it and once because the day's at-risk lane raised it. Same name, same
// figures, two places to answer it, and both counted in the readings above.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func rankedBriefRow(itemID, dealID ids.UUID) ranked {
	return ranked{item: crmcontracts.WorklistItem{
		Id:       itemID.String(),
		Source:   crmcontracts.WorklistItemSourceBriefItem,
		Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
		Actions:  []crmcontracts.WorklistItemActions{"act", "set_aside", "dismiss"},
		Subject: &crmcontracts.AttentionSubject{
			Type: subjectDeal, Id: openapi_types.UUID(dealID),
		},
	}}
}

func atRiskRow(dealID ids.UUID) ranked {
	return ranked{
		item: crmcontracts.WorklistItem{
			Id:       dealID.String(),
			Source:   crmcontracts.WorklistItemSourceDealAtRisk,
			Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
			Level:    3,
			Actions:  []crmcontracts.WorklistItemActions{"open"},
			Subject: &crmcontracts.AttentionSubject{
				Type: subjectDeal, Id: openapi_types.UUID(dealID),
			},
		},
		semanticLevel: 3,
	}
}

// THE case the fold exists for.
func TestADealSurfacedByTheNightIsOneRowNotTwo(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()
	rows := []ranked{atRiskRow(deal), rankedBriefRow(briefItem, deal)}

	kept := foldBriefIntoRisk(rows)

	if len(kept) != 1 {
		t.Fatalf("kept %d rows, want the one at-risk row", len(kept))
	}
	if kept[0].item.Source != crmcontracts.WorklistItemSourceDealAtRisk {
		t.Errorf("the surviving row is %q — the at-risk row carries the day's figures "+
			"and is the one that must survive", kept[0].item.Source)
	}
}

// The brief's verbs answer to the brief's endpoints, so the surviving row has to
// carry its id or the fold silently takes act, set-aside and dismiss away.
func TestTheFoldedRowKeepsTheBriefMarkVerbs(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()

	kept := foldBriefIntoRisk([]ranked{atRiskRow(deal), rankedBriefRow(briefItem, deal)})

	if kept[0].item.BriefItemId == nil {
		t.Fatal("the surviving row names no brief item — the night can never be told it was answered")
	}
	if ids.UUID(*kept[0].item.BriefItemId) != briefItem {
		t.Errorf("brief_item_id = %v, want the folded entry %v", *kept[0].item.BriefItemId, briefItem)
	}
}

// The night's score is the reason the fold is more than tidying: it is the only
// signal that knows how this deal compares to every other deal the night weighed.
func TestTheFoldedRowInheritsTheNightsScore(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()
	brief := rankedBriefRow(briefItem, deal)
	brief.opportunity = 0.81

	kept := foldBriefIntoRisk([]ranked{atRiskRow(deal), brief})

	if kept[0].opportunity != 0.81 {
		t.Errorf("opportunity = %v, want the night's own score", kept[0].opportunity)
	}
}

// A deal the night surfaced and the day's lanes did not keeps its row. This fold
// removes duplicates, never rows.
func TestADealOnlyTheNightFoundKeepsItsRow(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()

	kept := foldBriefIntoRisk([]ranked{rankedBriefRow(briefItem, deal)})

	if len(kept) != 1 {
		t.Fatalf("kept %d rows, want the brief's own row", len(kept))
	}
	if kept[0].item.Source != crmcontracts.WorklistItemSourceBriefItem {
		t.Error("a deal only the night found lost its row")
	}
}

// THE invariant the fold is allowed to keep and nothing more. semanticLevelOf
// has three readers — the summary's signals, bucketsOf's partition, and the
// partition a walk freezes — and a fold that moved a level would move a figure
// a manager reads.
func TestFoldingABriefItemMovesNoBucketCount(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()
	before := bucketsOf([]ranked{atRiskRow(deal)})

	kept := foldBriefIntoRisk([]ranked{atRiskRow(deal), rankedBriefRow(briefItem, deal)})
	after := bucketsOf(kept)

	if before != after {
		t.Errorf("the buckets moved from %+v to %+v — the fold changed what a row MEANS, "+
			"not just how many rows say it", before, after)
	}
	if kept[0].semanticLevel != 3 {
		t.Errorf("semantic level = %d, want the survivor's own 3", kept[0].semanticLevel)
	}
}

// Two rows about one deal, and the FIRST is the one the brief attaches to: it is
// the one the ranking shows highest, and the verbs must land where the reader
// will actually look.
func TestTheBriefAttachesToTheHighestRowAboutItsDeal(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()
	first, second := atRiskRow(deal), atRiskRow(deal)
	second.item.Id = "second"

	kept := foldBriefIntoRisk([]ranked{first, second, rankedBriefRow(briefItem, deal)})

	if len(kept) != 2 {
		t.Fatalf("kept %d rows, want both at-risk rows", len(kept))
	}
	if kept[0].item.BriefItemId == nil {
		t.Fatal("the first row did not receive the brief's id")
	}
	if kept[1].item.BriefItemId != nil {
		t.Error("a second row about the same deal also claimed the brief's verbs")
	}
}

// The night's score reaches the row that SURVIVES, which is why it is stamped
// before the fold rather than after it.
func TestTheNightsScoreReachesTheRowThatSurvives(t *testing.T) {
	deal := ids.NewV7()
	rows := []ranked{atRiskRow(deal)}

	rows = stampOpportunity(rows, map[ids.UUID]float64{deal: 0.64})

	if rows[0].opportunity != 0.64 {
		t.Errorf("opportunity = %v, want the score for this deal", rows[0].opportunity)
	}
}

// A deal the night never weighed keeps zero and therefore loses the step to any
// deal it did — the honest answer, not a missing one.
func TestADealTheNightNeverWeighedScoresZero(t *testing.T) {
	rows := []ranked{atRiskRow(ids.NewV7())}

	rows = stampOpportunity(rows, map[ids.UUID]float64{ids.NewV7(): 0.9})

	if rows[0].opportunity != 0 {
		t.Errorf("opportunity = %v, want zero for a deal the night did not rank", rows[0].opportunity)
	}
}

// THE distinction the whole field turns on: the run's DATA CUTOFF, not the later
// instant at which it finished writing its rows down.
func TestChangedSinceBriefUsesTheRunsAsOfNotItsGeneratedAt(t *testing.T) {
	readAt := time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)
	// The reply landed after the night READ the records and before it finished
	// writing them. Judged by generated_at it would read as old.
	row := atRiskRow(ids.NewV7())
	row.occurredAt = time.Date(2026, 9, 4, 6, 20, 0, 0, time.UTC)

	marked := markChangedSinceBrief([]ranked{row}, readAt)

	if marked[0].item.ChangedSinceBrief == nil {
		t.Fatal("the row carries no answer at all")
	}
	if !*marked[0].item.ChangedSinceBrief {
		t.Error("a reply that landed after the run's cutoff reads as something the night saw")
	}
}

// Something the night did read is not new.
func TestSomethingTheNightSawIsNotChanged(t *testing.T) {
	readAt := time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)
	row := atRiskRow(ids.NewV7())
	row.occurredAt = time.Date(2026, 9, 3, 17, 0, 0, 0, time.UTC)

	marked := markChangedSinceBrief([]ranked{row}, readAt)

	if marked[0].item.ChangedSinceBrief == nil || *marked[0].item.ChangedSinceBrief {
		t.Errorf("changed = %v, want false for a record the night read", marked[0].item.ChangedSinceBrief)
	}
}

// No run means no answer. Absent and false are different facts: one says there
// was no night, the other says the night saw this.
func TestWithNoRunNoRowClaimsTheNightSawIt(t *testing.T) {
	row := atRiskRow(ids.NewV7())
	row.occurredAt = time.Date(2026, 9, 4, 6, 20, 0, 0, time.UTC)

	marked := markChangedSinceBrief([]ranked{row}, time.Time{})

	if marked[0].item.ChangedSinceBrief != nil {
		t.Errorf("changed = %v with no run to compare against — absent is the only honest answer",
			*marked[0].item.ChangedSinceBrief)
	}
}

// A row that cannot date what it reports says nothing rather than claiming the
// night saw it.
func TestARowWithNoMaterialMomentClaimsNothing(t *testing.T) {
	readAt := time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)

	marked := markChangedSinceBrief([]ranked{atRiskRow(ids.NewV7())}, readAt)

	if marked[0].item.ChangedSinceBrief != nil {
		t.Error("a row with no material moment claimed an answer it cannot support")
	}
}

// The fold is a FILTER, not a re-sort: the rows it keeps stay in the order they
// arrived, so it is safe to run before the ranking.
//
// A brief row the fold cannot match is appended after the rows it walked past,
// which is the one place order could move. It cannot matter — every caller
// sorts afterwards, and sortByRank is total over these rows — but a reader of
// this file should not have to take that on trust.
func TestTheFoldKeepsTheRowsItDoesNotTouchInOrder(t *testing.T) {
	first, second, third := atRiskRow(ids.NewV7()), atRiskRow(ids.NewV7()), atRiskRow(ids.NewV7())

	kept := foldBriefIntoRisk([]ranked{first, second, third})

	if len(kept) != 3 {
		t.Fatalf("kept %d rows, want all three", len(kept))
	}
	for at, want := range []ranked{first, second, third} {
		if kept[at].item.Id != want.item.Id {
			t.Errorf("row %d is %s, want %s — the fold re-ordered rows it does not touch",
				at, kept[at].item.Id, want.item.Id)
		}
	}
}

// A page with no brief rows at all comes back untouched.
func TestAPageWithNoBriefRowsIsUnchanged(t *testing.T) {
	deal := ids.NewV7()

	kept := foldBriefIntoRisk([]ranked{atRiskRow(deal)})

	if len(kept) != 1 || kept[0].item.BriefItemId != nil {
		t.Errorf("a page with no brief rows came back changed: %+v", kept)
	}
}

// The opportunity step breaks a tie INSIDE a level and never crosses one.
//
// This is the invariant that keeps the one feed one feed. The night ranks deals
// against each other on facts the day's lanes do not gather; the levels are the
// product's hard rule about what kind of work outranks what. A composite that
// could lift a row past a level would be the second ranking system this whole
// change exists to end.
func TestTheOpportunityStepNeverChangesARowsSemanticLevel(t *testing.T) {
	// A level-4 deal the night loved, against a level-3 deal it never saw.
	loved := atRiskRow(ids.NewV7())
	loved.item.Level = 4
	loved.semanticLevel = 4
	loved.opportunity = 0.99
	unseen := atRiskRow(ids.NewV7())

	ranked := rankAllFor([]ranked{loved, unseen}, ids.UUID{})

	if ranked[0].Id != unseen.item.Id {
		t.Errorf("the level-4 row the night scored 0.99 outranked a level-3 row — " +
			"the composite crossed a level, which is the second ranking system this feed ends")
	}
}

// Two rows at ONE level, and the night's score decides between them.
func TestTwoRiskRowsAtOneLevelOrderByOpportunity(t *testing.T) {
	quiet, promising := atRiskRow(ids.NewV7()), atRiskRow(ids.NewV7())
	promising.opportunity = 0.8

	ranked := rankAllFor([]ranked{quiet, promising}, ids.UUID{})

	if ranked[0].Id != promising.item.Id {
		t.Error("two rows at one level did not order by the night's score")
	}
}

// And it says so, with both sides, so a reader can check the claim.
func TestTheOpportunityStepExplainsItselfWithBothScores(t *testing.T) {
	quiet, promising := atRiskRow(ids.NewV7()), atRiskRow(ids.NewV7())
	promising.opportunity = 0.8
	quiet.opportunity = 0.2

	ranked := rankAllFor([]ranked{quiet, promising}, ids.UUID{})

	above := ranked[0].AboveNext
	if above == nil {
		t.Fatal("the top row explains nothing")
	}
	if above.Comparator != crmcontracts.WorklistComparisonComparatorOpportunity {
		t.Fatalf("comparator = %q, want opportunity", above.Comparator)
	}
	if above.Mine == nil || above.Mine.Score == nil || above.Theirs == nil || above.Theirs.Score == nil {
		t.Fatalf("the comparison carries no scores: %+v", above)
	}
	if *above.Mine.Score <= *above.Theirs.Score {
		t.Errorf("mine %v is not above theirs %v", *above.Mine.Score, *above.Theirs.Score)
	}
}

// A brief row AHEAD of the row it folds into.
//
// THE case the fold's index bookkeeping exists for, and the one every other test
// in this file cannot see. The fold walks once, dropping brief rows and
// recording where each survivor landed; the input index and the output index
// diverge the moment a row is dropped. Every fixture above puts the brief row
// LAST, so nothing is dropped before a survivor is recorded and the two indexes
// stay equal — which means all of them pass against a version that records the
// input position and writes at it.
//
// This one puts the brief row first. The at-risk row is then at input position 1
// and output position 0, and a fold that confused them writes the brief's id
// past the end of a one-row slice or onto the wrong row.
//
// Mutation-checked: recording `inputAt` instead of `len(kept)` fails this test
// and passes every other one in the file.
func TestABriefRowAheadOfItsSurvivorStillLandsOnIt(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()

	kept := foldBriefIntoRisk([]ranked{
		rankedBriefRow(briefItem, deal),
		atRiskRow(deal),
	})

	if len(kept) != 1 {
		t.Fatalf("kept %d rows, want the one at-risk row", len(kept))
	}
	if kept[0].item.BriefItemId == nil {
		t.Fatal("the survivor lost the brief's id — the fold wrote it at an input position " +
			"while the survivors live in the output slice")
	}
	if ids.UUID(*kept[0].item.BriefItemId) != briefItem {
		t.Errorf("brief_item_id = %v, want %v", *kept[0].item.BriefItemId, briefItem)
	}
}

// The same divergence, with several rows dropped before the survivor. Two brief
// rows ahead of it push the input and output indexes two apart.
func TestSeveralFoldedRowsAheadOfASurvivorStillLandOnIt(t *testing.T) {
	first, second, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	briefItem := ids.NewV7()

	kept := foldBriefIntoRisk([]ranked{
		rankedBriefRow(ids.NewV7(), first),
		rankedBriefRow(ids.NewV7(), second),
		rankedBriefRow(briefItem, deal),
		atRiskRow(deal),
	})

	// The two unmatched brief rows keep their own rows; the third folds.
	if len(kept) != 3 {
		t.Fatalf("kept %d rows, want the at-risk row and the two unmatched brief rows", len(kept))
	}
	var survivor *ranked
	for at := range kept {
		if kept[at].item.Source == crmcontracts.WorklistItemSourceDealAtRisk {
			survivor = &kept[at]
		}
	}
	if survivor == nil {
		t.Fatal("the at-risk row is gone")
	}
	if survivor.item.BriefItemId == nil || ids.UUID(*survivor.item.BriefItemId) != briefItem {
		t.Errorf("brief_item_id = %v, want %v — the fold lost track of where the survivor landed",
			survivor.item.BriefItemId, briefItem)
	}
}

// The brief's verbs survive the pass that removes the row they were attached to.
//
// THE defect that made this fold run last. A deal can be both in the night's
// brief and carrying an unanswered message. The at-risk row took the brief's id;
// dropDealsAlreadyWaiting then deleted that row, because one unanswered message
// is one row; and the waiting row that absorbs it inherits the deal's figures
// and reasons but not the brief's id. The entry vanished from the page, and with
// it the act, set-aside and dismiss verbs that answer the night — which the
// fold's own header calls a worse outcome than the duplicate it prevents.
//
// Written over the two passes IN THE ORDER worklist.go runs them, because the
// order is the fix: neither pass alone is wrong.
func TestTheBriefsVerbsSurviveTheRowTheyWereAttachedTo(t *testing.T) {
	deal, briefItem := ids.NewV7(), ids.NewV7()
	waiting := waitingRowAbout(deal)

	rows := dropDealsAlreadyWaiting([]ranked{atRiskRow(deal), waiting})
	rows = foldBriefIntoRisk(append(rows, rankedBriefRow(briefItem, deal)))

	var carrier *ranked
	for at := range rows {
		if rows[at].item.BriefItemId != nil {
			carrier = &rows[at]
		}
	}
	if carrier == nil {
		t.Fatal("no row on the page names the brief entry — the night can never be told it was " +
			"answered, and act, set aside and dismiss are gone with it")
	}
	if ids.UUID(*carrier.item.BriefItemId) != briefItem {
		t.Errorf("brief_item_id = %v, want %v", *carrier.item.BriefItemId, briefItem)
	}
}

// waitingRowAbout is the customer_waiting row that absorbs a deal's at-risk row.
func waitingRowAbout(dealID ids.UUID) ranked {
	return ranked{item: crmcontracts.WorklistItem{
		Id:       ids.NewV7().String(),
		Source:   sourceWaiting,
		Category: crmcontracts.WorklistItemCategoryCustomerWaiting,
		Level:    1,
		Actions:  []crmcontracts.WorklistItemActions{"open"},
		Subject: &crmcontracts.AttentionSubject{
			Type: subjectDeal, Id: openapi_types.UUID(dealID),
		},
	}}
}

// One reader's night never reaches another reader's page.
//
// THE property the predecessor of this branch got wrong, in the same shape: it
// kept per-read state on the Service, which one process shares across every
// request, and a reader whose own brief ran empty inherited the last reader's.
//
// Written against readingScores rather than against scoresOf, and the
// distinction is the whole point. scoresOf is pure — it has nowhere to leave a
// previous caller's answer, so a test over it proves the easy half. The scores
// and the cutoff are FIELDS on the Service, which is exactly the shape that
// leaked, so the assertion has to be that the shared value is untouched.
//
// Mutation-checked: making readingScores assign through `s` rather than a copy
// fails both halves.
func TestOneReadersNightDoesNotReachAnothersPage(t *testing.T) {
	theirDeal := ids.NewV7()
	theirCutoff := time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)
	shared := &Service{
		briefScores: map[ids.UUID]float64{theirDeal: 0.9},
		briefCutoff: theirCutoff,
	}

	// The next reader's brief ran empty: no scores, no run, no cutoff.
	mine := shared.readingScores(nil, time.Time{})

	if len(shared.briefScores) != 1 || shared.briefScores[theirDeal] != 0.9 {
		t.Errorf("the shared service's scores became %v — one read wrote through the value "+
			"every request shares", shared.briefScores)
	}
	if !shared.briefCutoff.Equal(theirCutoff) {
		t.Errorf("the shared service's cutoff became %v, want %v", shared.briefCutoff, theirCutoff)
	}
	if len(mine.briefScores) != 0 {
		t.Errorf("my page carries %v — another reader's night", mine.briefScores)
	}
	if !mine.briefCutoff.IsZero() {
		t.Errorf("my page carries cutoff %v, and my brief never ran", mine.briefCutoff)
	}
}
