// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The figures a client sums over the cards it loaded must reconcile with the
// day's own item counts, across pages, folds, arrivals and departures.

import (
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// itemsOn counts the underlying items a page's cards stand for, the way the
// contract's units table tells a client to.
func itemsOn(queue []crmcontracts.WorklistItem) int {
	items := 0
	for _, card := range queue {
		if card.Batch != nil {
			items += card.Batch.Count
			continue
		}
		items++
	}
	return items
}

func consideredOf(counts []crmcontracts.WorklistCount) int {
	total := 0
	for _, count := range counts {
		total += count.Considered
	}
	return total
}

func aDayOfTasksAndPairs(tasks, pairs int) crmcontracts.Attention {
	day := aDayOfTasks(tasks)
	day.NeedsYou = aDayOfAlikeDecisions(pairs).NeedsYou
	return day
}

// Eight duplicate pairs fold into one card behind ten tasks, walked three cards
// a page. One pair is resolved and one task arrives after page one.
func TestFoldedPairsReconcileWithTheDayAcrossAWalk(t *testing.T) {
	t.Parallel()
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(&walkStore{})

	first := walkFrom(t, svc, aDayOfTasksAndPairs(10, 8), 3, worklistCursor{})
	started := consideredOf(first.Counts)
	if started != 18 || first.Summary.Total != 18 {
		t.Fatalf("the day weighed %d items (summary %d), want 18: ten tasks and eight pairs",
			started, first.Summary.Total)
	}
	if first.NextCursor == nil {
		t.Fatal("eleven cards paged three at a time minted no cursor")
	}

	moved := aDayOfTasksAndPairs(10, 8)
	moved.NeedsYou = moved.NeedsYou[1:]
	moved.Planned = append(moved.Planned, item("task-arrived", "task", withDue(rankInstant)))

	served, groups, last := itemsOn(first.Queue), 0, first
	cursor := decodedCursor(t, *first.NextCursor)
	for range 10 {
		page := walkFrom(t, svc, moved, 3, cursor)
		served += itemsOn(page.Queue)
		for _, card := range page.Queue {
			if card.Id == "task-arrived" {
				t.Fatal("work that arrived mid-walk was served on the walk")
			}
			if card.Batch != nil {
				groups++
				if shown := countFor(t, page, "decisions").Shown; shown != card.Batch.Count {
					t.Errorf("the page counts %d decision items under a card standing for %d", shown, card.Batch.Count)
				}
			}
		}
		last = page
		if page.NextCursor == nil {
			break
		}
		cursor = decodedCursor(t, *page.NextCursor)
	}

	if groups != 1 {
		t.Errorf("the folded pairs were served as %d cards over the walk, want one", groups)
	}
	if last.Walk == nil {
		t.Fatal("the last page carried no walk, so the departure cannot be reconciled")
	}
	if gone := last.Walk.ChangedSinceSnapshot; served+gone != started {
		t.Errorf("served %d items and %d left, which does not reconcile with the %d the walk started with",
			served, gone, started)
	}
	if last.Walk.NewAvailable == nil || *last.Walk.NewAvailable < 1 {
		t.Error("the arrived task is neither served nor reported as waiting for a refresh")
	}
}

// A group cut at the decision lane's scan depth is a floor, and its category
// says so too. A client summing members must never present that sum as exact.
func TestALowerBoundGroupLeavesItsCategoryAFloor(t *testing.T) {
	t.Parallel()
	pairs := make([]crmcontracts.AttentionItem, 0, batchScanDepth)
	for i := range batchScanDepth {
		pairs = append(pairs, item(fmt.Sprintf("pair-%d", i), "dedupe_candidate"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: pairs}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	var group *crmcontracts.WorklistBatch
	for _, card := range got.Queue {
		if card.Batch != nil {
			group = card.Batch
		}
	}
	if group == nil || group.AtLeast == nil || !*group.AtLeast {
		t.Fatalf("a group read to the scan depth is not marked as a floor: %+v", group)
	}
	if !countFor(t, got, "decisions").MoreAvailable {
		t.Error("the category under a floor group reports an exact count")
	}
}
