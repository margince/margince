// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// earlierRequestsOnCard answers the earlier unanswered requests a request card
// stands for: the same conversation, sent before it, still open, and readable
// by this reader. The Worklist shows them as one card, so setting the card
// aside sets them aside too; otherwise the next one would surface as a fresh
// card the moment this one went. Empty for a card that is not a request.
func earlierRequestsOnCard(ctx context.Context, tx pgx.Tx, card ids.ActivityID) ([]ids.ActivityID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	cardID := arg(card)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id FROM activity a
		  JOIN activity card ON card.id = $%[1]d AND card.owed_verdict = '`+OwedVerdictAsksUs+`'
		 WHERE a.archived_at IS NULL AND a.thread_key = card.thread_key AND a.kind = card.kind
		   AND a.channel_provider IS NOT DISTINCT FROM card.channel_provider
		   AND (a.occurred_at, a.id) < (card.occurred_at, card.id)
		   AND a.owed_verdict = '`+OwedVerdictAsksUs+`'
		   AND `+requestOpenSQL+`
		   AND %[2]s`, cardID, content), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the requests a card stands for: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
}

// setOnEarlierRequests writes the card's judgement onto each earlier request it
// stands for, each with its own audit row and event.
func (s *Store) setOnEarlierRequests(
	ctx context.Context, tx pgx.Tx, card ids.ActivityID, write func(ids.ActivityID) error,
) error {
	earlier, err := earlierRequestsOnCard(ctx, tx, card)
	if err != nil {
		return err
	}
	for _, id := range earlier {
		if err := write(id); err != nil {
			return err
		}
	}
	return nil
}

// sameAct is the reader's judgements written with the card's own, by set_at:
// undoing the card takes back exactly what that press set aside, and nothing
// the reader judged on its own before or after.
func sameAct(ctx context.Context, tx pgx.Tx, card ids.ActivityID, reader ids.UUID) ([]ids.ActivityID, time.Time, error) {
	var at time.Time
	err := tx.QueryRow(ctx, `SELECT set_at FROM activity_reader_state WHERE activity_id = $1 AND reader_id = $2`,
		card, reader).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("activities: reading when the card was set aside: %w", err)
	}
	earlier, err := earlierRequestsOnCard(ctx, tx, card)
	if err != nil || len(earlier) == 0 {
		return nil, at, err
	}
	rows, err := tx.Query(ctx, `SELECT activity_id FROM activity_reader_state
		 WHERE reader_id = $1 AND set_at = $2 AND activity_id = ANY($3)`, reader, at, earlier)
	if err != nil {
		return nil, at, fmt.Errorf("activities: reading what the card set aside with it: %w", err)
	}
	same, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
	return same, at, err
}
