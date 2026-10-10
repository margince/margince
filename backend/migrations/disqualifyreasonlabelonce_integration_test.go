// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// One disqualification reason per label, over the twins a deployed database may
// already hold. They are seeded with no index, as the old writer left them.
// A migration that only built the index fails here rather than in a deploy.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

const disqualifyLabelOnceMigration = "core/1791627793_one_disqualification_reason_per_label.up.sql"

func TestDisqualifyReasonTwinsAreRenamedBeforeTheLabelIsHeldOnce(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DROP INDEX lead_disqualify_reason_label_once`); err != nil {
		t.Fatalf("restoring the schema the migration was written against: %v", err)
	}
	seed := func(label string, system bool, createdAt string) string {
		t.Helper()
		var id string
		if err := conn.QueryRow(ctx, `
			INSERT INTO lead_disqualify_reason (label, system, created_at) VALUES ($1, $2, $3)
			RETURNING id::text`, label, system, createdAt).Scan(&id); err != nil {
			t.Fatalf("seeding %q: %v", label, err)
		}
		return id
	}
	builtIn := seed("Went silent", true, "2026-03-01T00:00:00Z")
	olderTwin := seed(" went SILENT ", false, "2026-01-01T00:00:00Z")
	firstOfPair := seed("Gone dark", false, "2026-01-01T00:00:00Z")
	collidingTwin := seed("gone dark", false, "2026-02-01T00:00:00Z")
	seed("Gone dark (duplicate 1)", false, "2026-03-01T00:00:00Z")

	applyMigrationFile(t, conn, disqualifyLabelOnceMigration)

	labelOf := func(id string) string {
		t.Helper()
		var label string
		if err := conn.QueryRow(ctx, `SELECT label FROM lead_disqualify_reason WHERE id = $1`, id).Scan(&label); err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		return label
	}
	for _, c := range []struct{ id, want, why string }{
		{builtIn, "Went silent", "the built-in keeps its label over an older twin"},
		{olderTwin, "went SILENT (duplicate 1)", "a twin is renamed, not deleted"},
		{firstOfPair, "Gone dark", "the oldest keeps its label"},
		{collidingTwin, "gone dark (" + collidingTwin + ")", "a rename that would collide carries the row id"},
	} {
		if got := labelOf(c.id); got != c.want {
			t.Errorf("%s: label %q, want %q", c.why, got, c.want)
		}
	}

	_, err := conn.Exec(ctx, `INSERT INTO lead_disqualify_reason (label) VALUES ('GONE DARK ')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "lead_disqualify_reason_label_once" {
		t.Errorf("a second spelling of a held label was admitted after the migration: %v", err)
	}
}
