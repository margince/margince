// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Pinning a worklist row, against a real database.
//
// The promises here are the ones only Postgres can answer: the composite key
// that makes a pin name one row rather than one id, the upsert that makes a
// second pin the same success, and the table's own refusal of an unnamed row.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asColleague is the SECOND seat, for the case whose whole point is that a pin
// belongs to one person. Same grants and the same row scope — what differs is
// only who it is, which is what the pin is keyed on.
func asColleague(e *sendEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"person":   {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func pinStore(e *sendEnv) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// pinsNow reads what this reader has pinned, the way the assembler does.
func pinsNow(ctx context.Context, t *testing.T, e *sendEnv) map[WorklistRowRef]bool {
	t.Helper()
	var out map[WorklistRowRef]bool
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		got, err := pinStore(e).PinnedRows(ctx, tx)
		out = got
		return err
	}); err != nil {
		t.Fatalf("reading pins: %v", err)
	}
	return out
}

// The whole point: a row the rep pinned comes back as pinned.
func TestAPinnedRowIsReadBackAsPinned(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	row := WorklistRowRef{Source: "customer_waiting", RowID: ids.NewV7().String()}

	if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	if !pinsNow(ctx, t, e)[row] {
		t.Fatal("a row the rep pinned reads back unpinned")
	}
}

// A pin names a row by SOURCE and id together.
//
// The lanes mint ids independently, so the same underlying record's id can
// appear under two sources. Keyed on the id alone, pinning a waiting message
// would silently pin the task beside it — a row the reader never touched
// jumping to the top of their day with nothing to explain it.
func TestAPinNamesOneLanesRowAndNotAnotherWithTheSameID(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	shared := ids.NewV7().String()
	waiting := WorklistRowRef{Source: "customer_waiting", RowID: shared}
	task := WorklistRowRef{Source: "task", RowID: shared}

	if err := pinStore(e).PinWorklistRow(ctx, waiting); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	pinned := pinsNow(ctx, t, e)
	if !pinned[waiting] {
		t.Error("the pinned row reads back unpinned")
	}
	if pinned[task] {
		t.Error("pinning a waiting message also pinned the task sharing its id")
	}
}

// And unpinning names one lane's row too.
//
// The insert's key and the delete's key are two spellings of the same rule, and
// only the insert was held: a delete that forgot the source would unpin the
// task beside the waiting message, so a rep clearing one pin would silently
// lose another they had made deliberately.
func TestUnpinningOneLanesRowLeavesAnotherWithTheSameID(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	shared := ids.NewV7().String()
	waiting := WorklistRowRef{Source: "customer_waiting", RowID: shared}
	task := WorklistRowRef{Source: "task", RowID: shared}
	for _, row := range []WorklistRowRef{waiting, task} {
		if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
			t.Fatalf("pinning %+v: %v", row, err)
		}
	}

	if err := pinStore(e).UnpinWorklistRow(ctx, waiting); err != nil {
		t.Fatalf("unpinning: %v", err)
	}

	pinned := pinsNow(ctx, t, e)
	if pinned[waiting] {
		t.Error("the unpinned row is still pinned")
	}
	if !pinned[task] {
		t.Error("unpinning a waiting message also unpinned the task sharing its id")
	}
}

// It binds ONE reader. Pinning reorders the caller's own morning, and applying
// it to a colleague would reorder a day whose owner never asked.
func TestAPinBindsOnlyTheReaderWhoMadeIt(t *testing.T) {
	e := setupSend(t)
	row := WorklistRowRef{Source: "task", RowID: ids.NewV7().String()}
	if err := pinStore(e).PinWorklistRow(e.as(principal.RowScopeAll), row); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	if pinsNow(asColleague(e), t, e)[row] {
		t.Fatal("one rep's pin reordered a colleague's day")
	}
}

// Unpinning gives the row back to the ranking.
func TestUnpinningReturnsTheRowToTheRanking(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	row := WorklistRowRef{Source: "task", RowID: ids.NewV7().String()}
	if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	if err := pinStore(e).UnpinWorklistRow(ctx, row); err != nil {
		t.Fatalf("unpinning: %v", err)
	}

	if pinsNow(ctx, t, e)[row] {
		t.Fatal("an unpinned row still leads the day")
	}
}

// Pinning twice is the same success, so a double-click is not an error.
func TestPinningTheSameRowAgainSucceeds(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	row := WorklistRowRef{Source: "task", RowID: ids.NewV7().String()}

	for range 2 {
		if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
			t.Fatalf("pinning: %v", err)
		}
	}

	if !pinsNow(ctx, t, e)[row] {
		t.Fatal("the row is not pinned after being pinned twice")
	}
}

// Pinning twice audits ONCE.
//
// The insert is ON CONFLICT DO NOTHING, so the second call changes nothing —
// but it went on writing an audit row anyway, which reads to somebody counting
// how often a rep overrides the order as a rep who double-clicked being twice
// as opinionated. The unpin path always made this test; the pin path did not.
func TestPinningTwiceRecordsOneJudgement(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	row := WorklistRowRef{Source: "task", RowID: ids.NewV7().String()}

	for range 2 {
		if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
			t.Fatalf("pinning: %v", err)
		}
	}

	if got := pinAudits(t, e, row); got != 1 {
		t.Fatalf("two identical pins wrote %d audit rows, want the one decision", got)
	}
}

// pinAudits counts the trail entries naming one pinned row.
//
// Matched on the IMAGE rather than the entity id, because a folded group's id
// is synthetic and reaches the audit row as the nil uuid — so the subject alone
// cannot tell two groups apart, which is why the image carries the row id.
func pinAudits(t *testing.T, e *sendEnv, row WorklistRowRef) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'worklist_pin'
		   AND after->>'row_id' = $1 AND after->>'source' = $2`,
		row.RowID, row.Source).Scan(&n); err != nil {
		t.Fatalf("counting pin audits: %v", err)
	}
	return n
}

// The trail names WHICH row was pinned, which the subject cannot always carry.
func TestThePinTrailNamesTheRowAndTheReader(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	// A synthetic id, the shape a folded group carries: not a uuid, so the
	// audit subject is the nil id and the image is the only thing that can say
	// which group this was.
	row := WorklistRowRef{Source: "batch", RowID: "uncertain_contact"}
	if err := pinStore(e).PinWorklistRow(ctx, row); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	if got := pinAudits(t, e, row); got != 1 {
		t.Fatalf("the trail holds %d entries naming this group, want the one", got)
	}
}

// A source the queue does not produce is refused rather than stored.
//
// The table takes any string otherwise: junk that matches no row, sits there
// because only its author can remove it, and is read on every assembly of that
// reader's page.
func TestASourceTheQueueDoesNotProduceIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)

	if err := pinStore(e).PinWorklistRow(ctx,
		WorklistRowRef{Source: "not_a_lane", RowID: ids.NewV7().String()}); err == nil {
		t.Error("a pin naming a lane this queue does not produce was accepted")
	}
	// And a real lane IS accepted, without which the refusal above would pass
	// against a check that refused everything.
	if err := pinStore(e).PinWorklistRow(ctx,
		WorklistRowRef{Source: "batch", RowID: "uncertain_contact"}); err != nil {
		t.Fatalf("a folded group's own source was refused: %v", err)
	}
}

// And unpinning a row nobody pinned is the same success: the reader's goal
// state already holds.
func TestUnpinningARowNobodyPinnedSucceeds(t *testing.T) {
	e := setupSend(t)

	if err := pinStore(e).UnpinWorklistRow(e.as(principal.RowScopeAll),
		WorklistRowRef{Source: "task", RowID: ids.NewV7().String()}); err != nil {
		t.Fatalf("unpinning a row that was never pinned: %v", err)
	}
}

// A row with no name is refused rather than stored.
//
// An empty source or id matches no row, so the pin would sit in the table
// forever and the read could not tell it from a pin whose row is simply not on
// today's page.
func TestAnUnnamedRowIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)

	for _, row := range []WorklistRowRef{
		{Source: "", RowID: ids.NewV7().String()},
		{Source: "task", RowID: ""},
	} {
		if err := pinStore(e).PinWorklistRow(ctx, row); err == nil {
			t.Errorf("a pin naming %+v was accepted", row)
		}
	}
}

// The database refuses it too, which is what holds the rule when a future
// caller reaches the table without going through the store.
func TestTheDatabaseRefusesAnUnnamedPin(t *testing.T) {
	e := setupSend(t)

	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO worklist_pin (reader_id, source, row_id, set_by)
		VALUES ($1, '', 'a-row', 'test')`, e.rep); err == nil {
		t.Error("a pin with no source was stored")
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO worklist_pin (reader_id, source, row_id, set_by)
		VALUES ($1, 'task', '', 'test')`, e.rep); err == nil {
		t.Error("a pin with no row id was stored")
	}
	// And the honest row IS accepted, without which the refusals above would
	// pass against a constraint that refused everything.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO worklist_pin (reader_id, source, row_id, set_by)
		VALUES ($1, 'task', 'a-row', 'test')`, e.rep); err != nil {
		t.Fatalf("a named pin was refused: %v", err)
	}
}
