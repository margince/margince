// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An older participant row holding a character that draws nothing is cleaned on
// its next update. It folds into the row that already names that party.
func TestAnOlderHiddenCharacterAddressIsCleanedAndFoldedOnUpdate(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()
	activity := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Quote', now(), 'gmail', 'connector:gmail')`, activity); err != nil {
		t.Fatal(err)
	}
	hidden := "\u200banna@buyer.example"
	testdb.WithParticipantSpellingCheckLifted(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role, display_name)
			VALUES ($1, $2, 'cc', ''), ($1, 'anna@buyer.example', 'cc', 'Anna')`, activity, hidden)
		return err
	})

	if _, err := e.owner.Exec(ctx, `UPDATE activity_participant SET response = 'accepted'
		WHERE activity_id = $1 AND address = $2`, activity, hidden); err != nil {
		t.Fatalf("updating the older row: %v", err)
	}
	var rows int
	var address, name string
	if err := e.owner.QueryRow(ctx, `SELECT count(*) OVER (), address, display_name FROM activity_participant
		WHERE activity_id = $1 AND role = 'cc'`, activity).Scan(&rows, &address, &name); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || address != "anna@buyer.example" || name != "Anna" {
		t.Errorf("the activity holds %d cc rows, %q named %q; want one clean row that kept the name", rows, address, name)
	}
}
