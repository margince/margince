// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/introduction"
)

type introCarryPreviewer interface {
	PreviewIntroCarryTx(context.Context, pgx.Tx, ids.ContactID, ids.ContactID) ([]introduction.CarriedRequest, error)
}

func (s *Store) previewIntroCarry(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) ([]introduction.CarriedRequest, error) {
	if reader, ok := s.introCarrier.(introCarryPreviewer); ok {
		return reader.PreviewIntroCarryTx(ctx, tx, from, to)
	}
	return nil, refuseUnwiredIntroCarry(ctx, tx, from)
}
