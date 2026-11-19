// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Mailbox names one of a member's connections. A member may connect several
// accounts of one provider, so the id names the row. A caller that names only
// the provider reaches the row while the member holds just one.
type Mailbox struct {
	Provider string
	ID       ids.UUID
}

// ProviderMailbox names a member's only mailbox of provider.
func ProviderMailbox(provider string) Mailbox { return Mailbox{Provider: provider} }

// ErrMailboxAmbiguous refuses a provider-only address when the member holds
// more than one mailbox of that provider. The remedy is the connection id.
var ErrMailboxAmbiguous = fmt.Errorf("capture: you have more than one mailbox of this provider; name the one you mean by connection_id: %w",
	apperrors.ErrInvalidArgument)

// memberMailboxTx resolves mb to one of user's connection rows, withdrawn rows
// included, as every settings writer has always reached them. A provider-only
// address picks the one live row over withdrawn ones; two live rows, or two
// withdrawn ones and no live one, need the id.
func memberMailboxTx(ctx context.Context, tx pgx.Tx, user ids.UUID, mb Mailbox) (ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, status <> 'disconnected' FROM capture_connection
		 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL AND ($3::uuid IS NULL OR id = $3)`,
		user, mb.Provider, mailboxIDArg(mb))
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: finding the mailbox: %w", err)
	}
	var all, live []ids.UUID
	var id ids.UUID
	var connected bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &connected}, func() error {
		all = append(all, id)
		if connected {
			live = append(live, id)
		}
		return nil
	}); err != nil {
		return ids.Nil, fmt.Errorf("capture: finding the mailbox: %w", err)
	}
	switch {
	case len(live) == 1:
		return live[0], nil
	case len(live) == 0 && len(all) == 1:
		return all[0], nil
	case len(all) == 0:
		return ids.Nil, ErrNoConnection
	}
	return ids.Nil, ErrMailboxAmbiguous
}

func mailboxIDArg(mb Mailbox) *ids.UUID {
	if mb.ID.IsZero() {
		return nil
	}
	return &mb.ID
}

// resolveMemberMailbox is memberMailboxTx in its own transaction, for the
// settings writers that then address the row by id and owner.
func (r *Registry) resolveMemberMailbox(ctx context.Context, user ids.UUID, mb Mailbox) (connectionMatch, error) {
	var id ids.UUID
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = memberMailboxTx(ctx, tx, user, mb)
		return err
	})
	if err != nil {
		return connectionMatch{}, err
	}
	return memberConnection(user, id), nil
}

// seatMailboxTx answers the seat's connection a capture came through: the
// run's own row when ctx carries one, else the seat's only connection of
// provider. A seat holding several, with no run to say which, has no answer
// and ok is false, which every caller reads as its safe default.
func seatMailboxTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider string) (ids.UUID, bool, error) {
	if run, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection); bound {
		return run.ID, true, nil
	}
	id, err := memberMailboxTx(ctx, tx, seat, ProviderMailbox(provider))
	if errors.Is(err, ErrNoConnection) || errors.Is(err, ErrMailboxAmbiguous) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, err
	}
	return id, true, nil
}

// runConnection is the connection a sync, page or push runs for, and nil
// outside one. Statements filter a seat's mailboxes of a provider by it.
func runConnection(ctx context.Context) *ids.UUID {
	if run, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection); bound {
		return &run.ID
	}
	return nil
}

// MailboxNamed is the mailbox a request names: the provider, and the
// connection id when the request sent one.
func MailboxNamed[T ~[16]byte](provider string, connection *T) Mailbox {
	if connection == nil {
		return ProviderMailbox(provider)
	}
	return Mailbox{Provider: provider, ID: ids.UUID(*connection)}
}
