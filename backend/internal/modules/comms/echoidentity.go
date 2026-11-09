// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Recovering a sent message's provider-stamped identity when the read-back
// after the send failed. The provider's captured copy of the message (its
// echo) carries that identity. Whichever of the receipt and the echo commits
// second re-keys the send, and nothing is sent again.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// EchoIdentity answers the identity of the captured echo of one sent message,
// found by the provider's message id in the sending seat's own mailbox. capture
// owns the import rows, so compose injects it.
// sentAt bounds the match to the account the connection held when the message
// was sent.
type EchoIdentity func(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, providerMessageID string, sentAt time.Time) (string, bool, error)

// WithEchoIdentity hands the store the read that recovers an identity the
// read-back could not.
func WithEchoIdentity(read EchoIdentity) StoreOption {
	return func(s *Store) { s.echoIdentity = read }
}

// errIdentityReadDenied is the breadcrumb for a mailbox that refused to read
// the sent message back. No later read answers either, so the message keeps
// its staged identity.
// sendIdentityActor is who an echo's adoption of its send's identity is
// recorded as: the send worker, whose reconcile the adoption completes.
const sendIdentityActor = "system:comms-send"

var errIdentityReadDenied = errors.New("comms: read_denied: the mailbox refused to read the sent message back, so its identity stays unknown")

// sentReceipt is what commitReceipt read back from the row it closed.
type sentReceipt struct {
	activity       ids.ActivityID
	staged         string
	seat           ids.UUID
	provider       string
	providerSentID string
	sentAt         time.Time
}

// identityAfterReceipt is the stamped identity to reconcile onto. A denied
// read-back is recorded and owes nothing more. A transient one looks for an
// echo already captured. Without one, the echo's capture adopts it later.
func (s *Store) identityAfterReceipt(ctx context.Context, delivery ids.UUID, sent sentReceipt, receipt connector.SendReceipt) string {
	switch receipt.IdentityUnread {
	case connector.IdentityUnreadDenied:
		s.breadcrumb(ctx, delivery, sent.staged, "", errIdentityReadDenied)
		return ""
	case connector.IdentityUnreadTransient:
		stamped, err := s.capturedEchoIdentity(ctx, sent)
		if err != nil {
			s.breadcrumb(ctx, delivery, sent.staged, "", err)
		}
		return stamped
	}
	return receipt.RFC822MessageID
}

// capturedEchoIdentity reads the echo under the sent-echo lock, so an echo
// capture still in flight is waited for rather than missed.
func (s *Store) capturedEchoIdentity(callerCtx context.Context, sent sentReceipt) (string, error) {
	if s.echoIdentity == nil || sent.providerSentID == "" {
		return "", nil
	}
	ctx, cancel := detachedWrite(callerCtx)
	defer cancel()
	var stamped string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockSentEchoKey(ctx, tx, sent.seat, sent.provider, sent.providerSentID); err != nil {
			return err
		}
		var err error
		stamped, _, err = s.echoIdentity(ctx, tx, sent.seat, sent.provider, sent.providerSentID, sent.sentAt)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("comms: looking for the sent message's captured echo: %w", err)
	}
	return stamped, nil
}

// AdoptEchoIdentityTx re-keys the seat's sent message onto its echo's identity,
// in the capture transaction, before the echo is written. The echo then lands
// on the send's natural key as a replay. It answers whether a send moved.
//
// The caller holds storekit.LockSentEchoKey. The lookup is scoped to the seat
// and provider, so another mailbox's message with the same id is untouched.
// A provider id names a message only within one account. A delivery sent
// before the connection's current account was bound (accountSince) never matches.
func (s *Store) AdoptEchoIdentityTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, providerMessageID, stamped string, accountSince *time.Time) (bool, error) {
	if providerMessageID == "" || !connector.ValidMessageID(stamped) {
		return false, nil
	}
	var delivery ids.UUID
	var activity ids.ActivityID
	var staged string
	err := tx.QueryRow(ctx, `
		SELECT id, activity_id, message_id FROM comms_outbound
		 WHERE user_id = $1 AND provider = $2 AND provider_message_id = $3
		   AND status = 'sent' AND channel_user_id IS NULL AND message_id <> $4
		   AND ($5::timestamptz IS NULL OR sent_at >= $5)
		 ORDER BY sent_at DESC, id LIMIT 1`, seat, provider, providerMessageID, stamped, accountSince).Scan(&delivery, &activity, &staged)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("comms: finding the send a captured echo belongs to: %w", err)
	}
	if s.identity == nil {
		return false, errNoReconciler
	}
	// The re-key is the send's bookkeeping, recorded as the send worker records
	// its own reconcile, not as a write of the mailbox owner's.
	return true, s.reKey(principal.SystemActing(ctx, sendIdentityActor), tx, delivery, activity, staged, stamped)
}
