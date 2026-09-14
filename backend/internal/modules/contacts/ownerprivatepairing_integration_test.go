// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A contact that says 'owner' and names nobody must not exist.
//
// The row-scope arm reads the pair as `visibility <> 'owner' OR owner_id = me`,
// so such a row satisfies neither side and is invisible to EVERY seat — its
// author, an admin, and any later attempt to repair it through the same
// endpoint. A record destroyed rather than a record made private.
//
// refuseUnreadableResult guards the ONE-STEP case: a patch that sets
// visibility to 'owner' while clearing or omitting the owner is refused. It
// cannot guard the two-step one, because it is a rule about a patch and this is
// a property of the ROW: narrow first with an owner named, clear the owner in a
// second patch, and each write is individually admissible while the pair they
// leave behind is not (#2137).
//
// So the estate holds it. A CHECK cannot be got around by splitting a write in
// two, which is the whole difference between a rule about a request and a rule
// about a record.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// clearOwnerDirectly is the second step, written the way the patch path writes
// it: the estate is asked, not the store, because what is under test is
// whether the DATABASE refuses the pair however it is reached.
func clearOwnerDirectly(t *testing.T, e *privacyEnv, id ids.ContactID) error {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	return e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact SET owner_id = NULL WHERE id = $1`, id)
		return err
	})
}

func TestAnOwnerPrivateContactCannotLoseItsOwner(t *testing.T) {
	e := setupCapturePrivacy(t)
	owned := e.captureContact(t, "owner")

	// The row as it starts: owner-private, and its owner can read it.
	if !e.canRead(e.as(e.owner, principal.RowScopeOwn), t, owned) {
		t.Fatal("the owner cannot read their own owner-private contact, so this test is not about what it says")
	}

	if err := clearOwnerDirectly(t, e, owned); err == nil {
		// Report what the row became, because "it was allowed" is a weaker
		// statement than what the row then is.
		var readable bool
		for _, seat := range []ids.UUID{e.owner, e.teammate, e.admin} {
			if e.canRead(e.as(seat, principal.RowScopeOwn), t, owned) {
				readable = true
			}
		}
		// The unbounded seat separately: if an admin with row_scope=all can
		// still reach it, the row is stranded rather than destroyed, and a
		// repair path exists.
		t.Logf("unbounded seat can read the stranded row: %v",
			e.canRead(e.as(e.admin, principal.RowScopeAll), t, owned))
		t.Errorf("an owner-private contact lost its owner and the estate accepted it; readable by any "+
			"of the three seats afterwards: %v. A row that says 'owner' and names nobody satisfies "+
			"neither side of the row-scope arm — it is destroyed rather than made private, and no "+
			"later patch through the same endpoint can reach it to repair it", readable)
	}
}

// And the pair is refused whichever way round it is written: narrowing a
// contact that already has no owner is the other order of the same two steps.
func TestAnOwnerlessContactCannotBecomeOwnerPrivate(t *testing.T) {
	e := setupCapturePrivacy(t)
	shared := e.captureContact(t, "workspace")
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact SET owner_id = NULL WHERE id = $1`, shared)
		return err
	}); err != nil {
		t.Fatalf("a workspace contact may have no owner, and this seeds that: %v", err)
	}

	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, shared)
		return err
	})
	if err == nil {
		t.Error("an ownerless contact was narrowed to owner-private and the estate accepted it")
	}
}

// The states either column reaches on its own are ordinary and stay ordinary.
// A rule that refused them would make the column unusable rather than safe.
func TestTheOrdinaryVisibilityStatesAreUntouched(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := context.Background()
	_ = ctx

	// Owner-private WITH an owner.
	if id := e.captureContact(t, "owner"); id.IsZero() {
		t.Error("an owner-private contact naming its owner was refused")
	}
	// Workspace-visible with an owner, and workspace-visible with none.
	shared := e.captureContact(t, "workspace")
	if err := e.store.tx(e.as(e.owner, principal.RowScopeOwn), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.as(e.owner, principal.RowScopeOwn),
			`UPDATE contact SET owner_id = NULL WHERE id = $1`, shared)
		return err
	}); err != nil {
		t.Errorf("a workspace contact was refused an empty owner, and an unassigned shared contact is "+
			"the ordinary state of a record nobody has claimed: %v", err)
	}
}
