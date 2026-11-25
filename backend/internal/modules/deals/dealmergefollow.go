// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxMergeHops bounds how far a redirect is followed. A survivor can itself be
// merged later, so the answer is the end of the chain.
const maxMergeHops = 8

// FollowMergeTx answers the deal that carries on for a merged-away deal, and the
// deal itself when it was not merged. Who is responsible for a retired duplicate
// is whoever is responsible for the deal that replaced it.
func FollowMergeTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (ids.UUID, error) {
	for range maxMergeHops {
		var next *ids.UUID
		if err := tx.QueryRow(ctx, `SELECT merged_into_id FROM deal WHERE id = $1`, id).Scan(&next); err != nil {
			return id, err
		}
		if next == nil {
			return id, nil
		}
		id = *next
	}
	return id, nil
}
