// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func withLeadTaskIdentities(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	return storekit.WithMailSeatIdentitySnapshot(ctx, tx)
}

func leadTaskIdentitiesSQL(ctx context.Context, arg func(any) int) (string, error) {
	return storekit.PreparedMailSeatIdentitiesSQL(ctx, arg)
}
