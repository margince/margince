// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A deal holds one open room at a time. Since 1794363873 that rule compares
// access windows, so an expired room can sit beside a newer open one. Rolling
// back must store the older one as expired, or the old unique index cannot
// come back.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestRollingTheOpenRoomWindowBackStoresTheOlderRoomAsExpired(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}
	var dealID string
	if err := conn.QueryRow(ctx, `
		WITH p AS (INSERT INTO pipeline (name) VALUES ('Rooms') RETURNING id),
		     s AS (INSERT INTO stage (pipeline_id, name, position) SELECT id, 'Open', 0 FROM p RETURNING id, pipeline_id)
		INSERT INTO deal (name, stage_id, pipeline_id, source, captured_by)
		SELECT 'Room deal', id, pipeline_id, 'manual', 'human:fixture' FROM s RETURNING id`).Scan(&dealID); err != nil {
		t.Fatalf("seeding a deal: %v", err)
	}
	insertRoom := func(title, createdAgo, expiresAgo string) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO deal_room (deal_id, title, source, captured_by, created_at, expires_at)
			VALUES ($1, $2, 'manual', 'human:fixture', now() - $3::interval, now() - $4::interval)`,
			dealID, title, createdAgo, expiresAgo)
		return err
	}
	// The window can never be inverted: the schema refuses an end at or before
	// creation, so tstzrange(created_at, expires_at) always has a valid bound.
	if err := insertRoom("Inverted", "1 day", "2 days"); err == nil {
		t.Fatal("a room ending before it was created was stored")
	} else if name, ok := storekit.CheckViolation(err); !ok || name != "deal_room_expires_after_creation" {
		t.Fatalf("an inverted window was refused by %v, want deal_room_expires_after_creation", err)
	}
	if err := insertRoom("Expired", "3 days", "2 days"); err != nil {
		t.Fatalf("seeding the expired room: %v", err)
	}
	if err := insertRoom("Current", "1 day", "-30 days"); err != nil {
		t.Fatalf("a new room beside an expired one was refused: %v", err)
	}

	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1794363873"), 1); err != nil {
		t.Fatalf("down: %v", err)
	}

	states := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT title, state FROM deal_room WHERE deal_id = $1`, dealID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var title, state string
		if err := rows.Scan(&title, &state); err != nil {
			t.Fatal(err)
		}
		states[title] = state
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if states["Expired"] != "expired" || states["Current"] != "live" {
		t.Fatalf("after the rollback the rooms read %v, want the older expired and the newer live", states)
	}
	var index *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('uq_deal_room_active')::text`).Scan(&index); err != nil {
		t.Fatal(err)
	}
	if index == nil {
		t.Fatal("the rollback did not bring uq_deal_room_active back")
	}
}
