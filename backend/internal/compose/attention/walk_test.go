// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a frozen walk promises, and the two things it deliberately does not.
//
// The promise: the rows a reader started with are the rows they finish with, in
// the order they started in, however the day moves behind them. The two
// exceptions are both reported rather than hidden — work that LEAVES goes
// immediately, because a frozen headline over rows somebody can no longer see
// would be steadier and false, and work that ARRIVES waits for a refresh.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAWalkKeepsItsRowsWhileTheDayMovesUnderIt is the acceptance case.
//
// A day of eight rows, walked two pages deep, with work inserted and the
// ranking disturbed in between. The walk returns its original eight exactly
// once and in its original order — which the unfrozen pager cannot promise,
// and says so in its own contract.
func TestAWalkKeepsItsRowsWhileTheDayMovesUnderIt(t *testing.T) {
	t.Parallel()
	walks := &walkStore{}
	started := rankInstant
	svc := (&Service{now: func() time.Time { return started }}).WithWalks(walks)
	day := aDayOfTasks(8)

	first := walkFrom(t, svc, day, 3, worklistCursor{})
	if first.NextCursor == nil {
		t.Fatal("a day of eight rows paged three at a time minted no cursor")
	}
	if first.Walk == nil {
		t.Fatal("the first page froze no walk, so there is nothing to hold still")
	}

	// The day moves, and the RANKING with it: every surviving task's deadline is
	// reversed, so a re-rank would return the same eight rows back to front.
	// Without that the fixture cannot tell a frozen order from a fresh one — a
	// day whose ranking is stable ranks the same either way, and a walk that
	// re-sorted its survivors passed this test until the deadlines moved.
	moved := aDayReordered(8)
	moved.Planned = append(moved.Planned,
		item("z-newcomer", "task", withDue(rankInstant.Add(-time.Hour))))

	seen := map[string]int{}
	order := []string{}
	for _, row := range first.Queue {
		seen[row.Id]++
		order = append(order, row.Id)
	}
	cursor := decodedCursor(t, *first.NextCursor)
	for page := 0; page < 4 && cursor.At > 0; page++ {
		next := walkFrom(t, svc, moved, 3, cursor)
		for _, row := range next.Queue {
			seen[row.Id]++
			order = append(order, row.Id)
		}
		if next.NextCursor == nil {
			break
		}
		cursor = decodedCursor(t, *next.NextCursor)
	}

	if len(seen) != 8 {
		t.Errorf("the walk covered %d distinct rows, want the eight it started with", len(seen))
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("row %q was served %d times on one walk", id, times)
		}
	}
	if _, arrived := seen["z-newcomer"]; arrived {
		t.Error("work that arrived mid-walk joined it, so the reader's day grew under them")
	}
	// And in the SEQUENCE it started in, which is the promise the row set alone
	// does not carry: a walk that re-ranked its survivors would return the same
	// eight rows in an order the reader had not been shown.
	want := frozenOrderOf(svc)
	if len(order) != len(want) {
		t.Fatalf("the walk served %d rows, want the %d it froze", len(order), len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("the walk served %v, want the order it froze: %v", order, want)
		}
	}
}

// frozenOrderOf is the sequence a first page over this day would freeze.
//
// Read from the store rather than restated here, so the assertion is about the
// walk KEEPING its order rather than about a list somebody typed twice.
func frozenOrderOf(svc *Service) []string {
	store, held := svc.walks.(*walkStore)
	if !held {
		return nil
	}
	for _, walk := range store.kept {
		out := make([]string, 0, len(walk.Rows))
		for _, at := range walk.Rows {
			out = append(out, at.RowID)
		}
		return out
	}
	return nil
}

// TestWorkThatArrivesWaitsForARefreshAndSaysSo.
//
// New rows must not join a walk in progress — that is what keeps the headline
// still — but the reader has to be told there is something to refresh for, or
// the queue silently withholds work it knows about.
func TestWorkThatArrivesWaitsForARefreshAndSaysSo(t *testing.T) {
	t.Parallel()
	walks := &walkStore{}
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(walks)

	first := walkFrom(t, svc, aDayOfTasks(4), 2, worklistCursor{})
	if first.Walk == nil || first.NextCursor == nil {
		t.Fatal("the first page froze no walk to resume")
	}
	// A first page cannot have work behind it, so it says nothing about any.
	if first.Walk.NewAvailable != nil {
		t.Errorf("a first page reported %d rows waiting, before anything could arrive",
			*first.Walk.NewAvailable)
	}

	busier := aDayOfTasks(4)
	busier.Planned = append(busier.Planned,
		item("newcomer-one", "task"), item("newcomer-two", "task"))

	next := walkFrom(t, svc, busier, 2, decodedCursor(t, *first.NextCursor))

	if next.Walk == nil || next.Walk.NewAvailable == nil {
		t.Fatal("a resumed page said nothing about work waiting behind the walk")
	}
	if *next.Walk.NewAvailable != 2 {
		t.Errorf("the page reports %d rows waiting, want the two that arrived",
			*next.Walk.NewAvailable)
	}
}

// TestWorkThatLeavesGoesImmediatelyAndIsCounted.
//
// The one direction membership moves the other way. A row that was answered,
// deleted or withdrawn is not served from the walk — it no longer exists to
// serve — and the count that falls is explained rather than left to move.
func TestWorkThatLeavesGoesImmediatelyAndIsCounted(t *testing.T) {
	t.Parallel()
	walks := &walkStore{}
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(walks)

	first := walkFrom(t, svc, aDayOfTasks(6), 2, worklistCursor{})
	if first.NextCursor == nil {
		t.Fatal("the first page minted no cursor")
	}
	// Two of the six are dealt with between pages.
	thinner := aDayOfTasks(6)
	thinner.Planned = thinner.Planned[:4]

	next := walkFrom(t, svc, thinner, 10, decodedCursor(t, *first.NextCursor))

	if next.Walk == nil {
		t.Fatal("a resumed page carried no walk state")
	}
	if next.Walk.ChangedSinceSnapshot != 2 {
		t.Errorf("the page reports %d rows gone, want the two that were dealt with",
			next.Walk.ChangedSinceSnapshot)
	}
	for _, row := range next.Queue {
		if row.Id == "task-4" || row.Id == "task-5" {
			t.Errorf("row %q was served from the walk after it left the day", row.Id)
		}
	}
}

// TestAWalkThatCannotBeResumedIsRefusedRatherThanRestarted.
//
// TWO WRONG ANSWERS were available and the second is the dangerous one. Serving
// a fresh page under a resumed token hands the reader rows they have already
// seen, in a new order, with nothing saying the walk they were on had ended.
// Serving an EMPTY page is worse: on this queue an empty day means the work is
// done, so an expired walk would tell a rep their morning was clear.
//
// So it refuses, with the code a cursor minted under a different question
// already earns — the remedy is the same one, ask again without the cursor.
func TestAWalkThatCannotBeResumedIsRefusedRatherThanRestarted(t *testing.T) {
	t.Parallel()
	svc := (&Service{now: func() time.Time { return rankInstant }}).
		WithWalks(&walkStore{refuse: true})
	cursor := worklistCursor{
		At: 2, Params: fingerprint(scopeAll, "", ids.UUID{}), Snapshot: ids.NewV7(),
	}

	walk, walking, err := svc.walkNamedBy(context.Background(), cursor, scopeAll, "", ids.UUID{})

	if err == nil {
		t.Fatalf("a walk that cannot be resumed was allowed through as %v — the page that "+
			"follows is either an empty day or rows the reader has already seen", walk)
	}
	if walking {
		t.Error("a refused walk still reported itself as one to page, so the projection " +
			"would resume into a snapshot the store declined to hand over")
	}
	var mismatch *storekit.CursorSortMismatchError
	if !errors.As(err, &mismatch) {
		t.Errorf("a refused walk answered %T, want the refusal a stale cursor earns so the "+
			"client re-issues without it", err)
	}
}

// TestWithoutAStoreTheQueuePagesTheWayItAlwaysDid.
//
// An installation that wires no snapshot store gets the offset pager, not a
// degraded version of something else — and no walk on the response, because
// there is none to describe.
func TestWithoutAStoreTheQueuePagesTheWayItAlwaysDid(t *testing.T) {
	t.Parallel()
	svc := &Service{now: func() time.Time { return rankInstant }}

	out := walkFrom(t, svc, aDayOfTasks(6), 2, worklistCursor{})

	if len(out.Queue) != 2 {
		t.Errorf("an unwired queue drew %d rows, want the page it was asked for", len(out.Queue))
	}
	if out.Walk != nil {
		t.Error("an unwired queue reported a walk it never froze")
	}
	if out.NextCursor == nil {
		t.Error("an unwired queue minted no cursor, so its walk cannot continue at all")
	}
}

// walkFrom pages a day the way the endpoint does: resolve the walk the cursor
// names FIRST, then project.
//
// The production path splits those two steps — a token that cannot be continued
// is refused before a dozen lanes are read — so a test that called the
// projection alone would exercise a service that never resumed anything.
func walkFrom(
	t *testing.T, svc *Service, day crmcontracts.Attention, limit int, cursor worklistCursor,
) crmcontracts.Worklist {
	t.Helper()
	walk, walking, err := svc.walkNamedBy(context.Background(), cursor, scopeAll, "", ids.UUID{})
	if err != nil {
		t.Fatalf("resolving the walk this cursor names: %v", err)
	}
	return svc.readingWalk(walk, walking).worklistFrom(context.Background(), day, scopeAll, "", limit,
		waitingRead{}, leadRead{}, cursor, nil)
}

// decodedCursor reads a minted token back the way the handler does.
//
// Fixed to the unnarrowed question, which is the only one these walks ask. A
// cursor carried onto a different scope or filter is refused by design, and the
// test that proves that refusal builds its own token rather than coming through
// here — so a parameter for either would only ever receive one value.
func decodedCursor(t *testing.T, token string) worklistCursor {
	t.Helper()
	cursor, err := decodeCursor(token, scopeAll, "", ids.UUID{})
	if err != nil {
		t.Fatalf("decoding the cursor this server just minted: %v", err)
	}
	return cursor
}

// aDayReordered is the same n tasks with their deadlines reversed, so the
// comparator puts them in the opposite sequence.
//
// The fixture a frozen walk needs: over a day that ranks the same on every read
// there is nothing for freezing to protect, and a test built on one cannot see
// the difference it exists to prove.
func aDayReordered(n int) crmcontracts.Attention {
	tasks := make([]crmcontracts.AttentionItem, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, item(
			"task-"+string(rune('0'+i)), "task",
			withDue(rankInstant.Add(time.Duration(n-i)*time.Hour))))
	}
	return crmcontracts.Attention{AsOf: rankInstant, Planned: tasks}
}

// aDayOfTasks is n agreed tasks, each with its own deadline so the ranking has
// something to order by and the sequence is stable across reads.
func aDayOfTasks(n int) crmcontracts.Attention {
	tasks := make([]crmcontracts.AttentionItem, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, item(
			"task-"+string(rune('0'+i)), "task",
			withDue(rankInstant.Add(time.Duration(i)*time.Hour))))
	}
	return crmcontracts.Attention{AsOf: rankInstant, Planned: tasks}
}

// walkStore is the seam, in memory.
//
// A real store would prove the same behaviour and cost a database; what these
// tests are about is the ASSEMBLER's use of a walk — which rows it freezes,
// which it serves, what it reports — and the store's own refusals are held
// against real Postgres in worklistsnap_integration_test.go.
type walkStore struct {
	kept   map[ids.UUID]worklistsnap.Snapshot
	refuse bool
}

func (w *walkStore) Freeze(
	_ context.Context, _ string, asOf time.Time,
	buckets worklistsnap.Buckets, rows []worklistsnap.Row,
) (ids.UUID, error) {
	if w.kept == nil {
		w.kept = map[ids.UUID]worklistsnap.Snapshot{}
	}
	id := ids.NewV7()
	w.kept[id] = worklistsnap.Snapshot{ID: id, AsOf: asOf, Buckets: buckets, Rows: rows}
	return id, nil
}

func (w *walkStore) Resume(
	_ context.Context, id ids.UUID, _ string,
) (worklistsnap.Snapshot, error) {
	if w.refuse {
		return worklistsnap.Snapshot{}, apperrors.ErrNotFound
	}
	walk, held := w.kept[id]
	if !held {
		return worklistsnap.Snapshot{}, apperrors.ErrNotFound
	}
	return walk, nil
}

// TestAWalkDoesNotSkipRowsWhenEarlierOnesAreDealtWith is the case the offset
// resume gets wrong if it counts positions in a SHRINKING list.
//
// Page one covers the first three survivors and stops at offset three. Two of
// those three are then answered. If offset three is read against the shortened
// list it lands two rows further on than the reader ever got, and the rows in
// between are skipped — silently, on a queue whose whole purpose is that work is
// not forgotten.
func TestAWalkDoesNotSkipRowsWhenEarlierOnesAreDealtWith(t *testing.T) {
	t.Parallel()
	walks := &walkStore{}
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(walks)

	first := walkFrom(t, svc, aDayOfTasks(8), 3, worklistCursor{})
	if len(first.Queue) != 3 || first.NextCursor == nil {
		t.Fatalf("the first page drew %d rows, want three and a cursor", len(first.Queue))
	}
	served := map[string]bool{}
	for _, row := range first.Queue {
		served[row.Id] = true
	}

	// Two rows the reader has ALREADY seen are answered between pages.
	thinner := aDayOfTasks(8)
	kept := make([]crmcontracts.AttentionItem, 0, 6)
	for _, task := range thinner.Planned {
		if task.Id == "task-0" || task.Id == "task-1" {
			continue
		}
		kept = append(kept, task)
	}
	thinner.Planned = kept

	next := walkFrom(t, svc, thinner, 10, decodedCursor(t, *first.NextCursor))
	for _, row := range next.Queue {
		served[row.Id] = true
	}

	// Every row of the walk that still exists must have been served on one of
	// the two pages. task-0 and task-1 are gone and are not owed.
	for _, want := range []string{"task-2", "task-3", "task-4", "task-5", "task-6", "task-7"} {
		if !served[want] {
			t.Errorf("row %q was never served: the walk skipped it when earlier rows "+
				"were dealt with between pages", want)
		}
	}
}

// TestAFoldedGroupLosingOneMemberDoesNotLoseTheOthers.
//
// A batch is a SYNTHETIC row: its id is minted from the group's key and cause,
// so it exists only while the fold produces it. Drop one member below the fold
// floor and the group stops being minted — and if the walk compares frozen
// identities against a live set that refolded, the whole group reads as gone
// while its still-unresolved members read as newly arrived. Real work a reader
// was walking would drop out of the walk until they refreshed.
func TestAFoldedGroupLosingOneMemberDoesNotLoseTheOthers(t *testing.T) {
	t.Parallel()
	walks := &walkStore{}
	svc := (&Service{now: func() time.Time { return rankInstant }}).WithWalks(walks)

	// A task ahead of the group, so page one is the task and the group waits
	// behind the cursor — which is what makes the group's fate on page two the
	// thing this test observes.
	whole := aDayOfAlikeDecisions(batchFloor)
	whole.Planned = []crmcontracts.AttentionItem{item("task-ahead", "task", withDue(rankInstant))}

	first := walkFrom(t, svc, whole, 1, worklistCursor{})
	if first.NextCursor == nil {
		t.Fatal("the fixture did not page, so there is no resume to judge")
	}
	// The group's MEMBERS are in the frozen walk, not the group's own synthetic
	// id. That is the fix this test drove: a group frozen by its own id vanishes
	// when the fold stops producing it. The members are what persist, so they
	// are what a walk holds.
	frozen := frozenOrderOf(svc)
	members := 0
	for _, id := range frozen {
		if id == "duplicates" {
			t.Fatalf("the walk froze the group's synthetic id: %v — it exists only while the "+
				"fold produces it, so the walk loses the whole group when one member goes", frozen)
		}
		if strings.HasPrefix(id, "pair-") {
			members++
		}
	}
	if members < batchFloor {
		t.Fatalf("the walk froze %v, which holds %d of the group's members: this test cannot "+
			"see the refold without them", frozen, members)
	}

	// One member is dealt with, dropping the group below its floor.
	thinner := aDayOfAlikeDecisions(batchFloor - 1)
	thinner.Planned = whole.Planned

	next := walkFrom(t, svc, thinner, 10, decodedCursor(t, *first.NextCursor))

	if next.Walk == nil {
		t.Fatal("the resumed page carried no walk")
	}
	// The surviving members are unresolved work the reader was walking. They
	// must not be reported as arrivals waiting behind a refresh.
	if next.Walk.NewAvailable != nil && *next.Walk.NewAvailable >= batchFloor-1 {
		t.Errorf("the walk reports %d rows newly arrived after one member of a folded group "+
			"was dealt with — the group's surviving members are work the reader was already "+
			"walking, not new work", *next.Walk.NewAvailable)
	}
}

// aDayOfAlikeDecisions is n routine duplicate pairs, which fold into one group
// once they reach the floor.
func aDayOfAlikeDecisions(n int) crmcontracts.Attention {
	pairs := make([]crmcontracts.AttentionItem, 0, n)
	for i := 0; i < n; i++ {
		pairs = append(pairs, item(fmt.Sprintf("pair-%d", i), "dedupe_candidate"))
	}
	return crmcontracts.Attention{AsOf: rankInstant, NeedsYou: pairs}
}
