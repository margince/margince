// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Sink) prepareCapturedActivityTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, rec connector.NormalizedRecord, fields ActivityFields) (connector.NormalizedRecord, ActivityFields, error) {
	// Alias adoption locks activity rows, so the merge lock must precede address resolution.
	if err := s.takeMergeLockFirst(ctx, tx, rec); err != nil {
		return rec, fields, err
	}
	var err error
	if rec, fields, err = s.readAgainstTheSeatsAddressesTx(ctx, tx, seat, rec, fields); err != nil {
		return rec, fields, err
	}
	if rec, err = s.rootThreadTx(ctx, tx, seat, rec, fields); err != nil {
		return rec, fields, err
	}
	rec, err = storeOriginalTx(ctx, tx, rec)
	return rec, fields, err
}
