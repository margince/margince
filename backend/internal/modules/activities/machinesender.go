// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InboundSender is one received mail not yet marked as a sending system's,
// with the address capture filed as its sender.
type InboundSender struct {
	ID      ids.UUID
	Address string
}

// unmarkedInboundMailScope selects received mail with a sender address that is
// not marked sender_is_machine yet. A marked row leaves it, so a replay of the
// data task finds nothing to change.
const unmarkedInboundMailScope = `FROM activity a
	 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.counterparty_email IS NOT NULL AND NOT a.sender_is_machine`

// InboundSendersTx reads the next limit unmarked received mails after the
// cursor, in id order. Which of them a sending system sent is the caller's
// judgement, by the rule capture stamps the column with.
func InboundSendersTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]InboundSender, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.counterparty_email `+unmarkedInboundMailScope+`
		   AND a.id > $1 ORDER BY a.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading received mail senders: %w", err)
	}
	senders, err := pgx.CollectRows(rows, pgx.RowToStructByPos[InboundSender])
	if err != nil {
		return nil, fmt.Errorf("activities: reading received mail senders: %w", err)
	}
	return senders, nil
}

// MarkMachineSendersTx marks the given received mails as sent by a sending
// system and returns the ids it changed. A row already marked is left alone.
func MarkMachineSendersTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(activityIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `UPDATE activity a SET sender_is_machine = true
		 WHERE a.id = ANY($1) AND a.kind = 'email' AND a.direction = 'inbound' AND NOT a.sender_is_machine
		 RETURNING a.id`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("activities: marking mail from sending systems: %w", err)
	}
	marked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: marking mail from sending systems: %w", err)
	}
	return marked, nil
}

// CountInboundSenders counts the unmarked received mails, reading at most
// maximum+1.
func CountInboundSenders(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+unmarkedInboundMailScope+` LIMIT $1) unmarked`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting received mail senders: %w", err)
	}
	return count, nil
}
