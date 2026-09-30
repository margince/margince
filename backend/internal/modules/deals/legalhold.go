// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The litigation-hold writer for the deal table. The shape of the write is
// storekit.SetLegalHold, shared with the other two owners of holdable tables.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetLegalHold places or lifts the hold on one deal.
//
// Gated as a DELETE for the reason the sibling writers are: a hold decides
// whether a record can be erased, so the erasure authority governs it.
func (s *Store) SetLegalHold(ctx context.Context, id ids.UUID, held bool, reason string) error {
	if err := auth.Require(ctx, "deal", principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, dealTable, id); err != nil {
			return err
		}
		return storekit.SetLegalHold(ctx, tx, dealTable, id, held, reason)
	})
}
