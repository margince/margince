// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The writer's column list is checked against the TABLE, not against a copy of
// the table written down beside it.
//
// A column added to voice_profile_version and not to the writer is silent when
// it has a default and a hard error when it does not — and the silent case is
// the one that already happened: review_reasons is NOT NULL DEFAULT '{}', one
// of the three hand-written INSERTs left it out, and a manually activated
// version read back indistinguishable from one with genuinely no reasons.
//
// So the corpus is information_schema, and the only entries written down here
// are the columns the DATABASE fills. Each says why it is not the writer's to
// choose. Anything else new fails until somebody decides what this writer does
// with it, which is the decision the omission skipped.

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// databaseOwnedVersionColumns are the columns no writer supplies, with the
// reason each belongs to the database rather than to a caller.
var databaseOwnedVersionColumns = gatekit.Waive(map[string]string{
	"id":          "the primary key, minted by the column default",
	"version":     "the optimistic-concurrency counter, which starts at its default and moves only on update",
	"created_at":  "when the row was written, which is the database's clock and not a caller's",
	"archived_at": "set by an archive, never by the write that creates the row",
})

func TestTheVoiceVersionWriterCoversEveryColumnACallerChooses(t *testing.T) {
	// A ratified column the writer has taken over, or one the table no longer
	// has, is reported rather than quietly standing: an entry that matches
	// nothing is an exemption still covering a column nobody looked at again.
	defer databaseOwnedVersionColumns.AssertAllMatched(t)
	assertWriterCoversTable(t, "voice_profile_version", voiceVersionWriteColumns, databaseOwnedVersionColumns)
}

// The delta row's own list, held the same way: it is the second table this
// writer owns, and a column added there is as invisible as one added to the
// version.
var databaseOwnedDeltaColumns = gatekit.Waive(map[string]string{
	"id":          "the primary key, minted by the column default",
	"created_at":  "when the row was written, which is the database's clock and not a caller's",
	"updated_at":  "null until something updates the row, which is a fact about the row rather than a value the writer picks; the one update that exists (a candidate's outcome being decided) stamps it there",
	"archived_at": "set by an archive, never by the write that creates the row",
})

func TestTheVoiceDeltaWriterCoversEveryColumnACallerChooses(t *testing.T) {
	defer databaseOwnedDeltaColumns.AssertAllMatched(t)
	assertWriterCoversTable(t, "voice_profile_delta", voiceDeltaWriteColumns, databaseOwnedDeltaColumns)
}

func assertWriterCoversTable(t *testing.T, table string, written []string, databaseOwned *gatekit.Waivers[string]) {
	t.Helper()
	inTable := tableColumns(t, table)
	if len(inTable) == 0 {
		t.Fatalf("%s reports no columns at all, so this census is reading an empty tree", table)
	}
	waived := func(column string) bool { return databaseOwned.Waived(t, column) }
	for _, column := range columnsNothingWrites(inTable, written, waived) {
		t.Errorf("%s.%s is a column of the table and nothing writes it. A caller that means \"none\" must "+
			"say so; leaving it to the column default makes a writer that forgot it indistinguishable "+
			"from one that chose it. Add it to the writer, or record here why the database owns it",
			table, column)
	}
	for _, column := range columnsTheTableDoesNotHave(inTable, written) {
		t.Errorf("%s.%s is in the writer's column list and is not a column of the table. The INSERT will "+
			"fail at runtime for every caller; the writer is a column behind a migration that dropped or "+
			"renamed it, or the name is misspelled",
			table, column)
	}
}

// tableColumns reads the live column set, so the census cannot go stale
// against the migration that adds one.
func tableColumns(t *testing.T, table string) []string {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the column-census connection: %v", err)
		}
	})
	rows, err := conn.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		  AND is_generated = 'NEVER'`, table)
	if err != nil {
		t.Fatalf("reading %s's columns: %v", table, err)
	}
	columns, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collecting %s's columns: %v", table, err)
	}
	sort.Strings(columns)
	return columns
}
