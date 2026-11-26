// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// seedTag files one tag, archived when archived is true.
func seedTag(t *testing.T, conn *pgx.Conn, name string, archived bool) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(),
		`INSERT INTO tag (name, archived_at) VALUES ($1, CASE WHEN $2 THEN now() END) RETURNING id`,
		name, archived).Scan(&id); err != nil {
		t.Fatalf("seeding the tag %s: %v", name, err)
	}
	return id
}

// seedTagAudit files one audit row on a tag, at a known instant, with the after
// image a merge, a restore or a retirement writes.
func seedTagAudit(t *testing.T, conn *pgx.Conn, tag, after string, at time.Time) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `INSERT INTO audit_log
		(actor_type, actor_id, action, entity_type, entity_id, after, occurred_at)
		VALUES ('human', 'test', 'update', 'tag', $1, NULLIF($2, '')::jsonb, $3)`,
		tag, after, at); err != nil {
		t.Fatalf("seeding an audit row on %s: %v", tag, err)
	}
}

// A tag merged before the column existed takes its survivor from the merge's
// audit row, and only while that row is its last word.
func TestTheMergedTagBackfillNamesOnlyTagsStillMerged(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795701165")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the column back: %v", err)
	}

	kept := seedTag(t, conn, "Renamed", false)
	merged := seedTag(t, conn, "Colored", true)
	retired := seedTag(t, conn, "Trade Fair", true)
	mergedThenRetired := seedTag(t, conn, "Old Merge", true)
	mergedThenRestored := seedTag(t, conn, "Back Again", false)
	mergeAfter := `{"merged_into":"` + kept + `","moved":1,"collapsed":0}`
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seedTagAudit(t, conn, merged, mergeAfter, at)
	seedTagAudit(t, conn, retired, "", at)
	seedTagAudit(t, conn, mergedThenRetired, mergeAfter, at)
	seedTagAudit(t, conn, mergedThenRetired, `{"archived_at":null,"name":"Old Merge"}`, at.Add(time.Hour))
	seedTagAudit(t, conn, mergedThenRetired, "", at.Add(2*time.Hour))
	seedTagAudit(t, conn, mergedThenRestored, mergeAfter, at)
	seedTagAudit(t, conn, mergedThenRestored, `{"archived_at":null,"name":"Back Again"}`, at.Add(time.Hour))

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for tag, want := range map[string]*string{
		merged: &kept, retired: nil, mergedThenRetired: nil, mergedThenRestored: nil, kept: nil,
	} {
		var into *string
		if err := conn.QueryRow(ctx, `SELECT merged_into_id::text FROM tag WHERE id = $1`, tag).Scan(&into); err != nil {
			t.Fatal(err)
		}
		if (into == nil) != (want == nil) || (into != nil && *into != *want) {
			t.Errorf("tag %s merged_into_id = %v, want %v", tag, orNull(into), orNull(want))
		}
	}
}

func orNull(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}
