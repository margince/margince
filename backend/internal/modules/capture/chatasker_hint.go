// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The hint a colleague gets when they answer a question about forwarded messages
// with something that is no answer, such as a thank-you. A batch is hinted at
// most once. The claim is written before the send, and a refused hint is not
// repeated.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// hintTarget is the held forwards of one colleague on one account that are owed
// a hint.
type hintTarget struct {
	member, connection ids.UUID
	provider, thread   string
	raws               []ids.UUID
}

func (a *ChatAsker) sendHints(ctx context.Context) error {
	var due []*hintTarget
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) (err error) {
		due, err = hintsOwedTx(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	var failed error
	for _, target := range due {
		failed = errors.Join(failed, a.sendHint(ctx, target))
	}
	return failed
}

func (a *ChatAsker) sendHint(ctx context.Context, t *hintTarget) error {
	account, verified, err := a.recipient(ctx, t.provider, t.member)
	if err != nil || !verified {
		return err
	}
	if err := connector.AllowChannelSend(connector.PurposeClarification, t.provider, t.thread, verified); err != nil {
		return nil //nolint:nilerr // a refused conversation is left unhinted, not retried
	}
	var claimed int64
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE raw_capture SET ask_hinted_at = now(), ask_hint_due_at = NULL
			 WHERE id = ANY($1) AND ask_hint_due_at IS NOT NULL AND ask_hinted_at IS NULL`, t.raws)
		claimed = tag.RowsAffected()
		return err
	}); err != nil || claimed == 0 {
		return err
	}
	receipt, err := a.send(ctx, t.provider, t.connection, account, hintQuestion(account.Locale, t.provider), "hint:"+t.raws[0].String(), 0)
	if err != nil {
		a.Log.WarnContext(ctx, "capture: the hint about answering was not delivered", "err", err)
		return nil
	}
	if receipt.ProviderMessageID == "" {
		return nil
	}
	return a.DB.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE raw_capture SET ask_message_ids = array_append(ask_message_ids, $2) WHERE id = ANY($1)`,
			t.raws, clarifyMessageKey(t.provider, account.ID, receipt.ProviderMessageID))
		return err
	})
}

// hintsOwedTx reads the held forwards flagged for a hint, grouped by colleague
// and account.
func hintsOwedTx(ctx context.Context, tx pgx.Tx) ([]*hintTarget, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.held_by, r.held_connection, a.channel_provider, a.thread_key
		  FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id
		 WHERE r.ask_hint_due_at IS NOT NULL AND r.ask_hinted_at IS NULL AND `+pendingforward.Predicate("a")+`
		 ORDER BY r.held_by, r.held_connection NULLS FIRST, r.id`)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the hints owed: %w", err)
	}
	defer rows.Close()
	type key struct{ member, connection ids.UUID }
	groups := map[key]*hintTarget{}
	var order []*hintTarget
	for rows.Next() {
		var raw ids.UUID
		var t hintTarget
		var conn *ids.UUID
		if err := rows.Scan(&raw, &t.member, &conn, &t.provider, &t.thread); err != nil {
			return nil, fmt.Errorf("capture: reading the hints owed: %w", err)
		}
		if conn != nil {
			t.connection = *conn
		}
		k := key{t.member, t.connection}
		g, ok := groups[k]
		if !ok {
			g = &t
			groups[k] = g
			order = append(order, g)
		}
		g.raws = append(g.raws, raw)
	}
	return order, rows.Err()
}
