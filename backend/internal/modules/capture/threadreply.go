// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func emitPendingThreadReplyTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	var rec connector.NormalizedRecord
	var fields ActivityFields
	var precision string
	err := tx.QueryRow(ctx, `SELECT a.source_system,a.source_id,a.thread_key,coalesce(a.counterparty_email,''),a.kind,a.direction,coalesce(a.channel_provider,''),a.occurred_at,a.occurred_precision FROM capture_thread_reply r JOIN activity a ON a.id=r.activity_id WHERE r.activity_id=@id AND NOT r.emitted AND a.archived_at IS NULL AND a.restricted_at IS NULL AND activity_thread_available(a.thread_key) FOR UPDATE OF r`, pgx.NamedArgs{"id": id}).Scan(&rec.NaturalKey.SourceSystem, &rec.NaturalKey.SourceID, &rec.ThreadKey, &rec.Counterparty.Email, &fields.Kind, &fields.Direction, &fields.ChannelProvider, &fields.OccurredAt, &precision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	fields.DateOnly = precision == "day"
	return emitReplyNow(ctx, tx, ids.Nil, id, rec, fields)
}
