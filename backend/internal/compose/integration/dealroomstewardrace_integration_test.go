// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A room edit writes the steward it was sent, even when somebody moves the
// steward between its read and its write.
//
// The app sends the standing steward back with every save of the room's words.
// A save that repeats it is the ordinary case. What makes the repeat safe is
// that storekit.Patch binds every field it is given, equal or not.
//
// A patch that skipped a repeated field would read correctly and lose this
// race. The edit reads its row before the lock, so a steward moved in between
// would survive, and the caller would be told otherwise.
func TestARoomEditWritesTheStewardItWasSent(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, roomStewardAdmin)
	store := dealrooms.NewStore(e.DB())
	asked := ids.From[ids.UserKind](e.Rep1)
	room, err := store.CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](e.SeedWonDealLinkedTo(t)), Title: "Acme — Deal Room",
		Source: "manual", StewardUserID: &asked,
	})
	if err != nil {
		t.Fatalf("opening the room: %v", err)
	}
	roomID := ids.UUID(room.Id)

	// A writer holding the room's row, so the edit below blocks where it reads.
	holder := OwnerConn(t)
	tx, err := holder.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	//craft:ignore swallowed-errors the commit below leaves nothing to roll back
	defer func() { _ = tx.Rollback(t.Context()) }()
	var holderPID int32
	if err := tx.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(),
		`SELECT id FROM deal_room WHERE id = $1 FOR UPDATE`, roomID); err != nil {
		t.Fatalf("holding the room's row: %v", err)
	}

	title := "Acme — renewal room"
	repeated := &asked
	edited := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, err := store.UpdateRoom(ctx, ids.From[ids.DealRoomKind](roomID), dealrooms.UpdateRoomInput{
			Title: &title, StewardUserID: &repeated,
		})
		edited <- err
	}()

	// The edit has reached the row and stopped there, so what follows lands
	// strictly between its read and its write.
	testdb.AwaitQueuedBehind(t, holder, holderPID, 1, returned, "the room edit")

	moved := ids.From[ids.UserKind](e.Rep3)
	if _, err := tx.Exec(t.Context(),
		`UPDATE deal_room SET steward_user_id = $1 WHERE id = $2`, moved.UUID, roomID); err != nil {
		t.Fatalf("moving the steward: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-edited; err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}

	var standing ids.UUID
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT steward_user_id FROM deal_room WHERE id = $1`, roomID).Scan(&standing); err != nil {
		t.Fatal(err)
	}
	if standing != asked.UUID {
		t.Errorf("the room stands with steward %s, want the %s the edit sent", standing, asked)
	}
}
