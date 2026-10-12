// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The down-migration of 1791270706 refuses to run over a recorded Worklist
// completion rather than delete it: the row is the history an undo reads.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestTheWorklistCompletionDownMigrationRefusesOverRecordedCompletions(t *testing.T) {
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
	if _, err := conn.Exec(ctx, `
		INSERT INTO bulk_operation (record_type, verb, requested_by, changed_count, skipped_count)
		VALUES ('worklist_item', 'complete', 'human:fixture', 1, 0)`); err != nil {
		t.Fatalf("seeding a recorded completion: %v", err)
	}

	_, err = dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1791270706"), 1)
	if err == nil || !strings.Contains(err.Error(), "holds Worklist completions") {
		t.Fatalf("down → %v, want the migration's own refusal over a recorded completion", err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM bulk_operation WHERE verb = 'complete'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("%d recorded completions after the refused downgrade, want the one seeded", kept)
	}
}
