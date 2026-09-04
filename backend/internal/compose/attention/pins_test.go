// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A pinned row escapes the fold that would have hidden it in a group.
//
// The two rules meet in the order they run: pins are applied before the routine
// decisions are folded, and folding matches on `Level == levelRoutine`. So a
// pinned approval is no longer routine and stays its own row — which is what a
// reader who pinned it asked for, and the opposite of what folding it would
// give them.
//
// Worth holding rather than leaving to the order, because the order is easy to
// change and the failure is quiet: the pin would still be stored, the row would
// still be raised, and it would then be folded into a group with no way for the
// reader to see the row they pinned.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAPinnedDecisionStaysItsOwnRowRatherThanFolding(t *testing.T) {
	t.Parallel()

	needs := make([]crmcontracts.AttentionItem, 0, 12)
	for i := range 12 {
		needs = append(needs, item(string(rune('a'+i)), "approval", withKind("capture_counterparty")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: needs}
	rows := classifyDay(day, rankInstant, dayMoney{})

	// One of the twelve, pinned — then folded the way the assembler folds.
	pinned := applyPins(rows, map[RowRef]bool{{Source: "approval", RowID: "a"}: true})
	folded := foldRoutineDecisionsBounded(pinned, false)

	var stoodAlone bool
	for _, row := range folded {
		if row.item.Id == "a" {
			stoodAlone = true
		}
	}
	if !stoodAlone {
		t.Fatal("the pinned decision was folded into a group, so the reader cannot " +
			"see the row they asked to lead their day")
	}
	// And the other eleven DID fold, without which this would pass against a
	// fold that had stopped working.
	if len(folded) >= len(rows) {
		t.Fatalf("nothing folded: %d rows in, %d out", len(rows), len(folded))
	}
}

// A pin says WHY the row is where it is.
//
// The comparator explains a row against the one below it, so a page whose only
// row is pinned — or two adjacent pinned rows — has no comparison to draw. The
// reason is what makes the one row a reader definitely knows the cause of the
// one row the page can also account for.
func TestAPinnedRowSaysItWasPinned(t *testing.T) {
	t.Parallel()

	rows := applyPins([]ranked{candidate("a", levelRoutine)},
		map[RowRef]bool{{RowID: "a"}: true})

	var said bool
	for _, r := range rows[0].item.Because {
		if r.Kind == "pinned" {
			said = true
		}
	}
	if !said {
		t.Fatalf("a pinned row explains itself as %+v, with no pin among them",
			rows[0].item.Because)
	}
}

// Pinning does not make a row urgent.
//
// The summary's `urgent` is "somebody is waiting, or a promise is breaking",
// and a rep pinning a piece of hygiene to the top of their own morning has made
// neither true. The pin moves where the row SORTS; reading one field for both
// questions let one reader's preference move a figure their manager reads.
func TestPinningARoutineRowDoesNotMakeItUrgent(t *testing.T) {
	t.Parallel()

	rows := applyPins([]ranked{candidate("hygiene", levelRoutine)},
		map[RowRef]bool{{RowID: "hygiene"}: true})

	got := summarize(rows, materialBar{})
	if got.Urgent != 0 {
		t.Errorf("a pinned routine row counted %d urgent", got.Urgent)
	}
	if got.LowerPriority != 1 {
		t.Errorf("it counted %d as routine, want the one it still is", got.LowerPriority)
	}
	// And it DID move: the level the ranking reads is the pin level.
	if rows[0].item.Level != levelPinned {
		t.Errorf("the row sorts at level %d, want the pin level", rows[0].item.Level)
	}
}

// Pinning an urgent row leaves it urgent, without which the case above would
// pass against a summary that had stopped counting urgency at all.
func TestPinningAWaitingRowLeavesItUrgent(t *testing.T) {
	t.Parallel()

	rows := applyPins([]ranked{candidate("waiting", levelWaiting)},
		map[RowRef]bool{{RowID: "waiting"}: true})

	if got := summarize(rows, materialBar{}); got.Urgent != 1 {
		t.Errorf("a pinned waiting customer counted %d urgent, want the one", got.Urgent)
	}
}

// pinsFor is a seam answering one reader's pins, keyed by whoever asks.
type pinsFor map[string]map[RowRef]bool

func (p pinsFor) PinnedRows(ctx context.Context) (map[RowRef]bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	return p[actor.ID], nil
}

func readerNamed(id string) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: id, UserID: ids.NewV7(),
	})
}

// One reader's pins never reach another's page.
//
// The whole point of readingPins returning a COPY: a single Service serves
// every request, so a field set on it would follow one reader's page onto the
// next. The failure would be a rep seeing a colleague's row at the top of their
// own day with nothing to explain it.
//
// Asserted through ONE service asked twice, which is the shape production has.
// A test that read each reader's pins straight from the store would pass over a
// service that leaked, because the leak is not in the store.
func TestOneReadersPinsDoNotReachAnothersPage(t *testing.T) {
	t.Parallel()

	shared := &Service{pins: pinsFor{
		"human:first": {{Source: "task", RowID: "theirs"}: true},
	}}

	first, err := shared.readingPins(readerNamed("human:first"))
	if err != nil {
		t.Fatalf("reading the first reader's pins: %v", err)
	}
	second, err := shared.readingPins(readerNamed("human:second"))
	if err != nil {
		t.Fatalf("reading the second reader's pins: %v", err)
	}

	if !first.pinned[RowRef{Source: "task", RowID: "theirs"}] {
		t.Error("the reader who pinned the row does not have it")
	}
	if len(second.pinned) != 0 {
		t.Errorf("a colleague's page carries %d of somebody else's pins", len(second.pinned))
	}
	// And the SHARED service still holds none, which is what makes the copy a
	// copy rather than a reset that happens to run in order.
	if len(shared.pinned) != 0 {
		t.Errorf("the shared service kept %d pins between requests", len(shared.pinned))
	}
}

// A refused pin read leaves the reader their day.
//
// Every lane in this feed omits on refusal rather than failing the page, and a
// pin store gating on a grant the worklist never required must not be the one
// thing that 403s a reader who could previously receive a partial page.
func TestARefusedPinReadStillDrawsTheDay(t *testing.T) {
	t.Parallel()

	// No actor, which is what the seam above refuses on.
	svc := &Service{pins: pinsFor{}}
	reading, err := svc.readingPins(context.Background())
	if err != nil {
		t.Fatalf("a refused pin read took the whole page: %v", err)
	}
	if len(reading.pinned) != 0 {
		t.Errorf("a refused read produced %d pins", len(reading.pinned))
	}
}

// A FOLDED GROUP can be pinned, which is what the contract promises and what
// one pass could not deliver.
//
// The group's row is MINTED by the fold: its id is synthetic and did not exist
// when the first pass ran, so a pin on it matched nothing. Both passes are
// needed and neither replaces the other — the first is what lets a pinned
// MEMBER escape being folded at all, and the second is what lets the group
// itself lead the page.
//
// Driven through worklistFrom rather than through applyPins, because what is
// under test is WHERE the assembler applies pins. A test calling applyPins on
// an already-folded slice passes over an assembler that never runs the second
// pass at all — which is what the first version of this test did.
func TestAFoldedGroupCanBePinned(t *testing.T) {
	t.Parallel()

	needs := make([]crmcontracts.AttentionItem, 0, 12)
	for i := range 12 {
		needs = append(needs, item(string(rune('a'+i)), "approval", withKind("capture_counterparty")))
	}
	// A row that outranks the group by a long way, so the group leads only if
	// the pin actually moved it. Without this the fixture holds one row and the
	// assertion passes whatever the pin did.
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: needs,
		Meetings: lane(item("meeting", "meeting", withDue(rankInstant.Add(30*time.Minute)))),
	}

	// The group's id, learned from an unpinned assembly of the same day.
	plain := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 50,
		waitingRead{}, leadRead{}, worklistCursor{}, nil)
	var groupID string
	for _, row := range plain.Queue {
		if row.Source == "batch" {
			groupID = row.Id
		}
	}
	if groupID == "" {
		t.Fatal("nothing folded, so this fixture cannot test a group's pin")
	}
	// And the group does NOT lead on its own, which is what makes the assertion
	// below about the pin rather than about the fixture.
	if plain.Queue[0].Id == groupID {
		t.Fatal("the group already leads unpinned, so this fixture proves nothing")
	}

	svc := &Service{pinned: map[RowRef]bool{{Source: "batch", RowID: groupID}: true}}
	out := svc.worklistFrom(t.Context(), day, scopeAll, "", 50,
		waitingRead{}, leadRead{}, worklistCursor{}, nil)

	if len(out.Queue) == 0 {
		t.Fatal("the pinned day drew no rows")
	}
	if out.Queue[0].Id != groupID {
		t.Fatalf("the page leads with %q, want the pinned group %q — its id is "+
			"minted by the fold, so a pin applied only before it matches nothing",
			out.Queue[0].Id, groupID)
	}
}
