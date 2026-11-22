// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A meeting copy's guest a member relinked reaches the survivor still marked,
// so no calendar roster on the survivor retires the member's link.
func TestAFoldedCopyCarriesAGuestsRelinkMark(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()
	meeting := func() ids.UUID {
		var id ids.UUID
		if err := e.owner.QueryRow(ctx, `INSERT INTO activity (kind, occurred_at, direction, source, captured_by)
 VALUES ('meeting', now() - interval '1 day', 'inbound', 'gcal', 'connector:gcal') RETURNING id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	survivor, duplicate := meeting(), meeting()
	var contact ids.UUID
	if err := e.owner.QueryRow(ctx, `INSERT INTO contact (full_name, source, captured_by)
 VALUES ('The Chosen Contact', 'manual', 'human:test') RETURNING id`).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role, contact_id, roster_seats, relinked_at)
 VALUES ($1, 'guest@partner.example', 'attendee', $2, ARRAY[$3::uuid], now())`, duplicate, contact, e.rep); err != nil {
		t.Fatal(err)
	}

	system := principal.SystemActing(principal.WithWorkspaceID(ctx, e.ws), "system:meeting-fold")
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	if err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
		return store.RetireMeetingCopyTx(system, tx, ids.From[ids.ActivityKind](duplicate), ids.From[ids.ActivityKind](survivor))
	}); err != nil {
		t.Fatalf("folding the copy: %v", err)
	}

	var carried, marked int
	if err := e.owner.QueryRow(ctx, `SELECT count(*), count(relinked_at) FROM activity_participant
 WHERE activity_id = $1 AND address = 'guest@partner.example'`, survivor).Scan(&carried, &marked); err != nil {
		t.Fatal(err)
	}
	if carried != 1 {
		t.Fatalf("the survivor holds %d rows for the guest, want the one carried", carried)
	}
	if marked != 1 {
		t.Error("the carried guest lost its relink mark, so the survivor's next sync may retire the member's link")
	}
}

// A folded copy's calendar joins the survivor's roster row for the same guest,
// so that guest stays until both calendars dropped them. A row no roster wrote
// on the survivor stays a member's.
func TestAFoldedCopyCarriesItsCalendarToTheSurvivorsGuests(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()
	meeting := func() ids.UUID {
		var id ids.UUID
		if err := e.owner.QueryRow(ctx, `INSERT INTO activity (kind, occurred_at, direction, source, captured_by)
 VALUES ('meeting', now() - interval '1 day', 'inbound', 'gcal', 'connector:gcal') RETURNING id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	survivor, duplicate := meeting(), meeting()
	otherCalendar := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role, roster_seats, roster_seat)
 VALUES ($1, 'both@partner.example', 'attendee', ARRAY[$3::uuid], NULL), ($1, 'member@partner.example', 'attendee', NULL, NULL),
        ($2, 'both@partner.example', 'attendee', ARRAY[$4::uuid], NULL), ($2, 'member@partner.example', 'attendee', ARRAY[$4::uuid], NULL),
        ($2, 'older@partner.example', 'attendee', NULL, $3)`, survivor, duplicate, e.rep, otherCalendar); err != nil {
		t.Fatal(err)
	}

	system := principal.SystemActing(principal.WithWorkspaceID(ctx, e.ws), "system:meeting-fold")
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	if err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
		return store.RetireMeetingCopyTx(system, tx, ids.From[ids.ActivityKind](duplicate), ids.From[ids.ActivityKind](survivor))
	}); err != nil {
		t.Fatalf("folding the copy: %v", err)
	}

	for address, want := range map[string]int{"both@partner.example": 2, "member@partner.example": 0, "older@partner.example": 1} {
		var rows, calendars int
		if err := e.owner.QueryRow(ctx, `SELECT count(*), coalesce(sum(cardinality(roster_seats)), 0) FROM activity_participant
 WHERE activity_id = $1 AND address = $2`, survivor, address).Scan(&rows, &calendars); err != nil {
			t.Fatal(err)
		}
		if rows != 1 || calendars != want {
			t.Errorf("%s: the survivor holds %d rows naming %d calendars, want 1 row naming %d", address, rows, calendars, want)
		}
	}
}
