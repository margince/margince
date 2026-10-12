// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The vocabulary the tag suggestion model reads against, and the ledger of
// which captured items it has read under which vocabulary.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SuggestibleTag is one live tag an admin marked suggestible, as the model
// reads it.
type SuggestibleTag struct {
	ID          ids.TagID
	Name        string
	Description string
}

// SuggestibleTagsTx reads the workspace's suggestible tags, ordered by id.
// System-only, for the scout that proposes them.
func SuggestibleTagsTx(ctx context.Context, tx pgx.Tx) ([]SuggestibleTag, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, name, coalesce(description, '') FROM tag
		 WHERE archived_at IS NULL AND suggestible
		 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("collections: reading the suggestible tags: %w", err)
	}
	tags, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SuggestibleTag, error) {
		var t SuggestibleTag
		err := row.Scan(&t.ID, &t.Name, &t.Description)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("collections: scanning the suggestible tags: %w", err)
	}
	return tags, nil
}

// TagInterestUnreadClause says the activity has not been read under the
// vocabulary digest. activity and digest are SQL expressions.
func TagInterestUnreadClause(activity, digest string) string {
	return `NOT EXISTS (SELECT 1 FROM tag_interest_read tir
	  WHERE tir.activity_id = ` + activity + ` AND tir.vocabulary_digest = ` + digest + `)`
}

// MarkTagInterestReadTx records that the model read each activity under the
// digest. An activity deleted or restricted since it was read is skipped, and
// read again when it is available. System-only.
func MarkTagInterestReadTx(ctx context.Context, tx pgx.Tx, digest string, activities []ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if len(activities) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tag_interest_read (activity_id, vocabulary_digest)
		SELECT a.id, $2::text FROM activity a WHERE a.id = ANY($1::uuid[]) AND `+auth.ActivityAvailableClause("a")+`
		ON CONFLICT (activity_id) DO UPDATE
		   SET vocabulary_digest = EXCLUDED.vocabulary_digest, read_at = now()`,
		activities, digest); err != nil {
		return fmt.Errorf("collections: recording what the tag suggestion model read: %w", err)
	}
	return nil
}
