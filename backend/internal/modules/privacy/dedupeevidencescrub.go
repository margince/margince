// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// emptyDedupeEvidence is the snapshot's own empty shape. The column is NOT NULL
// and the detector writes a JSON ARRAY of comparison rows, so a pair whose
// evidence has been destroyed carries no rows rather than no value.
const emptyDedupeEvidence = `[]`
const dedupeArchivedKey = "archived"

// Anonymizing leaves subject rows in place, so foreign-key cascades cannot clear
// their comparison snapshots. Archive undecided pairs without inventing a verdict.
// Contacts rechecks keep evidence for later review; privacy must destroy all
// subject snapshots in the same transaction as the subject’s data.
func scrubDedupeEvidence(ctx context.Context, tx pgx.Tx, contacts, leads []ids.UUID) error {
	if len(contacts) == 0 && len(leads) == 0 {
		return nil
	}
	args := pgx.StrictNamedArgs{"contacts": contacts, leadsParameter: leads}
	rows, err := tx.Query(ctx, `SELECT id FROM dedupe_candidate
  WHERE (left_contact_id = ANY(@contacts) OR right_contact_id = ANY(@contacts)
   OR left_lead_id = ANY(@leads) OR right_lead_id = ANY(@leads))
   AND disposition = 'open' AND archived_at IS NULL FOR UPDATE`, args)
	if err != nil {
		return fmt.Errorf("privacy: locking pending duplicate matches: %w", err)
	}
	closed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: reading pending duplicate matches: %w", err)
	}
	args["empty"] = emptyDedupeEvidence
	scrubbed, err := scrubbedIDs(ctx, tx, `
		UPDATE dedupe_candidate
		   SET evidence = @empty::jsonb,
		       archived_at = CASE WHEN disposition = 'open' THEN coalesce(archived_at, now()) ELSE archived_at END,
		       version = version + 1
		 WHERE (left_contact_id = ANY(@contacts) OR right_contact_id = ANY(@contacts)
		    OR left_lead_id = ANY(@leads) OR right_lead_id = ANY(@leads))
		   AND (evidence IS DISTINCT FROM @empty::jsonb
		        OR (disposition = 'open' AND archived_at IS NULL)) RETURNING id`,
		args)
	if err != nil {
		return fmt.Errorf("privacy: destroying the subject's duplicate-pair evidence: %w", err)
	}
	if err := tombstoneCollateralScrubs(ctx, tx, "dedupe_candidate", scrubbed, subjectRedactionReason, subjectRedactionCause); err != nil {
		return err
	}
	for _, id := range closed {
		if _, err := storekit.Audit(ctx, tx, "archive", "dedupe_candidate", id,
			map[string]any{dedupeArchivedKey: false}, map[string]any{dedupeArchivedKey: true, evidenceKeyReason: "subject_erased"}); err != nil {
			return fmt.Errorf("privacy: auditing duplicate match closure: %w", err)
		}
	}
	return nil
}
