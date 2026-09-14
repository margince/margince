// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Relink provenance before duplicate relationships are archived. Otherwise the
// surviving role loses its purchased support, while replay reads the retired
// duplicate as a human dismissal.
func relinkEmploymentSupport(ctx context.Context, tx pgx.Tx, source, target ids.UUID, mergingContact bool) error {
	column, other := companyFK, contactFK
	if mergingContact {
		column, other = contactFK, companyFK
	}
	args := []any{source}
	sourcePos := len(args)
	args = append(args, target)
	targetPos := len(args)
	duplicates := storekit.SQLf(`SELECT a.id old_id,min(b.id::text)::uuid new_id FROM relationship a JOIN relationship b
 ON a.kind='employment' AND b.kind=a.kind AND a.%s=b.%s
 AND a.%s=$%d AND b.%s=$%d AND a.archived_at IS NULL AND b.archived_at IS NULL
 AND %s GROUP BY a.id`, other, other, column, sourcePos, column, targetPos, roleKeyedDuplicateSQL)
	if _, err := tx.Exec(ctx, `WITH duplicates AS (`+duplicates+`)
 UPDATE provider_employment_resolution e SET relationship_id=d.new_id FROM duplicates d WHERE e.relationship_id=d.old_id`, args...); err != nil {
		return err
	}
	// A run may already support the survivor. Remove that redundant ledger row
	// before moving the remaining support, preserving the existing unique key.
	if _, err := tx.Exec(ctx, `WITH duplicates AS (`+duplicates+`)
 DELETE FROM provider_applied_field f USING duplicates d WHERE f.target_table='relationship' AND f.target_row_id=d.old_id
 AND EXISTS(SELECT 1 FROM provider_applied_field kept WHERE kept.run_id=f.run_id AND kept.target_table=f.target_table
 AND kept.target_field=f.target_field AND kept.target_row_id=d.new_id)`, args...); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `WITH duplicates AS (`+duplicates+`)
 UPDATE provider_applied_field f SET target_row_id=d.new_id FROM duplicates d
 WHERE f.target_table='relationship' AND f.target_row_id=d.old_id`, args...)
	return err
}
