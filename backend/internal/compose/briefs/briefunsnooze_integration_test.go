// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// Taking a snooze back.
//
// The toast that follows a snooze has always offered an undo, and there was no
// writer behind it: a rep who set aside the wrong row, or chose "until the
// meeting" when they meant "until tomorrow", waited for the condition to lift.
//
// All instants are injected, like the snooze suite beside this one. Nothing here
// reads the wall clock.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestTakingBackASnoozeReturnsTheItemToTheQueue(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	snoozeAt := briefClock.Add(2 * time.Hour)
	until := briefClock.Add(48 * time.Hour)

	if _, err := b.engine.MarkSnoozed(
		b.repCtx, item.ID, values.ReopenOnTime, &until, nil, snoozeAt,
	); err != nil {
		t.Fatal(err)
	}

	// ANOTHER REP CANNOT TAKE IT BACK, and learns nothing by trying: the same
	// existence-hiding not-found every row-scope miss answers.
	rep2 := b.As(b.Rep2, []ids.UUID{b.Team1}, integration.AdminPerms)
	if _, err := b.engine.MarkUnsnoozed(rep2, item.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("foreign unsnooze → %v, want ErrNotFound (existence-hiding)", err)
	}

	back, err := b.engine.MarkUnsnoozed(b.repCtx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.State != briefStateNew {
		t.Errorf("state after taking back the snooze = %q, want %q — a snooze can only "+
			"be set on an actionable item, so new is where it came from",
			back.State, briefStateNew)
	}
	// ALL THREE COLUMNS, not just the state. The CHECK pairs them with the
	// snoozed state, so a row left holding a reopen condition while back in the
	// queue describes a snooze on an item nobody snoozed.
	if back.SnoozedUntil != nil || back.ReopenOn != "" || back.ReopenRef != nil {
		t.Errorf("item still carries its snooze: until=%v on=%q ref=%v",
			back.SnoozedUntil, back.ReopenOn, back.ReopenRef)
	}

	// Asserted against the ROW, not only the returned value: a writer that
	// updated its in-memory copy and not the database would satisfy every
	// assertion above and leave the rep's queue exactly as it was.
	var state string
	var storedUntil *time.Time
	var storedOn *string
	if err := owner.QueryRow(context.Background(),
		`SELECT state, snoozed_until, reopen_on FROM brief_item WHERE id = $1`, item.ID).
		Scan(&state, &storedUntil, &storedOn); err != nil {
		t.Fatal(err)
	}
	if state != briefStateNew || storedUntil != nil || storedOn != nil {
		t.Errorf("stored row = state %q until %v on %v, want %q with both cleared",
			state, storedUntil, storedOn, briefStateNew)
	}

	// And it is markable again, which is what "back in the queue" MEANS. A row
	// reading `new` that no verb would accept is not a returned item.
	if _, err := b.engine.MarkActed(b.repCtx, item.ID, snoozeAt.Add(2*time.Minute)); err != nil {
		t.Errorf("acting on the returned item: %v — an item back in the queue takes "+
			"the queue's verbs", err)
	}
}

// TAKING BACK WHAT WAS NOT SET ASIDE IS A CONFLICT, never a quiet success.
//
// Answering 200 would tell a rep their click landed on a state that had already
// moved under them — the same reasoning markItem applies to a second mark. The
// three states are tried separately because they reach the guard by different
// routes: `new` never was snoozed, and acted/dismissed left the snooze behind.
func TestTakingBackASnoozeThatIsNotThereIsRefused(t *testing.T) {
	b := setupBrief(t)
	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	at := briefClock.Add(time.Hour)

	// Fresh: nothing was set aside.
	if _, err := b.engine.MarkUnsnoozed(b.repCtx, run.Items[0].ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("unsnooze of a fresh item → %v, want ErrConflict", err)
	}

	// Acted on: a decision about the deal, not a snooze, and not this verb's
	// to reverse.
	acted := run.Items[1].ID
	if _, err := b.engine.MarkActed(b.repCtx, acted, at); err != nil {
		t.Fatal(err)
	}
	if _, err := b.engine.MarkUnsnoozed(b.repCtx, acted); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("unsnooze of an acted item → %v, want ErrConflict", err)
	}
}

// A SECOND TAKE-BACK IS REFUSED TOO, which is what makes the control safe to
// press twice.
//
// The first one returns the item to the queue; the second finds nothing set
// aside and says so, rather than stamping a fresh state_at over a row a rep has
// since been working from.
func TestTakingBackASnoozeTwiceRefusesTheSecond(t *testing.T) {
	b := setupBrief(t)
	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	until := briefClock.Add(48 * time.Hour)
	at := briefClock.Add(time.Hour)

	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnTime, &until, nil, at); err != nil {
		t.Fatal(err)
	}
	if _, err := b.engine.MarkUnsnoozed(b.repCtx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.engine.MarkUnsnoozed(b.repCtx, item.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("second unsnooze → %v, want ErrConflict", err)
	}
}

// A SNOOZE WHOSE CONDITION HAS ALREADY LIFTED IS STILL TAKEN BACK.
//
// The row says snoozed until a read re-surfaces it, and the write is the same
// either way. Asking whether the condition had lifted would make the answer
// depend on whether a read had happened to run — a rep pressing undo on a
// visible toast would get a conflict for a reason nothing on their screen shows.
func TestTakingBackAnExpiredSnoozeStillReturnsTheItem(t *testing.T) {
	b := setupBrief(t)
	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	until := briefClock.Add(2 * time.Hour)

	if _, err := b.engine.MarkSnoozed(
		b.repCtx, item.ID, values.ReopenOnTime, &until, nil, briefClock,
	); err != nil {
		t.Fatal(err)
	}
	// Well past the moment it was waiting for, and no read has run in between.
	back, err := b.engine.MarkUnsnoozed(b.repCtx, item.ID)
	if err != nil {
		t.Fatalf("taking back an expired snooze: %v", err)
	}
	if back.State != briefStateNew || back.SnoozedUntil != nil {
		t.Errorf("expired snooze taken back = %+v, want %q with no instant",
			back, briefStateNew)
	}
}
