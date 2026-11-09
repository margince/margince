// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// An attempt sends a body it read earlier, with no transaction open, so an
// erasure can commit in between. deliverOnce, the only sender, has the row
// re-checked on the attempt's own connection before any request byte is written.

// errSendWithdrawn is an attempt the row no longer allows: the delivery was
// revoked or erased, another sender holds its claim, or its body changed.
var errSendWithdrawn = errors.New("webhooks: the delivery was withdrawn before it was sent")

// errSendUnchecked is an attempt whose row could not be read. Nothing is sent.
var errSendUnchecked = errors.New("webhooks: the delivery could not be re-checked before it was sent")

// errNoSendCheck is a dial for a request that carries no send check. The
// delivery client sends only attempts, and every attempt carries one.
var errNoSendCheck = errors.New("webhooks: a delivery connection was opened without a send check")

// The reasons last_error shows. Fixed text, because the operator reads it; the
// cause goes to the log.
const (
	sendUncheckedReason       = "the delivery could not be re-checked before sending, so this attempt was not sent"
	uncheckedConnectionReason = "the request did not go over a re-checked connection, so it does not count as delivered"
	earlyAnswerReason         = "the endpoint answered before the whole body was sent"
)

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

// sendGuard is the row check deliverOnce gives the attempt. It has its own
// time bound, because the dial context does not carry the request's.
func (d *Deliverer) sendGuard(ctx context.Context, t attemptTarget) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
		defer cancel()
		ok, err := d.store.stillToSend(ctx, t)
		if err != nil {
			d.log.Error("webhooks: re-checking a delivery before sending it", "delivery", t.deliveryID, "err", err)
			return fmt.Errorf("%w: %w", errSendUnchecked, err)
		}
		if !ok {
			return errSendWithdrawn
		}
		return nil
	}
}

const (
	checkNotRun int32 = iota
	checkPassed
	checkRefused
	checkUnchecked
)

// sendCheck is one attempt's row check, run by the connection it is sent on.
// A refusal sticks: no later answer from the transport overrides it.
type sendCheck struct {
	verify func() error
	state  atomic.Int32
}

func (c *sendCheck) run() error {
	err := c.verify()
	switch {
	case err == nil:
		c.state.CompareAndSwap(checkNotRun, checkPassed)
	case errors.Is(err, errSendWithdrawn):
		c.state.Store(checkRefused)
	default:
		c.state.Store(checkUnchecked)
	}
	return err
}

type sendCheckKey struct{}

func withSendCheck(ctx context.Context, check *sendCheck) context.Context {
	return context.WithValue(ctx, sendCheckKey{}, check)
}

// checkedDialer runs the request's send check once TCP and TLS are up. A
// refused check closes the connection before the request is written.
//
// The window left runs from the check's commit to the kernel taking the
// request. The request is written from memory at once, with no round trip.
type checkedDialer struct {
	dial      DialFunc
	tlsConfig *tls.Config
}

func (d checkedDialer) dialPlain(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.dial(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return checkConn(ctx, conn)
}

// dialTLS does the handshake itself, so the check runs after it. ALPN offers
// only HTTP/1.1.
func (d checkedDialer) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.dial(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if d.tlsConfig != nil {
		cfg = d.tlsConfig.Clone()
	}
	if cfg.ServerName == "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.Join(err, conn.Close())
		}
		cfg.ServerName = host
	}
	cfg.NextProtos = []string{"http/1.1"}
	tlsConn := tls.Client(conn, cfg)
	handshakeCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return checkConn(ctx, tlsConn)
}

func checkConn(ctx context.Context, conn net.Conn) (net.Conn, error) {
	err := errNoSendCheck
	if check, ok := ctx.Value(sendCheckKey{}).(*sendCheck); ok {
		err = check.run()
	}
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

// countedBody counts what the transport took of the body. An answer that
// arrives before the transport took all of it is not a delivery.
type countedBody struct {
	body *bytes.Reader
	read atomic.Int64
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	b.read.Add(int64(n))
	return n, err
}

// checkedOutcome decides an attempt from its check first. A refusal wins over
// any response. A 2xx counts only over a checked connection that took the body.
func checkedOutcome(check *sendCheck, body *countedBody, size int, res outcome) outcome {
	switch check.state.Load() {
	case checkRefused:
		return outcome{withdrawn: true}
	case checkUnchecked:
		return outcome{failure: sendUncheckedReason}
	case checkNotRun:
		if res.failure == "" {
			return outcome{statusCode: res.statusCode, failure: uncheckedConnectionReason}
		}
		return res
	}
	if res.failure == "" && body.read.Load() != int64(size) {
		return outcome{statusCode: res.statusCode, failure: earlyAnswerReason}
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
