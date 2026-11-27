// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// An installation holding two active templates for one outcome keeps the
// oldest, by creation time and then by id. That is the one the review writer
// already asked, so no rep sees other questions after the upgrade.
func TestTheOneTemplateMigrationKeepsTheOldestActiveTemplate(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1795139172")); err != nil {
		t.Fatalf("up to the migration before: %v", err)
	}
	// The two seeded rows share one created_at, so the id decides between them.
	for _, row := range []struct{ id, key string }{
		{"00000000-0000-7000-8000-000000000002", "tied_later_id"},
		{"00000000-0000-7000-8000-000000000001", "tied_first_id"},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO activity_review_template
			(id, key, label, outcome, questions, source, captured_by, created_at)
			VALUES ($1, $2, $2, 'won', '[{"key":"why","label":"Why?","type":"text","required":true}]', 'manual', 'human:x', '2000-01-01T00:00:00Z')`,
			row.id, row.key); err != nil {
			t.Fatalf("seeding template %s: %v", row.key, err)
		}
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if got := scalar(t, conn, `SELECT string_agg(key, ',') FROM activity_review_template WHERE outcome = 'won' AND active`); got != "tied_first_id" {
		t.Errorf("active won templates = %q, want only the oldest, tied_first_id", got)
	}
	if got := scalar(t, conn, `SELECT string_agg(key, ',') FROM activity_review_template WHERE outcome = 'lost' AND active`); got != "loss_review" {
		t.Errorf("active lost templates = %q, want the shipped loss_review untouched", got)
	}
}
