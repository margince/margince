// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An erasure tombstones what it scrubs, so the spine's readers can stop there.
//
// audit_log is append-only: the erase cannot rewrite the images it certifies
// gone, and every read of the spine stops at the newest scrub tombstone for the
// record instead. That boundary fires only where a tombstone EXISTS, so a table
// the erasure empties without tombstoning is one whose images stay readable
// however carefully the reads are written.
//
// Two carry the subject in plain text. An attachment's create image holds the
// filename its uploader typed — routinely the subject's own name — and a
// scheduled send's holds the message subject line. Neither type is projected by
// field history, so the compliance log is the only door either was readable
// through, and the tombstone is what shuts it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnErasureTombstonesTheAttachmentsItPurged(t *testing.T) {
	e := integration.Setup(t)
	const subjectEmail = "collateral.subject@counterparty.test"

	var person, attachment ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO person (full_name, owner_id, source, captured_by, visibility)
			VALUES ('Collateral Subject', $1, 'manual', 'human:test', 'workspace')
			RETURNING id`, e.Rep1).Scan(&person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:test')`, person, subjectEmail); err != nil {
			return err
		}
		// The row the erasure deletes, and the image that outlives it. The
		// filename is what a reader must stop being able to see.
		if err := tx.QueryRow(ctx, `
			INSERT INTO attachment (entity_type, entity_id, filename, storage_key, source, captured_by)
			VALUES ('person', $1, 'collateral-subject-passport.pdf', 'blob/collateral', 'manual', 'human:test')
			RETURNING id`, person).Scan(&attachment); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after, occurred_at)
			VALUES ('human', 'human:test', 'create', 'attachment', $1,
			        '{"filename":"collateral-subject-passport.pdf"}'::jsonb, $2)`,
			attachment, time.Now().UTC().Add(-time.Hour))
		return err
	}); err != nil {
		t.Fatalf("seeding the subject and their attachment: %v", err)
	}

	// The eraser refuses to delete an attachment row whose object it cannot
	// purge, so the bytes never outlive their only key. An in-memory store is
	// the boundary stood in for; what is under test is the tombstone.
	if err := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory()).
		ErasePerson(e.Admin(), person, "subject request"); err != nil {
		t.Fatalf("ErasePerson: %v", err)
	}

	// The tombstone, written by the erasure itself — not seeded here, because
	// what is under test is that the erasure writes one.
	if n := countRows(t, e, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'attachment' AND entity_id = $1 AND action = 'erase'`, attachment); n != 1 {
		t.Fatalf("the erasure left %d tombstone(s) on the purged attachment, want 1 — "+
			"without one, its filename stays readable through the compliance log", n)
	}
}
