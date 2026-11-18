// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SendingMailbox is one of a member's live connections a mail may go out from.
type SendingMailbox struct {
	ID             ids.UUID
	Provider       string
	AccountLabel   *string
	ProviderScopes []string
}

// SendingMailboxes lists userID's live connections of providers in the order
// they first connected them. The send pre-flight asks each in turn, so the
// first that can transmit is the default a new mail goes out from.
func (r *Registry) SendingMailboxes(ctx context.Context, userID ids.UserID, providers []string) ([]SendingMailbox, error) {
	var out []SendingMailbox
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, provider, account_label, provider_scopes FROM capture_connection
			 WHERE user_id = $1 AND provider = ANY($2) AND status IN ('connected','error') AND archived_at IS NULL
			 ORDER BY created_at, id`, userID, providers)
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

// ArrivalMailbox answers userID's live connection that delivered activity to
// them: the mailbox a reply to it goes out from. false when none still holds
// it, and the reply goes out from the default.
func (r *Registry) ArrivalMailbox(ctx context.Context, userID ids.UserID, activity ids.ActivityID) (ids.UUID, bool, error) {
	var id ids.UUID
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT c.id FROM capture_import ci
			  JOIN capture_connection c ON c.user_id = ci.user_id AND `+storekit.MailboxMarkSQL("c")+` = ANY(ci.delivered_by)
			 WHERE ci.activity_id = $1 AND ci.user_id = $2 AND c.status IN ('connected','error') AND c.archived_at IS NULL
			 ORDER BY c.created_at, c.id LIMIT 1`, activity, userID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("capture: reading the mailbox a message arrived in: %w", err)
	}
	return id, true, nil
}

// ChooseSendingMailbox picks the mailbox a send by userID goes out from, among
// their live mailboxes of providers that canSend. It is from when they named
// one, else the one anchor arrived in, else the first they connected. A from
// that is not theirs or cannot send chooses nothing. The send is then refused
// instead of leaving through a mailbox they did not ask for.
func (r *Registry) ChooseSendingMailbox(
	ctx context.Context, userID ids.UserID, from ids.UUID, anchor ids.ActivityID, providers []string,
	canSend func(SendingMailbox) (bool, error),
) (SendingMailbox, error) {
	all, err := r.SendingMailboxes(ctx, userID, providers)
	if err != nil {
		return SendingMailbox{}, err
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
		arrival, found, err := r.ArrivalMailbox(ctx, userID, anchor)
		if err != nil {
			return SendingMailbox{}, err
		}
		if mailbox, ok := capable[arrival]; found && ok {
			return mailbox, nil
		}
	}
	if first == nil {
		return SendingMailbox{}, nil
	}
	return *first, nil
}
