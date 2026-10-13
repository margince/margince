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

// TagInterestUnreadClause says the activity has no answer read under both
// digests: the vocabulary, and the content the prompt shows. activity,
// vocabulary and content are SQL expressions.
func TagInterestUnreadClause(activity, vocabulary, content string) string {
	return `NOT EXISTS (SELECT 1 FROM tag_interest_read tir
	  WHERE tir.activity_id = ` + activity + ` AND tir.vocabulary_digest = ` + vocabulary + `
	    AND tir.content_digest = ` + content + `)`
}

// TagInterestAnswerSQL selects one tag_id row per tag the model found in the
// activity, for an answer read under both digests and no other. activity,
// vocabulary and content are SQL expressions.
func TagInterestAnswerSQL(activity, vocabulary, content string) string {
	return `SELECT unnest(tia.tag_ids) AS tag_id FROM tag_interest_read tia
	  WHERE tia.activity_id = ` + activity + ` AND tia.vocabulary_digest = ` + vocabulary + `
	    AND tia.content_digest = ` + content
}

// TagInterestAnswer is the model's answer for one activity: the tags it shows
// interest in, possibly none, and the digest of the text that was read.
type TagInterestAnswer struct {
	ActivityID    ids.UUID
	ContentDigest string
	TagIDs        []ids.UUID
}

// RecordTagInterestAnswersTx stores each answer under the vocabulary digest,
// replacing an older one. An activity deleted or restricted since it was read
// is skipped, and read again when it is available. System-only.
func RecordTagInterestAnswersTx(ctx context.Context, tx pgx.Tx, vocabulary string, answers []TagInterestAnswer) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	for _, answer := range answers {
		tags := answer.TagIDs
		if tags == nil {
			tags = []ids.UUID{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tag_interest_read (activity_id, vocabulary_digest, content_digest, tag_ids)
			SELECT a.id, $2::text, $3::text, $4::uuid[] FROM activity a
			 WHERE a.id = $1 AND `+auth.ActivityAvailableClause("a")+`
			ON CONFLICT (activity_id) DO UPDATE
			   SET vocabulary_digest = EXCLUDED.vocabulary_digest, content_digest = EXCLUDED.content_digest,
			       tag_ids = EXCLUDED.tag_ids, read_at = now()`,
			answer.ActivityID, vocabulary, answer.ContentDigest, tags); err != nil {
			return fmt.Errorf("collections: recording what the tag suggestion model read: %w", err)
		}
	}
	return nil
}
