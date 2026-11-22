// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SendingMailbox is one of a member's live connections a mail may go out from.
type SendingMailbox struct {
	ID             ids.UUID
	Provider       string
	AccountLabel   *string
	ProviderScopes []string
}

// SendingMailboxes lists userID's live connections of providers: the one they
// marked as default first, then in the order they connected them. The send pre-flight asks each in turn, so the
// first that can transmit is the default a new mail goes out from.
func (r *Registry) SendingMailboxes(ctx context.Context, userID ids.UserID, providers []string) ([]SendingMailbox, error) {
	var out []SendingMailbox
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, provider, account_label, provider_scopes FROM capture_connection
			 WHERE user_id = $1 AND provider = ANY($2) AND status IN ('connected','error') AND archived_at IS NULL
			 ORDER BY default_sender DESC, created_at, id`, userID, providers)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SendingMailbox, error) {
			var m SendingMailbox
			err := row.Scan(&m.ID, &m.Provider, &m.AccountLabel, &m.ProviderScopes)
			return m, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing the mailboxes a send may go out from: %w", err)
	}
	return out, nil
}

// arrivalMailboxes lists userID's live connections that delivered activity to
// them, first connected first: the mailboxes a reply to it may go out from.
func (r *Registry) arrivalMailboxes(ctx context.Context, userID ids.UserID, activity ids.ActivityID) ([]ids.UUID, error) {
	var out []ids.UUID
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id FROM capture_connection c
			 WHERE c.user_id = $2 AND c.status IN ('connected','error') AND c.archived_at IS NULL
			   AND `+storekit.DeliveredBySQL("c", "$1::uuid")+`
			 ORDER BY c.created_at, c.id`, activity, userID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the mailboxes a message arrived in: %w", err)
	}
	return out, nil
}

// ChooseSendingMailbox picks the mailbox a send by userID goes out from, among
// their live mailboxes of providers that canSend. It is from when they named
// one, else the first that can send of those anchor arrived in, else their
// default. A from
// that is not theirs or cannot send chooses nothing. A member with no live
// mailbox of those providers at all answers ErrNoConnection. The send is then refused
// instead of leaving through a mailbox they did not ask for.
func (r *Registry) ChooseSendingMailbox(
	ctx context.Context, userID ids.UserID, from ids.UUID, anchor ids.ActivityID, providers []string,
	canSend func(SendingMailbox) (bool, error),
) (SendingMailbox, error) {
	all, err := r.SendingMailboxes(ctx, userID, providers)
	if err != nil {
		return SendingMailbox{}, err
	}
	if len(all) == 0 {
		return SendingMailbox{}, ErrNoConnection
	}
	capable := make(map[ids.UUID]SendingMailbox, len(all))
	var first *SendingMailbox
	for i := range all {
		can, err := canSend(all[i])
		if err != nil {
			return SendingMailbox{}, err
		}
		if can {
			capable[all[i].ID] = all[i]
			if first == nil {
				first = &all[i]
			}
		}
	}
	if !from.IsZero() {
		return capable[from], nil
	}
	if !anchor.IsZero() {
		arrivals, err := r.arrivalMailboxes(ctx, userID, anchor)
		if err != nil {
			return SendingMailbox{}, err
		}
		for _, arrival := range arrivals {
			if mailbox, ok := capable[arrival]; ok {
				return mailbox, nil
			}
		}
	}
	if first == nil {
		return SendingMailbox{}, nil
	}
	return *first, nil
}

// SetDefaultSender marks the caller's mailbox the one their new mail goes out
// from, and unmarks the one that was. A withdrawn mailbox sends nothing, so it
// cannot be marked.
func (r *Registry) SetDefaultSender(ctx context.Context, mailbox Mailbox) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return ConnectionView{}, fmt.Errorf("only a user or their agent chooses their sending mailbox: %w", apperrors.ErrPermissionDenied)
	}
	match, err := r.resolveMemberMailbox(ctx, actor.UserID, mailbox)
	if err != nil {
		return ConnectionView{}, err
	}
	err = r.db.Tx(ctx, func(tx pgx.Tx) error { return setDefaultSenderTx(ctx, tx, actor.UserID, match, mailbox.Provider) })
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: choosing the sending mailbox: %w", err)
	}
	return r.connectionFor(ctx, match.id)
}

// setDefaultSenderTx moves the member's mark to match. The member's rows are
// locked first, so two concurrent choices serialize instead of racing the
// one-mark index.
func setDefaultSenderTx(ctx context.Context, tx pgx.Tx, user ids.UUID, match connectionMatch, provider string) error {
	if _, err := tx.Exec(ctx, `SELECT id FROM capture_connection WHERE user_id = $1 ORDER BY id FOR UPDATE`, user); err != nil {
		return err
	}
	var before *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT (SELECT id FROM capture_connection WHERE user_id = $1 AND default_sender)`,
		user).Scan(&before); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_connection SET default_sender = false WHERE user_id = $1 AND default_sender`, user); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET default_sender = true
		 WHERE `+match.where+` AND status IN ('connected','error') AND archived_at IS NULL`, match.args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoConnection
	}
	return auditLifecycle(ctx, tx, "update", captureConnectionObject, match.id,
		map[string]any{auditKeyProvider: provider, "default_sender_was": before},
		map[string]any{auditKeyProvider: provider, "default_sender": match.id})
}
