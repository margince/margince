// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// archiveDuplicatesKeepingPrimarySlot archives the merge's duplicate edges. An
// archived edge's primary-employer slot goes to the kept edge it folds into,
// the heir relinkEmploymentSupport gives the provider evidence.
// The heirs are read while both edges are live, and promoted after the archive
// because uq_rel_current_primary_employer is not deferrable. The slot witness
// keeps a contact's other primary.
func archiveDuplicatesKeepingPrimarySlot(ctx context.Context, tx pgx.Tx, mergingContact bool, archiveWhere string, args pgx.NamedArgs) error {
	rows, err := tx.Query(ctx, `WITH duplicates AS (`+mergeEmploymentDuplicates(mergingContact)+`)
 SELECT DISTINCT d.new_id FROM duplicates d
 JOIN relationship a ON a.id = d.old_id JOIN relationship n ON n.id = d.new_id
 WHERE `+employment.CurrentPrimarySlotSQL("a")+` AND NOT n.is_current_primary`, args)
	if err != nil {
		return fmt.Errorf("contacts: finding the edges a merge hands the primary employer to: %w", err)
	}
	heirs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("contacts: finding the edges a merge hands the primary employer to: %w", err)
	}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at = @archived`, archiveWhere, args); err != nil {
		return err
	}
	if len(heirs) == 0 {
		return nil
	}
	promoteArgs := pgx.NamedArgs{"heirs": heirs}
	_, err = mergeEdgeWrite(ctx, tx, `is_current_primary = true`,
		`a.id = ANY(@heirs) AND a.archived_at IS NULL AND NOT a.is_current_primary
 AND NOT EXISTS (SELECT 1 FROM relationship b WHERE `+mergePrimarySlotWitness+`)`, promoteArgs)
	return err
}
