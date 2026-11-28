// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A shared mailbox's reader team can reply as the mailbox once an admin turns
// replies on. A reply is one more send through comms. The pressing seat is the
// sender on the delivery, the audit row and captured_by. Only the From address
// and the credential are the mailbox's. The admin's choice is stored as the
// send scope on provider_scopes, which the dispatcher's authority gate already
// asks of every connection.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ErrSharedReplyRefused marks a send as a shared mailbox by a seat that no
// longer reads it, or for a mailbox no longer allowed to send. Both are facts
// about the mailbox, so a caller stops instead of retrying.
var ErrSharedReplyRefused = errors.New("capture: this seat may not reply as that shared mailbox")

// SharedReplyUnsupportedError refuses replies for a provider that cannot send
// as a shared mailbox.
type SharedReplyUnsupportedError struct{ Provider string }

func (e *SharedReplyUnsupportedError) Error() string {
	return fmt.Sprintf("a shared %s mailbox only captures mail; connect it through Microsoft or Google to reply as it", e.Provider)
}

// FieldFault answers the refusal on the switch, under the code the connect
// uses for a shared mailbox with SMTP settings.
func (e *SharedReplyUnsupportedError) FieldFault() (field, code, message string) {
	return fieldReplyEnabled, "shared_mailbox_cannot_send", e.Error()
}

// sharedSender is the provider's way of sending as a shared mailbox, when it
// has one.
//
//nolint:ireturn // returns the connector port's own seam
func (r *Registry) sharedSender(provider string) (connector.SharedMailboxSender, error) {
	c, err := r.connector(provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectorNotConfigured, err)
	}
	sender, ok := c.(connector.SharedMailboxSender)
	if !ok {
		return nil, &SharedReplyUnsupportedError{Provider: provider}
	}
	return sender, nil
}

// sharedSendScope is the scope an admin's yes stores for provider.
func (r *Registry) sharedSendScope(provider string) (string, error) {
	sender, err := r.sharedSender(provider)
	if err != nil {
		return "", err
	}
	return sender.SharedSendScope(), nil
}

// changeSharedReplyTx turns replies as the mailbox on or off. The scope is the
// admin's attestation that the installation's grant for this mailbox can send.
// A provider that disagrees refuses the send, and the delivery parks with that
// reason.
func changeSharedReplyTx(ctx context.Context, tx pgx.Tx, before SharedMailboxView, enabled bool, sendScope func(string) (string, error)) (SharedMailboxView, error) {
	scopes := []string{}
	if enabled {
		scope, err := sendScope(before.Provider)
		if err != nil {
			return before, err
		}
		scopes = append(scopes, scope)
	}
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET provider_scopes = $2 WHERE id = $1`, before.ID, scopes)
	if err != nil {
		return before, fmt.Errorf("capture: changing the replies of the shared mailbox: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return before, fmt.Errorf("the shared mailbox changed under this switch: %w", apperrors.ErrConflict)
	}
	next := before
	next.ReplyEnabled = enabled
	return next, announceSharedMailboxTx(ctx, tx, auditVerbUpdate, sharedChangeReply, next.ID,
		withReplyImage(sharedMailboxImage(before), before.ReplyEnabled), withReplyImage(sharedMailboxImage(next), enabled))
}

func withReplyImage(image map[string]any, enabled bool) map[string]any {
	image[fieldReplyEnabled] = enabled
	return image
}

// sharedReply is a shared mailbox the seat may reply as.
type sharedReply struct {
	provider string
	scopes   []string
	label    *string
	ref      *string
	auth     []byte
}

// sharedReplyTx reads the shared mailbox id when the seat is a live member of
// its live reader team and replies are on. Anything else is ok == false, with
// no hint of which part failed, so a seat outside the team learns nothing
// about the mailbox.
func sharedReplyTx(ctx context.Context, tx pgx.Tx, seat ids.UserID, id ids.UUID) (reply sharedReply, ok bool, err error) {
	err = tx.QueryRow(ctx, `SELECT c.provider, coalesce(c.provider_scopes, '{}'), c.account_label, c.credential_ref, c.auth
	  FROM capture_connection c
	 WHERE c.id = $1 AND c.user_id IS NULL AND c.status IN ('connected','error')
	   AND cardinality(coalesce(c.provider_scopes, '{}')) > 0
	   AND `+auth.SeatReadsSharedMailboxSQL("c", "$2"), id, seat).
		Scan(&reply.provider, &reply.scopes, &reply.label, &reply.ref, &reply.auth)
	if errors.Is(err, pgx.ErrNoRows) {
		return sharedReply{}, false, nil
	}
	return reply, err == nil, err
}

// ReplyableSharedMailbox finds the shared mailbox from names when the seat may
// reply as it. An unnamed choice never reaches a shared mailbox, so a member's
// own default is never replaced by a team address. Only a signed-in member
// replies as a team: an agent naming the id gets the answer of an unknown one.
func (r *Registry) ReplyableSharedMailbox(ctx context.Context, seat ids.UserID, from ids.UUID) (SendingMailbox, bool, error) {
	if actor, ok := principal.Actor(ctx); from.IsZero() || !ok || actor.Type != principal.PrincipalHuman {
		return SendingMailbox{}, false, nil
	}
	var found SendingMailbox
	var ok bool
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		reply, replyable, err := sharedReplyTx(ctx, tx, seat, from)
		if err != nil || !replyable {
			return err
		}
		found = SendingMailbox{ID: from, Provider: reply.provider, AccountLabel: reply.label, ProviderScopes: reply.scopes, Shared: true}
		ok = true
		return nil
	})
	if err != nil {
		return SendingMailbox{}, false, fmt.Errorf("capture: reading the shared mailbox a seat replies as: %w", err)
	}
	return found, ok, nil
}

// sharedSenderFor is the credential a reply as the shared mailbox transmits
// with. The seat's membership is asked again here, at transmit time, because a
// reply may wait in the queue while the seat leaves the team.
//
//nolint:ireturn // returns the connector port's own seam
func (r *Registry) sharedSenderFor(ctx context.Context, seat ids.UserID, id ids.UUID) (connector.EmailSender, connector.Auth, []string, error) {
	var reply sharedReply
	var replyable bool
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		reply, replyable, err = sharedReplyTx(ctx, tx, seat, id)
		return err
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("capture: resolving the shared mailbox a reply goes out from: %w", err)
	}
	if !replyable {
		return nil, nil, nil, ErrSharedReplyRefused
	}
	sender, err := r.sharedSender(reply.provider)
	if err != nil {
		return nil, nil, nil, err
	}
	bundle, err := r.resolveCredential(ctx, reply.ref, reply.auth)
	if err != nil {
		return nil, nil, nil, err
	}
	opened, state, err := sender.OpenSharedSender(bundle)
	if err != nil {
		return nil, nil, nil, err
	}
	return opened, state, slices.Clone(reply.scopes), nil
}

// namesSharedMailbox reports whether a send names a shared mailbox, which no
// member owns, so the member lookups below never find it.
func (r *Registry) namesSharedMailbox(ctx context.Context, mailbox Mailbox) (bool, error) {
	if mailbox.ID.IsZero() {
		return false, nil
	}
	var shared bool
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		shared, err = IsSharedMailboxTx(ctx, tx, mailbox.ID)
		return err
	}); err != nil {
		return false, fmt.Errorf("capture: resolving the mailbox a send names: %w", err)
	}
	return shared, nil
}

// IsSharedMailboxTx reports whether id is a shared mailbox, one no seat owns.
func IsSharedMailboxTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var shared bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM capture_connection WHERE id = $1 AND user_id IS NULL)`, id).Scan(&shared)
	return shared, err
}

// auditSendVerb is the audit verb of one message sent as a mailbox.
const auditSendVerb = "send_email"

// AuditSharedReplyTx writes the mailbox's own audit row for one reply sent as
// it, naming the seat that pressed send, the activity and the delivery. A
// connection a member owns writes nothing: the activity's own row already
// names its sender. It runs in the staging transaction, so the row exists
// when the delivery does.
func AuditSharedReplyTx(ctx context.Context, tx pgx.Tx, connection ids.UUID, activity ids.ActivityID, delivery ids.UUID) error {
	if connection.IsZero() {
		return nil
	}
	shared, err := IsSharedMailboxTx(ctx, tx, connection)
	if err != nil || !shared {
		return err
	}
	var address string
	if err := tx.QueryRow(ctx, `SELECT account_label FROM capture_connection WHERE id = $1`, connection).Scan(&address); err != nil {
		return fmt.Errorf("capture: reading the shared mailbox a reply goes out from: %w", err)
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if _, err := storekit.AuditEvent(ctx, tx, auditSendVerb, captureConnectionObject, connection, map[string]any{
		"activity_id": activity, "delivery_id": delivery, "sent_by": actor.UserID, "from": address,
	}); err != nil {
		return fmt.Errorf("capture: auditing the reply sent as shared mailbox %s: %w", connection, err)
	}
	return nil
}

// ReplyableSharedMailboxes lists the live shared mailboxes the acting member
// may reply as, by address. A mailbox the member may not use is absent.
func (r *Registry) ReplyableSharedMailboxes(ctx context.Context) ([]SendingMailbox, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, fmt.Errorf("only a signed-in member lists the mailboxes they reply as: %w", apperrors.ErrPermissionDenied)
	}
	var out []SendingMailbox
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.id, c.provider, c.account_label, c.provider_scopes FROM capture_connection c
		 WHERE c.user_id IS NULL AND c.status IN ('connected','error')
		   AND cardinality(coalesce(c.provider_scopes, '{}')) > 0
		   AND `+auth.SeatReadsSharedMailboxSQL("c", "$1")+` ORDER BY lower(c.account_label)`, actor.UserID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SendingMailbox, error) {
			m := SendingMailbox{Shared: true}
			return m, row.Scan(&m.ID, &m.Provider, &m.AccountLabel, &m.ProviderScopes)
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing the shared mailboxes a member replies as: %w", err)
	}
	return out, nil
}

// RecheckSharedReply asks again, just before a reply is handed to the provider,
// whether seat may still reply as connection. It answers nil for a connection
// no shared mailbox owns. It answers ErrSharedReplyRefused once the seat left
// the team, the team was archived, the mailbox disconnected or replies were
// turned off. The earlier credential lookup cannot know of a change committed
// after it.
func (r *Registry) RecheckSharedReply(ctx context.Context, seat ids.UserID, connection ids.UUID) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		shared, err := IsSharedMailboxTx(ctx, tx, connection)
		if err != nil || !shared {
			return err
		}
		_, replyable, err := sharedReplyTx(ctx, tx, seat, connection)
		if err != nil {
			return err
		}
		if !replyable {
			return ErrSharedReplyRefused
		}
		return nil
	})
}
