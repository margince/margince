// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// carryKeptProvenance moves the provenance of each field the survivor took
// from the merged-away record, read off the survivorship patch. So the value's
// origin travels with it. Each moved row is audited.
func carryKeptProvenance(ctx context.Context, tx pgx.Tx, objectType string, from, to ids.UUID, kept map[string]any) error {
	moved, err := storekit.MoveFieldProvenance(ctx, tx, objectType, from, to, slices.Sorted(maps.Keys(kept)))
	if err != nil {
		return err
	}
	if err := storekit.AuditEach(ctx, tx, "update", "field_provenance", moved,
		map[string]any{"object_id": from}, map[string]any{"object_id": to}); err != nil {
		return fmt.Errorf("audit the provenance the survivor took: %w", err)
	}
	return nil
}
