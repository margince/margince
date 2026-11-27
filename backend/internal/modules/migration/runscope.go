// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// ownRunScope narrows the import runs a statement reads to those the caller
// started, for a caller who imports only their own records. A run is theirs
// when its captured_by stamp ends in their seat's id, so a run their agent
// started counts as theirs too. A workspace importer, and the import worker,
// read every run. column is a compile-time column reference.
func ownRunScope(ctx context.Context, column string, arg func(any) int) string {
	if auth.WorkspaceImporter(ctx) {
		return ""
	}
	seat := storekit.OwnerOrActor(ctx)
	if seat == nil {
		return " AND false"
	}
	return storekit.SQLf(" AND right(%s, 36) = $%d", column, arg(seat.String()))
}
