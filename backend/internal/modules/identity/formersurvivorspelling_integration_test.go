// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A former teammate's survivor is the one spelled as the caller typed it.
//
// The login match reads both spellings of a domain. So the lineage can start
// from a seat per spelling and reach a live root for each.
//
// Read without an order, the answer was whichever root came back first.
func TestTheSurvivorSpelledAsTypedWinsAMergedLineage(t *testing.T) {
	e := setupRevocationEnv(t, "former-survivor-spelling")
	ctx := context.Background()
	const unicodeAddress = "gone@bücher.example"
	const punycodeAddress = "gone@xn--bcher-kva.example"

	// Both spellings first, then each merged: the invite refuses a second
	// spelling once the first is retired.
	roots := map[string]ids.UserID{}
	merged := map[string]ids.UserID{}
	for i, seat := range []struct{ email, name string }{
		{unicodeAddress, "Gone Unicode"},
		{punycodeAddress, "Gone Punycode"},
	} {
		id, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
			Email: seat.email, DisplayName: seat.name, Role: "rep",
		})
		if err != nil {
			t.Fatalf("inviting %s: %v", seat.email, err)
		}
		merged[seat.email] = id

		root, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
			Email: fmt.Sprintf("root%d@plain.example", i), DisplayName: seat.name + " Root",
			Role: "rep",
		})
		if err != nil {
			t.Fatalf("inviting the root for %s: %v", seat.email, err)
		}
		roots[seat.email] = root
	}
	for address, id := range merged {
		// app_user_merge_is_retired wants removed_at, and
		// app_user_removed_is_deactivated wants the status with it.
		if _, err := e.owner.Exec(ctx,
			`UPDATE app_user SET merged_into_user_id = $2, removed_at = now(),
			          status = 'deactivated'
			   WHERE id = $1`,
			id.UUID, roots[address].UUID); err != nil {
			t.Fatalf("merging %s: %v", address, err)
		}
	}

	// Each spelling answers its own root. Without the order one of these two
	// reads the other's, whichever the database reached first.
	for address, want := range roots {
		var got ids.UserID
		var found bool
		seat := principal.WithWorkspaceID(ctx, e.ws.UUID)
		if err := database.WithWorkspaceTx(seat, e.svc.db.Pool(), func(tx pgx.Tx) error {
			var err error
			got, found, err = mergedFormerSurvivor(seat, tx, address)
			return err
		}); err != nil {
			t.Fatalf("reading the survivor for %s: %v", address, err)
		}
		if !found {
			t.Fatalf("no survivor for %s, want its own root", address)
		}
		if got != want {
			t.Errorf("%s answered survivor %s, want its own root %s", address, got, want)
		}
	}
}
