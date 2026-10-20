// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/deals"
)

func (r approvalPreviewReader) validateProgressionCriteria(ctx context.Context, change deals.StageProgressionChange) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		return r.deals.ValidateProgressionCriteriaTx(ctx, tx, change)
	})
}
