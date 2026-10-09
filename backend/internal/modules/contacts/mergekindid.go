// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Re-homing the rows that name a record by kind and id.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// moveKindIDRows moves the polymorphic references of a merged-away contact or
// company. They carry no foreign key, so the schema cannot stop an
// (entity_type, entity_id) pair from pointing at an archived record.
func moveKindIDRows(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	if err := moveKindIDSingles(ctx, tx, entityType, sourceID, targetID); err != nil {
		return err
	}
	if err := moveKindIDGrants(ctx, tx, entityType, sourceID, targetID); err != nil {
		return err
	}
	return moveKindIDOwned(ctx, tx, entityType, sourceID, targetID)
}

// moveKindIDSingles moves the rows nothing makes unique per record, so a plain
// UPDATE can neither collide nor lose one.
func moveKindIDSingles(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`UPDATE signal SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
		`UPDATE attachment SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
		`UPDATE notice SET target_id = $2 WHERE target_type = $3 AND target_id = $1`,
		`UPDATE weekly_plan_commitment SET linked_record_id = $2
		 WHERE linked_record_type = $3 AND linked_record_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the rows naming the %s by kind and id: %w", entityType, err)
		}
	}
	return nil
}

// moveKindIDGrants carries the access shared on the retired company or contact.
// A subject who holds a grant on both keeps the stronger access and the longer
// reach, so nobody loses sight of the survivor.
func moveKindIDGrants(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`UPDATE record_grant b
		   SET access = CASE WHEN a.access = 'write' THEN 'write' ELSE b.access END,
		       expires_at = CASE WHEN a.expires_at IS NULL OR b.expires_at IS NULL THEN NULL
		                         ELSE GREATEST(a.expires_at, b.expires_at) END,
		       version = b.version + 1
		  FROM record_grant a
		 WHERE a.record_type = $3 AND a.record_id = $1
		   AND b.record_type = $3 AND b.record_id = $2
		   AND b.subject_type = a.subject_type AND b.subject_id = a.subject_id`,
		`DELETE FROM record_grant a
		  WHERE a.record_type = $3 AND a.record_id = $1 AND EXISTS (
		    SELECT 1 FROM record_grant b
		     WHERE b.record_type = $3 AND b.record_id = $2
		       AND b.subject_type = a.subject_type AND b.subject_id = a.subject_id)`,
		`UPDATE record_grant SET record_id = $2 WHERE record_type = $3 AND record_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("carry the grants on the %s: %w", entityType, err)
		}
	}
	return nil
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
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the %s's drafts and verdicts: %w", entityType, err)
		}
	}
	return nil
}
