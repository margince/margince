// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who a buyer can be pointed at for help.
//
// A room without a steward is a real state the schema admits, repaired by
// transferring one. A steward who EXISTS BUT CANNOT ACT is worse: the room
// looks staffed, so a buyer waits on somebody whose access was withdrawn and
// nothing prompts anyone to transfer it. Absence is at least visible.
//
// The old check asked only `archived_at IS NULL`, which admits the ordinary
// shape of somebody who has left — deactivated, not archived.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// deactivate withdraws a colleague's access without archiving the record — the
// shape a seat takes when somebody leaves.
func deactivate(t *testing.T, e *Env, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.As(e.AdminUser, nil, roomStewardAdmin), e.DB().Pool(),
		func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(),
				`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, user)
			return err
		}); err != nil {
		t.Fatalf("deactivating the colleague: %v", err)
	}
}

// own gives the deal an owner, which the fixture seeder does not.
func own(t *testing.T, e *Env, deal, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.As(e.AdminUser, nil, roomStewardAdmin), e.DB().Pool(),
		func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(),
				`UPDATE deal SET owner_id = $2 WHERE id = $1`, deal, user)
			return err
		}); err != nil {
		t.Fatalf("giving the deal an owner: %v", err)
	}
}

func TestARoomRefusesADeactivatedColleagueTheCallerNamed(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, roomStewardAdmin)
	dealID := e.SeedWonDealLinkedTo(t)
	deactivate(t, e, e.Rep1)

	steward := ids.From[ids.UserKind](e.Rep1)
	_, err := dealrooms.NewStore(e.DB()).CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](dealID), Title: "Acme — Deal Room",
		StewardUserID: &steward, Source: "ui",
	})

	if err == nil {
		t.Fatal("a room opened with a deactivated colleague as its steward: the room looks staffed " +
			"and the buyer is pointed at somebody whose access was withdrawn")
	}
	// The refusal must be ABOUT THE STEWARD. A seat missing a grant fails this
	// call too, and a case that accepted any error would pass without the fix
	// ever running — which is how a test comes to certify nothing.
	if !strings.Contains(err.Error(), "steward") {
		t.Fatalf("the room was refused for some other reason: %v", err)
	}
	// Refused rather than silently dropped: the caller CHOSE this contact, and
	// telling them is what lets them choose again. The refusal must not read as
	// a missing DEAL, which sends them looking in the wrong place.
	if errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the refusal reads as a missing deal rather than an ineligible steward: %v", err)
	}
}

// A DEAL OWNER who has been deactivated is a different case: nobody made a
// mistake in this request, so the room opens with no steward rather than being
// refused over a staffing change.
func TestARoomOpensWithNoStewardWhenTheDealsOwnerHasBeenDeactivated(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, roomStewardAdmin)
	dealID := e.SeedWonDealLinkedTo(t)
	// The seeder leaves the deal unowned, and an unowned deal reads as "no
	// steward" whatever this function does — so the case would pass over the
	// defect it is named for. The owner is what makes it a case at all.
	own(t, e, dealID, e.Rep1)
	deactivate(t, e, e.Rep1)

	room, err := dealrooms.NewStore(e.DB()).CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](dealID), Title: "Acme — Deal Room", Source: "ui",
	})
	if err != nil {
		t.Fatalf("opening a room on a deal whose owner has left: %v — refusing here blocks a room "+
			"over a staffing change, which is the outcome the function's own doc rejects", err)
	}
	if room.StewardUserId != nil {
		t.Errorf("the room named %v as its steward, and that seat cannot act — no steward is the "+
			"honest answer, and it is the one somebody can see and repair", *room.StewardUserId)
	}
}

// roomStewardAdmin is the seat that opens a room.
var roomStewardAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"deal":      {Create: true, Read: true, Update: true},
		"deal_room": {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeAll,
}
