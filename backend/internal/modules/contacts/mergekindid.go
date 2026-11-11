// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Re-homing the rows that name a record by kind and id.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/sweepreach"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// moveKindIDRows moves the polymorphic references of a merged-away contact or
// company. They carry no foreign key, so the schema cannot stop an
// (entity_type, entity_id) pair from pointing at an archived record.
func moveKindIDRows(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	if err := moveKindIDSingles(ctx, tx, entityType, sourceID, targetID); err != nil {
		return err
	}
	if err := moveKindIDOwned(ctx, tx, entityType, sourceID, targetID); err != nil {
		return err
	}
	return moveRecordLinks(ctx, tx, entityType, sourceID, targetID)
}

// moveRecordLinks re-ends the typed links of a merged-away record on the
// survivor. The table is insert and delete only. A pair may hold one link per
// type. So each end is copied with ON CONFLICT DO NOTHING (the survivor's own
// link stands) and the retired record's rows are then deleted. A link
// between the two records is skipped: it would join the survivor to itself.
func moveRecordLinks(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by, created_at)
		 SELECT from_type, $2::uuid, to_type, to_id, link_type, captured_by, created_at FROM record_link
		  WHERE from_type = $3 AND from_id = $1 AND NOT (to_type = $3 AND to_id = $2)
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by, created_at)
		 SELECT from_type, from_id, to_type, $2::uuid, link_type, captured_by, created_at FROM record_link
		  WHERE to_type = $3 AND to_id = $1 AND NOT (from_type = $3 AND from_id = $2)
		 ON CONFLICT DO NOTHING`,
		`DELETE FROM record_link
		  WHERE ((from_type = $3 AND from_id = $1) OR (to_type = $3 AND to_id = $1))
		    AND $2::uuid IS NOT NULL`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the %s's typed links: %w", entityType, err)
		}
	}
	return nil
}

// moveKindIDSingles moves the rows nothing makes unique per record, so a plain
// UPDATE can neither collide nor lose one.
func moveKindIDSingles(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`UPDATE signal SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
		`UPDATE attachment SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
		`UPDATE notice SET target_id = $2 WHERE target_type = $3 AND target_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the rows naming the %s by kind and id: %w", entityType, err)
		}
	}
	// The declared tables follow every record kind the merge moves, the
	// company's fixed links among them.
	return sweepreach.Relink(ctx, tx, reach.Record(entityType), sourceID, targetID)
}

// moveKindIDOwned moves what one reader owns about a record: an unsent draft
// and a verdict on an AI claim. Both are unique per owner and anchor. Where the
// survivor already holds one, it stands and the retired copy is dropped, as the
// dismissals do.
func moveKindIDOwned(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`DELETE FROM mail_draft a
		  WHERE a.anchor_type = $3 AND a.anchor_id = $1 AND EXISTS (
		    SELECT 1 FROM mail_draft b
		     WHERE b.anchor_type = $3 AND b.anchor_id = $2 AND b.author_id = a.author_id)`,
		`UPDATE mail_draft SET anchor_id = $2 WHERE anchor_type = $3 AND anchor_id = $1`,
		`DELETE FROM ai_feedback a
		  WHERE a.subject_type = $3 AND a.subject_id = $1 AND EXISTS (
		    SELECT 1 FROM ai_feedback b
		     WHERE b.subject_type = $3 AND b.subject_id = $2
		       AND b.claim_kind = a.claim_kind AND b.claim_key = a.claim_key)`,
		`UPDATE ai_feedback SET subject_id = $2 WHERE subject_type = $3 AND subject_id = $1`,
		`DELETE FROM activity_link_suggestion a
		  WHERE a.entity_type = $3 AND a.entity_id = $1 AND EXISTS (
		    SELECT 1 FROM activity_link_suggestion b
		     WHERE b.entity_type = $3 AND b.entity_id = $2 AND b.activity_id = a.activity_id)`,
		`UPDATE activity_link_suggestion SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the %s's drafts and verdicts: %w", entityType, err)
		}
	}
	return nil
}
