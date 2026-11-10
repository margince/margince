// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// An attempt sends a body it read earlier, so an erasure can commit in
// between. deliverOnce, the only sender, has checkedTransport hold the row
// from a re-check until the request is written, and send nothing if refused.

// errSendWithdrawn is an attempt the row no longer allows: the delivery was
// revoked or erased, another sender holds its claim, or its body changed.
var errSendWithdrawn = errors.New("webhooks: the delivery was withdrawn before it was sent")

// errSendUnchecked is an attempt whose row could not be read. Nothing is sent.
var errSendUnchecked = errors.New("webhooks: the delivery could not be re-checked before it was sent")

// errNoSendCheck is a request that carries no send check. The delivery
// client sends only attempts, and every attempt carries one.
var errNoSendCheck = errors.New("webhooks: a delivery request was sent without a send check")

// The reasons last_error shows. Fixed text, because the operator reads it; the
// cause goes to the log.
const (
	sendUncheckedReason = "the delivery could not be re-checked before sending, so this attempt was not sent"
	unwrittenReason     = "the request was not fully written under a re-check, so it does not count as delivered"
)

// holdToSend re-checks the row and, if it still allows sending t's body,
// keeps it locked until release. The row must be pending or retrying, with the
// same uncleared body, and still carry t's claim (none for a first attempt).
//
// FOR SHARE waits for an erasure that wrote the row but has not committed, and
// makes a later erasure wait until release. The transaction runs in its own
// goroutine because the store opens one only around a callback.
func (s *Store) holdToSend(ctx context.Context, t attemptTarget) (func() error, error) {
	digest := sha256.Sum256(t.payload)
	checked, release := make(chan error, 1), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- s.db.Tx(ctx, func(tx pgx.Tx) error {
			var ok bool
			err := tx.QueryRow(ctx, `
				SELECT status IN ('pending', 'retrying') AND payload_pruned_at IS NULL
				       AND sha256(convert_to(payload, 'UTF8')) = $2
				       AND claimed_until IS NOT DISTINCT FROM $3
				FROM webhook_delivery WHERE id = $1
				FOR SHARE`, t.deliveryID, digest[:], t.claim).Scan(&ok)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !ok) {
				err = errSendWithdrawn
			}
			checked <- err
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	select {
	case err := <-checked:
		if err != nil {
			<-finished
			return nil, err
		}
		return func() error { close(release); return <-finished }, nil
	case err := <-finished:
		return nil, err
	}
}

// sendHold takes the row for one attempt. The release it returns ends the
// hold; it is nil when err is not.
type sendHold func() (release func(), err error)

// sendGuard is the hold deliverOnce gives each attempt. Its own time bound
// covers the check and the write, because the transport's context may not.
func (d *Deliverer) sendGuard(ctx context.Context, t attemptTarget) sendHold {
	return func() (func(), error) {
		ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
		release, err := d.holdToSend(ctx, t)
		if err != nil {
			cancel()
			if errors.Is(err, errSendWithdrawn) {
				return nil, err
			}
			d.log.Error("webhooks: re-checking a delivery before sending it", "delivery", t.deliveryID, "err", err)
			return nil, fmt.Errorf("%w: %w", errSendUnchecked, err)
		}
		return func() {
			defer cancel()
			if err := release(); err != nil {
				d.log.Error("webhooks: ending the hold on a sent delivery", "delivery", t.deliveryID, "err", err)
			}
		}, nil
	}
}

// holdToSend runs the test seam, if any, then takes the row.
func (d *Deliverer) holdToSend(ctx context.Context, t attemptTarget) (func() error, error) {
	if d.beforeSend != nil {
		if err := d.beforeSend(ctx); err != nil {
			return nil, err
		}
	}
	return d.store.holdToSend(ctx, t)
}

const (
	checkNotRun int32 = iota
	checkHeld
	checkWritten
	checkRefused
	checkUnchecked
)

// sendCheck is one attempt's hold and how far the request got under it. A
// refusal sticks: no answer from the receiver overrides it.
type sendCheck struct {
	hold sendHold
	// recheck re-asks whether the owner may still see the record, run UNDER the
	// hold and right before the body is written. attemptStillAuthorized answers
	// that question once, but the connection — DNS, TCP and the TLS handshake — is
	// built between that answer and the write, and a revocation that commits in
	// that window would otherwise still send. Nil leaves the hold as its only
	// gate (a replay with no subject to re-check, and the unit tests).
	recheck func() (bool, error)
	state   atomic.Int32
}

type sendCheckKey struct{}

func withSendCheck(ctx context.Context, check *sendCheck) context.Context {
	return context.WithValue(ctx, sendCheckKey{}, check)
}

// writeHeld writes the request while the row is held, and ends the hold once
// the write returns, before any response is read.
func (c *sendCheck) writeHeld(write func() error) error {
	release, err := c.hold()
	if err != nil {
		if errors.Is(err, errSendWithdrawn) {
			c.state.Store(checkRefused)
		} else {
			c.state.Store(checkUnchecked)
		}
		return err
	}
	defer release()
	// Under the hold, at the last moment before the body leaves: a revocation that
	// commits while the connection is being built must still stop the send. An
	// owner who can no longer see the record withdraws the attempt; an outage that
	// cannot answer counts as unchecked, exactly as the pre-send check treats each.
	if c.recheck != nil {
		switch ok, err := c.recheck(); {
		case err != nil:
			c.state.Store(checkUnchecked)
			return errors.Join(errSendUnchecked, err)
		case !ok:
			c.state.Store(checkRefused)
			return errSendWithdrawn
		}
	}
	c.state.Store(checkHeld)
	if err := write(); err != nil {
		return err
	}
	c.state.Store(checkWritten)
	return nil
}

// checkedOutcome decides an attempt from its check first. A refusal wins over
// any response. A 2xx counts only for a request written whole under the hold.
func checkedOutcome(check *sendCheck, res outcome) outcome {
	switch check.state.Load() {
	case checkRefused:
		return outcome{withdrawn: true}
	case checkUnchecked:
		return outcome{failure: sendUncheckedReason}
	case checkWritten:
		return res
	}
	if res.failure == "" {
		return outcome{statusCode: res.statusCode, failure: unwrittenReason}
	}
	return res
}

// claimedAt is the claim a target holds, for the log.
func claimedAt(t attemptTarget) string {
	if t.claim == nil {
		return "none"
	}
	return t.claim.Format(time.RFC3339Nano)
}
