// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writeWatchedConn runs onWrite before each write to the wire, TLS records
// included, and onRead before each read.
type writeWatchedConn struct {
	net.Conn
	onWrite, onRead func()
}

func (c writeWatchedConn) Write(p []byte) (int, error) {
	c.onWrite()
	return c.Conn.Write(p)
}

func (c writeWatchedConn) Read(p []byte) (int, error) {
	c.onRead()
	return c.Conn.Read(p)
}

// An erasure that starts after the check passed waits on the row until the
// request is written. At every write of the request the erasure is still
// blocked by the check's hold; it commits only after the hold ends.
func TestAnErasureThatStartsAfterTheCheckWaitsUntilTheRequestIsWritten(t *testing.T) {
	e := setupContractVis(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	sub, deliveries := e.seedDeliveries(t, 1)
	subject := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `UPDATE webhook_delivery SET entity_type = 'contact', entity_id = $2 WHERE id = $1`,
		deliveries[0], subject); err != nil {
		t.Fatal(err)
	}
	var gotBody atomic.Value
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	var checked, released, reading atomic.Bool
	var erasurePID int
	var violations atomic.Int64
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		// Every write after the check and before a read that follows the
		// release is the request; the erasure must still be blocked for each.
		return writeWatchedConn{Conn: conn, onWrite: func() {
			if checked.Load() && !reading.Load() && !erasureBlocked(t, e.owner, erasurePID) {
				violations.Add(1)
			}
		}, onRead: func() {
			if released.Load() {
				reading.Store(true)
			}
		}}, nil
	}
	cipher, err := NewCipher(make([]byte, WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDeliverer(NewStore(e.store.db, cipher), NewDeliveryClient(dial, transportTLS(t, receiver)),
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	target := attemptTarget{
		deliveryID: deliveries[0], subID: sub, targetURL: receiver.URL, sealedSecret: sealed,
		eventType: "contact.created", eventID: ids.NewV7(), payload: []byte(`{}`),
	}

	committed := make(chan error, 1)
	guard := d.sendGuard(ctx, target)
	hold := func() (func(), error) {
		release, err := guard()
		if err != nil {
			return nil, err
		}
		pid := make(chan int, 1)
		go func() {
			committed <- e.store.db.Tx(ctx, func(tx pgx.Tx) error {
				var backend int
				if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&backend); err != nil {
					return err
				}
				pid <- backend
				return ScrubSubjectDeliveriesTx(ctx, tx, "contact", []ids.UUID{subject})
			})
		}()
		erasurePID = <-pid
		waitUntilBlocked(t, e.owner, erasurePID, committed)
		checked.Store(true)
		return func() { released.Store(true); release() }, nil
	}

	if got := d.attempt(ctx, target, hold); got != (outcome{statusCode: http.StatusOK}) {
		t.Fatalf("attempt = %+v, want the body delivered", got)
	}
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	if n := violations.Load(); n != 0 {
		t.Fatalf("%d request write(s) ran after the erasure was free to commit", n)
	}
	if got, _ := gotBody.Load().(string); got != `{}` {
		t.Fatalf("receiver got %q, want the whole body", got)
	}
}

// erasureBlocked reports whether the erasure's session waits on another's lock.
func erasureBlocked(t *testing.T, owner *pgx.Conn, pid int) bool {
	t.Helper()
	if _, err := owner.Exec(context.Background(), `SELECT pg_stat_clear_snapshot()`); err != nil {
		t.Fatal(err)
	}
	var blocked bool
	if err := owner.QueryRow(context.Background(), `SELECT cardinality(pg_blocking_pids($1)) > 0`, pid).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	return blocked
}

// waitUntilBlocked returns once the erasure waits on the check's hold. An
// erasure that commits instead was never held back, which fails the test.
func waitUntilBlocked(t *testing.T, owner *pgx.Conn, pid int, committed <-chan error) {
	t.Helper()
	bound, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for bound.Err() == nil {
		select {
		case err := <-committed:
			t.Fatalf("the erasure committed (%v) while the check held the row", err)
		default:
		}
		if erasureBlocked(t, owner, pid) {
			return
		}
	}
	t.Fatal("the erasure never came to wait on the check's hold")
}
