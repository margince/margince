// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The three reads behind "since you last spoke" — stage moves, offers, Deal
// Room activity — run against a real database.
//
// They exist because the unit tests around this section drive the WORDING with
// a hand-built Input and never execute a statement. A column name guessed wrong
// is invisible to every one of them and answers 500 to the first rep who opens
// a brief, which is exactly what happened: deal_room_comment names its author
// author_participant_id, the query said participant_id, and nothing but a real
// query could tell the difference.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/meetingbrief"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// TestTheDealMoveReadsRunAgainstTheRealSchema executes all three statements the
// section is built from. It asserts they RUN and answer, not what they say:
// the wording is the unit tests' business, and the thing only a database can
// check is that every column named exists.
func TestTheDealMoveReadsRunAgainstTheRealSchema(t *testing.T) {
	e := Setup(t)
	dealID := e.SeedWonDealLinkedTo(t)
	ctx := principal.WithWorkspaceID(e.As(e.AdminUser, nil, roomErasureAdmin), e.WS)

	var moves []meetingbrief.DealMoveIn
	var hidden bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		moves, hidden, err = meetingbrief.ReadDealMovesForTest(ctx, tx, dealID,
			time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("the deal-move reads did not run: %v", err)
	}
	if hidden {
		t.Error("a caller holding deal_room read was told the room was hidden from them")
	}
	// A deal nobody worked has nothing to report, and saying so is the answer.
	if len(moves) != 0 {
		t.Errorf("a freshly seeded deal reported %d moves: %+v", len(moves), moves)
	}
}

// A reader without the deal_room grant is TOLD the room was left out, rather
// than being handed a brief that reads like a deal with a silent buyer.
func TestAReaderWithoutTheRoomGrantIsToldTheRoomWasLeftOut(t *testing.T) {
	e := Setup(t)
	dealID := e.SeedWonDealLinkedTo(t)
	noRooms := roomErasureAdmin
	noRooms.Objects = map[string]principal.ObjectGrant{
		"person":   {Read: true},
		"deal":     {Read: true},
		"activity": {Read: true},
	}
	ctx := principal.WithWorkspaceID(e.As(e.AdminUser, nil, noRooms), e.WS)

	var hidden bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		_, hidden, err = meetingbrief.ReadDealMovesForTest(ctx, tx, dealID,
			time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("the deal-move reads refused a reader they should have served: %v", err)
	}
	if !hidden {
		t.Error("a reader with no deal_room grant was not told the room was left out")
	}
}

// A reader who may not read deals is told nothing about the deal. The brief's
// own gates cover the meeting and the people in the room; until the deal gate
// was added, the section handed out the stage and the price of a deal the
// reader could not open.
func TestAReaderWithoutDealAccessIsToldNothingAboutTheDeal(t *testing.T) {
	e := Setup(t)
	dealID := e.SeedWonDealLinkedTo(t)
	noDeals := roomErasureAdmin
	noDeals.Objects = map[string]principal.ObjectGrant{
		"person":    {Read: true},
		"activity":  {Read: true},
		"deal_room": {Read: true},
	}
	ctx := principal.WithWorkspaceID(e.As(e.AdminUser, nil, noDeals), e.WS)

	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, _, err := meetingbrief.ReadDealMovesForTest(ctx, tx, dealID,
			time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC())
		return err
	})
	if err == nil {
		t.Fatal("a reader holding no deal grant was served the deal's stage and offer history")
	}
}
