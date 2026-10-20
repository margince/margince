// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealArchiveScope names the edges and assignments retired with the deal.
// Callers must authorize each referenced record before displaying any scope.
type DealArchiveScope = storekit.ArchiveScope

func (s *Store) PreviewDealArchive(ctx context.Context, id ids.DealID, version *int64) (DealArchiveScope, error) {
	var out DealArchiveScope
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, dealTable, id.UUID); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, dealTable, id.UUID, version); err != nil {
			return err
		}
		var err error
		out, err = storekit.PreviewArchiveScope(ctx, tx, id.UUID, dealArchiveRelationships, dealArchiveMemberships)
		return err
	})
	if err != nil {
		return DealArchiveScope{}, err
	}
	return out, nil
}
