// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settleCategoryClaims makes the two contacts' per-category admission claims
// fit one contact before the runs are repointed. The claims are unique per
// contact, provider and category, so a category both sides claim would
// collide on the repoint.
func settleCategoryClaims(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if err := skipQueuedRunsTheSourceIsBuying(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	// What still collides is two runs both past queued. Either may have been
	// charged and neither can be stopped, so both are kept, and the survivor's
	// claim stays while the source's is dropped. The rest of the source's
	// claims follow its runs through the key's ON UPDATE CASCADE.
	if _, err := tx.Exec(ctx, `
		DELETE FROM provider_run_category s
		 WHERE s.contact_id = $1
		   AND EXISTS (
		     SELECT 1 FROM provider_run_category o
		      WHERE o.contact_id = $2 AND o.provider = s.provider
		        AND o.category = s.category)`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("drop the merged-away record's colliding category claims: %w", err)
	}
	return nil
}

// skipQueuedRunsTheSourceIsBuying stops a survivor run that is still queued
// from buying a category a source run past queued is already buying. The
// queued run never reached the provider, so skipping it costs nothing, and
// its reservation stops counting once it is skipped. The source's queued runs
// were cancelled before this runs, so only that one direction is left.
func skipQueuedRunsTheSourceIsBuying(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	rows, err := tx.Query(ctx, `
		UPDATE provider_run q
		   SET state = 'skipped', skip_reason = 'category_in_flight', completed_at = now(),
		       input_fingerprint = 'merged:' || gen_random_uuid()::text
		 WHERE q.contact_id = $2 AND q.state = 'queued'
		   AND EXISTS (
		     SELECT 1 FROM provider_run_category mine
		       JOIN provider_run_category theirs
		         ON theirs.provider = mine.provider AND theirs.category = mine.category
		      WHERE mine.run_id = q.id AND theirs.contact_id = $1)
		RETURNING q.id`, sourceID.UUID, targetID.UUID)
	if err != nil {
		return fmt.Errorf("skip the survivor's queued runs the merged-away record is buying: %w", err)
	}
	skipped, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("read the skipped runs: %w", err)
	}
	const stateKey = "state"
	for _, runID := range skipped {
		if _, err := storekit.Audit(ctx, tx, "update", "provider_run", runID,
			map[string]any{stateKey: "queued"},
			map[string]any{stateKey: "skipped", "skip_reason": "category_in_flight"}); err != nil {
			return err
		}
	}
	return nil
}
