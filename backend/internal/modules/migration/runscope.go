// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OwnSeat is the seat a run is kept to when its importer imports only their
// own records: the human, or the human an agent acts for. A workspace import
// answers nil, and so does a caller with no seat. The run stores it as
// own_seat, so scoping never parses a provenance stamp.
func OwnSeat(ctx context.Context) *ids.UserID {
	if auth.WorkspaceImporter(ctx) {
		return nil
	}
	return storekit.OwnerOrActor(ctx)
}

// ownRunScope narrows the import runs a statement reads to those the caller
// opened, for a caller who imports only their own records. A workspace
// importer, and the import worker, read every run. column is a compile-time
// column reference.
func ownRunScope(ctx context.Context, column string, arg func(any) int) string {
	if auth.WorkspaceImporter(ctx) {
		return ""
	}
	seat := OwnSeat(ctx)
	if seat == nil {
		return " AND false"
	}
	return storekit.SQLf(" AND %s = $%d", column, arg(seat.UUID))
}

// ownSeatArg is OwnSeat as a statement argument: NULL for a workspace import.
func ownSeatArg(ctx context.Context) *ids.UUID {
	if seat := OwnSeat(ctx); seat != nil {
		return &seat.UUID
	}
	return nil
}
