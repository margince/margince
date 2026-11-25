// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resolutionCarries are the statements that move what the resolver decided
// about a merged-away record onto its survivor, per record kind. A resolution
// is still true of the survivor, and one left on the retired id stops matching.
var resolutionCarries = map[string][]struct{ entity, column, stmt string }{
	"contact": {
		{"signal", "resolved_contact_id", `UPDATE signal SET resolved_contact_id = $2 WHERE resolved_contact_id = $1 RETURNING id`},
	},
	"company": {
		{"signal", "resolved_company_id", `UPDATE signal SET resolved_company_id = $2 WHERE resolved_company_id = $1 RETURNING id`},
		// The append-only log behind those resolutions moves with them, or it
		// cannot be read back against the survivor it described.
		{"signal_resolution", "matched_company_id", `UPDATE signal_resolution SET matched_company_id = $2 WHERE matched_company_id = $1 RETURNING id`},
	},
}

// CarryResolutionsTx moves the signals and resolutions naming a merged-away
// contact or company onto the survivor, in the merge's transaction, and audits
// each moved row. The merge's own event is what a subscriber acts on.
func CarryResolutionsTx(ctx context.Context, tx pgx.Tx, kind string, from, to ids.UUID) error {
	carries, ok := resolutionCarries[kind]
	if !ok {
		return fmt.Errorf("signals: no resolution carry for a %q merge", kind)
	}
	if err := auth.Require(ctx, kind, principal.ActionUpdate); err != nil {
		return err
	}
	for _, c := range carries {
		rows, err := tx.Query(ctx, c.stmt, from, to)
		if err != nil {
			return fmt.Errorf("signals: moving %s.%s onto the surviving %s: %w", c.entity, c.column, kind, err)
		}
		moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("signals: moving %s.%s onto the surviving %s: %w", c.entity, c.column, kind, err)
		}
		if err := storekit.AuditEach(ctx, tx, "update", c.entity, moved,
			map[string]any{c.column: from}, map[string]any{c.column: to}); err != nil {
			return fmt.Errorf("signals: auditing the moved %s rows: %w", c.entity, err)
		}
	}
	return nil
}
