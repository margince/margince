// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The question a company chat account asks a colleague who forwarded messages
// that name no author, and the follow-up when an answer names no batch. One
// sweep per workspace forms the batches, asks about each once and sends the
// follow-ups. Nothing is sent from the webhook.
//
// A send is claimed before the provider is called. A refused send releases its
// claim and is tried again after a pause. A send that ended in doubt keeps its
// claim, so the colleague is never asked twice. connector.AllowChannelSend is
// the one rule on who may be written to.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MemberAccount is a member's verified account on the network a provider is on.
type MemberAccount struct {
	// Scope is the official account for Zalo and empty elsewhere.
	Scope string
	ID    string
	// Locale is the language the member reads Margince in, empty when unset.
	Locale string
}

// MemberAccounts answers which account a member verified, the inverse of
// MemberIdentities.
type MemberAccounts interface {
	AccountOf(ctx context.Context, tx pgx.Tx, provider string, member ids.UUID) (MemberAccount, bool, error)
}

// ChatAsker asks colleagues who wrote the messages they forwarded.
type ChatAsker struct {
	DB      *database.DB
	Members MemberAccounts
	Senders ChatSenders
	Now     func() time.Time
	Log     *slog.Logger
}

// replyWindowOf is how long after a colleague last wrote the provider lets the
// account answer them, zero when it states none.
func replyWindowOf(provider string) time.Duration {
	switch provider {
	case whatsapp.Provider:
		return whatsapp.ReplyWindow
	case zalo.Provider:
		return zalo.ReplyWindow
	}
	return 0
}

// Sweep forms the settled batches, asks about each one not yet asked, and sends
// the follow-ups owed. One batch that cannot be asked does not stop the rest.
func (a *ChatAsker) Sweep(ctx context.Context) error {
	var due []ClarifyBatch
	err := a.DB.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := FormClarifyBatchesTx(ctx, tx); err != nil {
			return err
		}
		var err error
		due, err = DueClarifyBatchesTx(ctx, tx)
		return err
	})
	if err != nil {
		return err
	}
	var failed error
	for _, batch := range due {
		if err := a.ask(ctx, batch); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	return errors.Join(failed, a.sendFollowUps(ctx), a.sendHints(ctx))
}

// recipient is the member's verified account, read at the moment of sending.
func (a *ChatAsker) recipient(ctx context.Context, provider string, member ids.UUID) (MemberAccount, bool, error) {
	var account MemberAccount
	var found bool
	err := a.DB.Tx(ctx, func(tx pgx.Tx) (err error) {
		account, found, err = a.Members.AccountOf(ctx, tx, provider, member)
		return err
	})
	return account, found, err
}

// ask sends the question about one batch.
func (a *ChatAsker) ask(ctx context.Context, batch ClarifyBatch) error {
	account, verified, err := a.recipient(ctx, batch.Provider, batch.Member)
	if err != nil || !verified {
		return err
	}
	if window := replyWindowOf(batch.Provider); window > 0 && a.Now().Sub(batch.LastWrote) > window {
		a.Log.InfoContext(ctx, "capture: a batch of forwards is past the provider's reply window and stays held")
		return nil
	}
	if err := connector.AllowChannelSend(connector.PurposeClarification, batch.Provider, batch.ThreadKey, verified); err != nil {
		return nil //nolint:nilerr // a refused conversation is left unasked, not retried
	}
	var claimed bool
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) (err error) {
		claimed, err = ClaimClarifyBatchTx(ctx, tx, batch.ID)
		return err
	}); err != nil || !claimed {
		return err
	}
	attempt := batch.Attempts
	receipt, sendErr := a.send(ctx, batch.Provider, batch.Connection, account, clarifyQuestion(account.Locale, batch.Provider), "ask:"+batch.ID.String(), attempt)
	return a.settle(ctx, batch.ID, receipt.ProviderMessageID, sendErr)
}

// send transmits one text to a verified colleague through the account's sender.
func (a *ChatAsker) send(ctx context.Context, provider string, connection ids.UUID, to MemberAccount, text, key string, attempt int) (connector.SendReceipt, error) {
	sender, auth, err := a.Senders.For(ctx, provider, connection)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	return sender.SendMessage(ctx, auth, connector.ChannelMessage{
		Recipient:      connector.ChannelIdentity{Provider: provider, AccountScope: to.Scope, ChannelUserID: to.ID},
		Body:           text,
		IdempotencyKey: key,
		Attempt:        attempt,
	})
}

// settle records how a claimed send ended. A send that ended in doubt keeps its
// claim.
func (a *ChatAsker) settle(ctx context.Context, batch ids.UUID, messageID string, sendErr error) error {
	retry := true
	switch {
	case sendErr == nil && messageID == "":
		messageID = "sent:" + batch.String()
	case sendErr == nil:
	case errors.Is(sendErr, connector.ErrSendOutcomeUnknown):
		a.Log.WarnContext(ctx, "capture: the outcome of a question about forwards is unknown, so it is not asked again")
		return nil
	case errors.Is(sendErr, connector.ErrOutsideReplyWindow), errors.Is(sendErr, connector.ErrRecipientUnreachable):
		retry = false
	}
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) error { return SettleClarifyBatchTx(ctx, tx, batch, messageID, retry) }); err != nil {
		return err
	}
	if sendErr != nil {
		a.Log.WarnContext(ctx, "capture: a question about forwards was not delivered", "retry", retry, "err", sendErr)
	}
	return nil
}

// followUp is what one colleague on one account is owed.
type followUp struct {
	member, connection ids.UUID
	provider, thread   string
	raws               []ids.UUID
	batches            []*pickItem
	byBatch            map[ids.UUID]*pickItem
}

// sendFollowUps sends each colleague owed one the question that lists their
// waiting batches. It claims the flag before sending, and does not retry a
// follow-up the provider refused: the colleague's next answer asks again.
func (a *ChatAsker) sendFollowUps(ctx context.Context) error {
	var owed []*followUp
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) (err error) {
		owed, err = followUpsOwedTx(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	var failed error
	for _, f := range owed {
		if err := a.sendFollowUp(ctx, f); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	return failed
}

func (a *ChatAsker) sendFollowUp(ctx context.Context, f *followUp) error {
	account, verified, err := a.recipient(ctx, f.provider, f.member)
	if err != nil || !verified {
		return err
	}
	if err := connector.AllowChannelSend(connector.PurposeClarification, f.provider, f.thread, verified); err != nil {
		return nil //nolint:nilerr // a refused conversation is left unasked, not retried
	}
	var claimed int64
	if err := a.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE raw_capture SET ask_followup_at = NULL WHERE id = ANY($1) AND ask_followup_at IS NOT NULL`, f.raws)
		claimed = tag.RowsAffected()
		return err
	}); err != nil || claimed == 0 {
		return err
	}
	items := make([]pickItem, 0, len(f.batches))
	for _, item := range f.batches {
		items = append(items, *item)
	}
	receipt, err := a.send(ctx, f.provider, f.connection, account, followUpQuestion(account.Locale, items), "pick:"+f.raws[0].String(), 0)
	if err != nil {
		a.Log.WarnContext(ctx, "capture: the follow-up question about forwards was not delivered", "err", err)
		return nil
	}
	if receipt.ProviderMessageID == "" {
		return nil
	}
	return a.DB.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE raw_capture SET ask_message_ids = array_append(ask_message_ids, $2) WHERE id = ANY($1)`,
			f.raws, receipt.ProviderMessageID)
		return err
	})
}

// followUpsOwedTx reads the held forwards flagged for a follow-up, grouped by
// colleague and account, with the batches each group lists oldest first.
func followUpsOwedTx(ctx context.Context, tx pgx.Tx) ([]*followUp, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.held_by, r.held_connection, r.ask_batch_id, a.channel_provider, a.thread_key, a.occurred_at, a.body
		  FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id
		 WHERE r.held_by IN (SELECT held_by FROM raw_capture WHERE ask_followup_at IS NOT NULL)
		   AND r.ask_sent_at IS NOT NULL AND `+pendingforward.Predicate("a")+`
		 ORDER BY r.held_by, r.held_connection NULLS FIRST, r.ask_sent_at, a.occurred_at, r.id`)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the follow-up questions owed: %w", err)
	}
	type key struct{ member, connection ids.UUID }
	groups := map[key]*followUp{}
	var order []*followUp
	for rows.Next() {
		var raw, member, batch ids.UUID
		var conn *ids.UUID
		var provider, thread, body string
		var at time.Time
		if err := rows.Scan(&raw, &member, &conn, &batch, &provider, &thread, &at, &body); err != nil {
			rows.Close()
			return nil, fmt.Errorf("capture: reading the follow-up questions owed: %w", err)
		}
		k := key{member: member}
		if conn != nil {
			k.connection = *conn
		}
		g, ok := groups[k]
		if !ok {
			g = &followUp{member: member, connection: k.connection, provider: provider, thread: thread, byBatch: map[ids.UUID]*pickItem{}}
			groups[k] = g
			order = append(order, g)
		}
		g.raws = append(g.raws, raw)
		item, ok := g.byBatch[batch]
		if !ok {
			item = &pickItem{At: at, Excerpt: body}
			g.byBatch[batch] = item
			g.batches = append(g.batches, item)
		}
		item.Count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the follow-up questions owed: %w", err)
	}
	return flagged(ctx, tx, order)
}

// flagged keeps the groups with at least one message that carries the flag.
func flagged(ctx context.Context, tx pgx.Tx, groups []*followUp) ([]*followUp, error) {
	var out []*followUp
	for _, g := range groups {
		var owed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM raw_capture WHERE id = ANY($1) AND ask_followup_at IS NOT NULL)`, g.raws).Scan(&owed); err != nil {
			return nil, fmt.Errorf("capture: reading the follow-up questions owed: %w", err)
		}
		if owed {
			out = append(out, g)
		}
	}
	return out, nil
}
