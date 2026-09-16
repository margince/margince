// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam between the ranked queue and the contact page's last-touch reader:
// a row and the record it opens answer "who wrote last" from one statement.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionContactTouch reads when a contact last wrote and when we did through
// the contact page's own set reader, so a queue row and the record it opens
// answer "who wrote last" from one statement.
type attentionContactTouch struct{ pool *pgxpool.Pool }

func (r attentionContactTouch) LastTouch(
	ctx context.Context, contactIDs []ids.UUID,
) (map[ids.UUID]attention.TouchMoments, error) {
	wanted := make([]ids.ContactID, 0, len(contactIDs))
	for _, id := range contactIDs {
		wanted = append(wanted, ids.From[ids.ContactKind](id))
	}
	out := make(map[ids.UUID]attention.TouchMoments, len(contactIDs))
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		touched, err := contact360.LastTouchFor(ctx, tx, wanted, contact360.AssembleOptions{})
		if err != nil {
			return err
		}
		for id, touch := range touched {
			out[id.UUID] = attention.TouchMoments{LastInbound: touch.InboundAt, LastOutbound: touch.OutboundAt}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

