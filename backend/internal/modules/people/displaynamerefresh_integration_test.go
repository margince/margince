// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The repair for contacts that learned their name before the display followed
// it. Every one of these existed in a real workspace: a calendar invitation
// minted the contact under the label the organizer had saved, a signature taught
// the record the real name, and the page kept showing the label.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// staleDisplayName plants the state the repair exists for: split columns naming
// the person, a display name that does not, and a machine on the row.
//
// Written directly rather than through the ensure ladder, because that ladder
// now moves the display name with the parts — the very fix this repairs the
// history of. The rows in a real workspace were written by the OLD code, and a
// test that could only produce them through the new code would be testing
// nothing.
func (e *dedupeEnv) staleDisplayName(ctx context.Context, t *testing.T, display, first, last, capturedBy string) ids.PersonID {
	t.Helper()
	id := ids.From[ids.PersonKind](ids.NewV7())
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO person (id, full_name, first_name, last_name, source, captured_by)
			VALUES ($1, $2, $3, $4, 'gmail:test', $5)`,
			id, display, first, last, capturedBy)
		return err
	}); err != nil {
		t.Fatalf("planting the stale record: %v", err)
	}
	return id
}

func TestTheRepairShowsTheNameTheRecordAlreadyKnows(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	id := e.staleDisplayName(ctx, t, "Bw", "Björn", "Welter", "connector:gmail")

	var moved bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = RefreshDisplayNameTx(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	if !moved {
		t.Fatal("the repair reported no change, want the display name moved")
	}
	if full, _, _ := e.storedName(ctx, t, id); full != "Björn Welter" {
		t.Errorf("full_name = %q, want the name the record's own columns carry", full)
	}
}

// A contact a person added by hand keeps the name they typed, whatever the split
// columns say. captured_by is what says a human made this record.
func TestTheRepairLeavesAContactAPersonCreatedAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	id := e.staleDisplayName(ctx, t, "Bobby", "Robert", "Fischer", "human:someone")

	var moved bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = RefreshDisplayNameTx(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	if moved {
		t.Error("the repair rewrote a contact a person created")
	}
	if full, _, _ := e.storedName(ctx, t, id); full != "Bobby" {
		t.Errorf("full_name = %q, want the name the person typed — they added this contact, "+
			"and calling him Robert Fischer is not the repair's call", full)
	}
}

// Twice over the same record writes once. A repair that re-wrote on every tick
// would put one audit row and one person.updated event on the bus per pass, for
// a change that already happened.
func TestTheRepairWritesNothingTheSecondTime(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	id := e.staleDisplayName(ctx, t, "Juan", "Judith", "Andresen", "connector:gmail")

	for i, wantMoved := range []bool{true, false} {
		var moved bool
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			var err error
			moved, err = RefreshDisplayNameTx(ctx, tx, id)
			return err
		}); err != nil {
			t.Fatalf("pass %d: %v", i+1, err)
		}
		if moved != wantMoved {
			t.Fatalf("pass %d reported moved=%v, want %v — the second pass has nothing left to do",
				i+1, moved, wantMoved)
		}
	}
}
