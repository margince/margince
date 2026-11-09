// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// An attempt sends a body it read earlier, with no transaction open, so an
// erasure can commit in between. deliverOnce, the only sender, re-checks the
// row through sendGuard right before the body goes out.

// errSendWithdrawn is an attempt the row no longer allows: the delivery was
// revoked or erased, another sender holds its claim, or its body changed.
var errSendWithdrawn = errors.New("webhooks: the delivery was withdrawn before its body was sent")

// errSendUnchecked is an attempt whose row could not be read. Nothing is sent.
var errSendUnchecked = errors.New("webhooks: the delivery could not be re-checked before its body was sent")

// sendUncheckedReason is what the row says after errSendUnchecked. Fixed text,
// because last_error is shown to the operator; the cause goes to the log.
const sendUncheckedReason = "the delivery could not be re-checked before sending, so this attempt was not sent"

// stillToSend reports whether the row still allows sending t's body. It must
// be pending or retrying, with the same uncleared body. Its claim must still
// be t's (none for a first attempt).
//
// FOR SHARE waits for an erasure that wrote the row but has not committed.
// The lock ends with this short transaction, before anything is sent.
func (s *Store) stillToSend(ctx context.Context, t attemptTarget) (bool, error) {
	digest := sha256.Sum256(t.payload)
	var ok bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT status IN ('pending', 'retrying') AND payload_pruned_at IS NULL
			       AND sha256(convert_to(payload, 'UTF8')) = $2
			       AND claimed_until IS NOT DISTINCT FROM $3
			FROM webhook_delivery WHERE id = $1
			FOR SHARE`, t.deliveryID, digest[:], t.claim).Scan(&ok)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

// sendGuard is the check deliverOnce hands to attempt. The check gets its own
// time bound, because it runs inside the transport, where the client's
// timeout does not reach the database call.
func (d *Deliverer) sendGuard(ctx context.Context, t attemptTarget) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
		defer cancel()
		ok, err := d.store.stillToSend(ctx, t)
		if err != nil {
			return fmt.Errorf("%w: %w", errSendUnchecked, err)
		}
		if !ok {
			return errSendWithdrawn
		}
		return nil
	}
}

// guardedBody runs its check before the transport reads its first byte. A
// failed check fails the read, and the transport closes the connection
// without writing any of the body.
//
// The check runs at the first read, not before client.Do. Before client.Do,
// the dial and TLS handshake would still follow it, which can take seconds.
// Headers do leave before a refused read: Go flushes them first for a body of
// this type. They carry no record data.
//
// The remaining window is from the check's commit to the body's socket write.
// That is a memory copy, with no network round trip and no lock wait. An
// erasure committing inside it commits after the data left.
type guardedBody struct {
	check   func() error
	body    *bytes.Reader
	checked bool
}

func (g *guardedBody) Read(p []byte) (int, error) {
	if !g.checked {
		if err := g.check(); err != nil {
			return 0, err
		}
		g.checked = true
	}
	return g.body.Read(p)
}

func (*guardedBody) Close() error { return nil }

// withdrawnOutcome turns a refused check into the attempt's outcome, or
// reports that the error was not the check's.
func withdrawnOutcome(err error) (outcome, bool) {
	switch {
	case errors.Is(err, errSendWithdrawn):
		return outcome{withdrawn: true}, true
	case errors.Is(err, errSendUnchecked):
		return outcome{failure: sendUncheckedReason}, true
	}
	return outcome{}, false
}

// claimedAt is the claim a target holds, for the log.
func claimedAt(t attemptTarget) string {
	if t.claim == nil {
		return "none"
	}
	return t.claim.Format(time.RFC3339Nano)
}
