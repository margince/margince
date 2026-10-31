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

// aWalkDay is tasks ahead of eight duplicate pairs, with three tag suggestions
// ranked behind the folded group so it is never the walk's last card.
func aWalkDay(tasks int) crmcontracts.Attention {
	day := aDayOfTasks(tasks)
	day.NeedsYou = aDayOfAlikeDecisions(8).NeedsYou
	for i := range 3 {
		day.NeedsYou = append(day.NeedsYou, item(fmt.Sprintf("tag-%d", i), "tag_suggestion"))
	}
	return day
}

// checkDecisionUnits fails a page whose decisions figure is not the items its
// decision cards stand for.
func checkDecisionUnits(t *testing.T, page crmcontracts.Worklist) {
	t.Helper()
	cards := []crmcontracts.WorklistItem{}
	for _, card := range page.Queue {
		if card.Category == "decisions" {
			cards = append(cards, card)
		}
	}
	if len(cards) == 0 {
		return
	}
	if shown := countFor(t, page, "decisions").Shown; shown != itemsOn(cards) {
		t.Errorf("the page counts %d decision items over cards standing for %d", shown, itemsOn(cards))
	}
}

// Eight pairs fold into one card, walked three cards a page: the group first,
// last on page one, last on page two, and mid-page. Work is resolved and one
// task arrives after page one.
func TestFoldedPairsReconcileWithTheDayAcrossAWalk(t *testing.T) {
	t.Parallel()
	for _, ahead := range []int{0, 2, 5, 10} {
		t.Run(fmt.Sprintf("%d tasks ahead", ahead), func(t *testing.T) {
			t.Parallel()
			reconcileAWalk(t, ahead)
		})
	}
}

func reconcileAWalk(t *testing.T, ahead int) {
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(&walkStore{})
	first := walkFrom(t, svc, aWalkDay(ahead), 3, worklistCursor{})
	started := consideredOf(first.Counts)
	if want := ahead + 11; started != want || first.Summary.Total != want {
		t.Fatalf("the day weighed %d items (summary %d), want %d", started, first.Summary.Total, want)
	}
	// The last tag suggestion is resolved before it is served. A pair is too,
	// where the group is not already on page one.
	moved := aWalkDay(ahead)
	moved.NeedsYou = moved.NeedsYou[:len(moved.NeedsYou)-1]
	if ahead >= 3 {
		moved.NeedsYou = moved.NeedsYou[1:]
	}
	moved.Planned = append(moved.Planned, item("task-arrived", "task", withDue(rankInstant)))

	served, seen, last := 0, map[string]int{}, first
	for page := first; ; {
		checkDecisionUnits(t, page)
		served += itemsOn(page.Queue)
		for _, card := range page.Queue {
			seen[card.Id]++
		}
		last = page
		// A walk that re-serves cards would never end, so the loop is capped.
		if page.NextCursor == nil || len(seen) > 40 {
			break
		}
		page = walkFrom(t, svc, moved, 3, decodedCursor(t, *page.NextCursor))
	}

	for id, times := range seen {
		if times != 1 {
			t.Errorf("card %q was served %d times on one walk", id, times)
		}
	}
	if seen["task-arrived"] > 0 {
		t.Error("work that arrived mid-walk was served on the walk")
	}
	if last.Walk == nil {
		t.Fatal("the last page carried no walk, so the departure cannot be reconciled")
	}
	if gone := last.Walk.ChangedSinceSnapshot; served+gone != started {
		t.Errorf("served %d items and %d left, which does not reconcile with the %d the walk started with",
			served, gone, started)
	}
	if arrived := last.Walk.NewAvailable; arrived == nil || *arrived != 1 {
		t.Errorf("one task arrived and the walk reports %v new items", arrived)
	}
}

// A group whose source holds more than was read is a floor, and its category
// says so too. A client summing members must never present that sum as exact.
func TestALowerBoundGroupLeavesItsCategoryAFloor(t *testing.T) {
	t.Parallel()
	pairs := make([]crmcontracts.AttentionItem, 0, batchScanDepth)
	for i := range batchScanDepth {
		pairs = append(pairs, item(fmt.Sprintf("pair-%d", i), "dedupe_candidate"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: pairs}
	day.Counts.DuplicatesOpen = new(batchScanDepth + 1)

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
