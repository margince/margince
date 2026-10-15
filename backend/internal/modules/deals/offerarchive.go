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

// RefuseArchiveOffer is ArchiveOffer's authority probe without the write: the
// stage-time half a governed archive runs before it asks a human.
func (s *Store) RefuseArchiveOffer(ctx context.Context, id ids.OfferID) error {
	if err := auth.Require(ctx, "offer", principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		_, err := writableOffer(ctx, tx, id, storekit.LiveOnly)
		return err
	})
}
