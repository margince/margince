// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A row whose company moved out of its group is not folded away on the old reading.
//
// The sweep groups every ghost in one pass and the import rewrites one owner's rows
// in its own transaction, so a name can be committed between the grouping and the
// write. Acted on blind, the pass folds a row that is no longer a duplicate into
// another connection and deletes it, and nothing brings that row back.
func TestARowWhoseCompanyMovedIsNotCollapsedOnTheOldGrouping(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	var keep, moved ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		insert := `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name,
			   company_name, normalized_company, match_status, source)
			VALUES ($1, 'Abbas Fawaz', 'abbas fawaz', $2, $3, 'suggested', 'csv_export')
			RETURNING id`
		if err := tx.QueryRow(ctx, insert, e.rep, "najahak.io | Growth", "najahak.io | growth").
			Scan(&keep); err != nil {
			return err
		}
		return tx.QueryRow(ctx, insert, e.rep, "najahak.io", "najahak.io").Scan(&moved)
	}); err != nil {
		t.Fatalf("seeding the duplicate pair: %v", err)
	}

	// The grouping as the pass read it, before the import commits.
	group, wanted := groupingAsRead(ctx, t, e)

	// The import's commit: this row now names a different employer, so it is no
	// longer the duplicate the grouping took it for.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE linkedin_connection SET company_name = 'Qubix Partners' WHERE id = $1`, moved)
		return err
	}); err != nil {
		t.Fatalf("committing the import's new company name: %v", err)
	}

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, _, err := collapseGroup(ctx, tx, group, wanted)
		return err
	}); err != nil {
		t.Fatalf("collapsing on a grouping that moved: %v", err)
	}

	var survives bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT exists (SELECT 1 FROM linkedin_connection WHERE id = $1)`, moved).Scan(&survives)
	}); err != nil {
		t.Fatalf("reading back the row: %v", err)
	}
	if !survives {
		t.Error("the row was folded away and deleted on a grouping it had already left, and " +
			"no later pass restores a connection the sweep destroyed")
	}

	// And the survivor keeps its stored key. Re-keying it alone would claim the
	// natural key the moved row still holds, which fails the unique constraint and
	// takes the whole pass down over one racing row.
	var survivorKey string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT normalized_company FROM linkedin_connection WHERE id = $1`, keep).
			Scan(&survivorKey)
	}); err != nil {
		t.Fatalf("reading back the survivor: %v", err)
	}
	if survivorKey != "najahak.io | growth" {
		t.Errorf("the survivor was re-keyed to %q while the group was unresolved", survivorKey)
	}
}

// groupingAsRead captures what the sweep's own read and grouping produce right now,
// which is the reading a later write must not assume still holds.
func groupingAsRead(ctx context.Context, t *testing.T, e *dedupeEnv) ([]ghostKeyRow, map[ids.UUID]string) {
	t.Helper()
	var all []ghostKeyRow
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		all, err = readGhostKeys(ctx, tx, ids.UUID{})
		return err
	}); err != nil {
		t.Fatalf("reading the ghost keys: %v", err)
	}
	groups, wanted := groupByCurrentKey(all)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		return group, wanted
	}
	t.Fatalf("the seeded pair did not group together, so the test asserts nothing")
	return nil, nil
}

// A name rewritten cosmetically has not left its group, and is still collapsed.
//
// The grouping is by DERIVED key, so a re-import that appends a tagline or changes the
// casing commits a different company_name that normalizes to the same key. Comparing
// raw names would read that as a row that moved and leave the duplicate standing every
// pass, which is the backlog this sweep exists to drain.
func TestACosmeticRenameStillCollapsesWithItsGroup(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	var keep, touched ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		insert := `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name,
			   company_name, normalized_company, match_status, source)
			VALUES ($1, 'Abbas Fawaz', 'abbas fawaz', $2, $3, 'suggested', 'csv_export')
			RETURNING id`
		if err := tx.QueryRow(ctx, insert, e.rep, "najahak.io | Growth", "najahak.io | growth").
			Scan(&keep); err != nil {
			return err
		}
		return tx.QueryRow(ctx, insert, e.rep, "najahak.io", "najahak.io").Scan(&touched)
	}); err != nil {
		t.Fatalf("seeding the duplicate pair: %v", err)
	}

	group, wanted := groupingAsRead(ctx, t, e)

	// The import re-states the same employer in a different spelling.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE linkedin_connection SET company_name = '  NAJAHAK.IO  ' WHERE id = $1`, touched)
		return err
	}); err != nil {
		t.Fatalf("committing the cosmetic rename: %v", err)
	}

	var merged int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		merged, _, err = collapseGroup(ctx, tx, group, wanted)
		return err
	}); err != nil {
		t.Fatalf("collapsing after a cosmetic rename: %v", err)
	}
	if merged != 1 {
		t.Errorf("merged %d rows, want 1: the rewritten name normalizes to the same key, so the "+
			"row never left the group and the duplicate is still a duplicate", merged)
	}
}
