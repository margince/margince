// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingConn counts the request bytes a receiver read.
type countingConn struct {
	net.Conn
	read *atomic.Int64
}

func (c countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, err
}

// rawReceiver is a TLS listener that records the request bytes each
// connection carried. In early mode it answers 200 the moment a connection
// is up, before it reads anything; otherwise it answers status after reading
// the whole request.
type rawReceiver struct {
	ln        net.Listener
	tlsConfig *tls.Config
	mu        sync.Mutex
	early     bool
	status    int
	conns     []*atomic.Int64
	bodies    []string
	done      sync.WaitGroup
}

func newRawReceiver(t *testing.T) *rawReceiver {
	t.Helper()
	cert := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(cert.Close)
	transport, ok := cert.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the test server's client has no *http.Transport")
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cert.TLS)
	if err != nil {
		t.Fatal(err)
	}
	r := &rawReceiver{ln: ln, tlsConfig: transport.TLSClientConfig.Clone(), status: http.StatusOK}
	r.tlsConfig.ServerName = "example.com"
	go r.serve()
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Log(err)
		}
		r.done.Wait()
	})
	return r
}

func (r *rawReceiver) serve() {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return
		}
		r.mu.Lock()
		read := &atomic.Int64{}
		r.conns = append(r.conns, read)
		early, status := r.early, r.status
		r.done.Add(1)
		r.mu.Unlock()
		go r.handle(countingConn{Conn: conn, read: read}, early, status)
	}
}

func (r *rawReceiver) handle(conn countingConn, early bool, status int) {
	defer r.done.Done()
	defer func() {
		//craft:ignore swallowed-errors closing a finished test exchange; the bytes it carried are already counted
		_ = conn.Close()
	}()
	if early {
		//craft:ignore swallowed-errors a refused attempt may close before this answer lands; the byte count is the check
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		//craft:ignore swallowed-errors the client's close is what ends the copy; the byte count is the check
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return
	}
	r.mu.Lock()
	r.bodies = append(r.bodies, string(body))
	r.mu.Unlock()
	//craft:ignore swallowed-errors the attempt's recorded outcome is the check, not this write
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", status, http.StatusText(status))
}

func (r *rawReceiver) set(early bool, status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.early, r.status = early, status
}

// bytesSince is the request bytes read on connections after the first n.
func (r *rawReceiver) bytesSince(n int) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, c := range r.conns[n:] {
		total += c.Load()
	}
	return total
}

func (r *rawReceiver) connCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conns)
}

func (r *rawReceiver) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies...)
}

type erasureRace struct {
	e          *integration.Env
	subject    ids.UUID
	subID      ids.UUID
	receiver   *rawReceiver
	deliverer  *webhooks.Deliverer
	now        time.Time
	mu         sync.Mutex
	beforeDial func()
}

// arm runs hook once, in the next attempt's dial: after the attempt loaded
// its body and before its connection is checked.
func (r *erasureRace) arm(hook func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeDial = hook
}

func (r *erasureRace) dial(ctx context.Context, network, _ string) (net.Conn, error) {
	r.mu.Lock()
	hook := r.beforeDial
	r.beforeDial = nil
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return (&net.Dialer{}).DialContext(ctx, network, r.receiver.ln.Addr().String())
}

func setupErasureRace(t *testing.T) *erasureRace {
	t.Helper()
	e := integration.Setup(t)
	r := &erasureRace{
		e: e, subject: keyedRecipient(t, e, "Raced Subject", "raced@webhook.test"),
		receiver: newRawReceiver(t), now: homeReadTime,
	}
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	store := webhooks.NewStore(e.DB(), cipher)
	sub, _, err := store.CreateSubscription(r.webhookAdmin(), webhooks.CreateSubscriptionInput{
		TargetURL: "https://hooks.example/contacts", EventTypes: []string{"contact.updated"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.subID = sub.ID
	client := webhooks.NewDeliveryClient(r.dial, r.receiver.tlsConfig)
	r.deliverer = webhooks.NewDeliverer(store, client, func() time.Time { return r.now }, agendaWebhookAuthority{}, slog.Default())
	return r
}

func (r *erasureRace) webhookAdmin() context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["webhook_subscription"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return r.e.As(r.e.AdminUser, nil, perms)
}

func (r *erasureRace) eraseSubject(t *testing.T) func() {
	return func() {
		if err := ComposedEraser(r.e.Pool, nil, nil).EraseContact(r.e.Admin(), r.subject, "test"); err != nil {
			t.Error(err)
		}
	}
}

func (r *erasureRace) contactUpdated(t *testing.T) {
	t.Helper()
	if err := r.deliverer.HandleEvent(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: "contact.updated", Version: 1, OccurredAt: homeReadTime,
		Actor:  kevents.Actor{Type: "system", ID: "system"},
		Entity: kevents.EntityRef{Type: "contact", ID: r.subject}, Payload: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
}

// parkAsRetrying makes the first attempt fail with a 500, so the delivery
// waits as a retry whose backoff has elapsed.
func (r *erasureRace) parkAsRetrying(t *testing.T) {
	t.Helper()
	r.receiver.set(false, http.StatusInternalServerError)
	r.contactUpdated(t)
	r.receiver.set(false, http.StatusOK)
	r.now = r.now.Add(time.Minute)
	if state := r.deliveryState(t); state != "retrying:false" {
		t.Fatalf("the failed first attempt left the delivery %s, want retrying", state)
	}
}

func (r *erasureRace) sweep(t *testing.T) {
	t.Helper()
	if err := r.deliverer.SweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (r *erasureRace) replay(t *testing.T, delivery ids.UUID) {
	t.Helper()
	if _, err := r.deliverer.Replay(r.webhookAdmin(), r.subID, delivery); err != nil {
		t.Fatal(err)
	}
}

func (r *erasureRace) deliveryState(t *testing.T) string {
	t.Helper()
	return r.e.WsScalar(t, `SELECT status || ':' || (payload = '')::text FROM webhook_delivery WHERE subscription_id = $1`, r.subID)
}

// assertNothingSentSince fails if a connection opened after the first n
// carried any request byte, or if the delivery is not cancelled and cleared.
func (r *erasureRace) assertNothingSentSince(t *testing.T, n int) {
	t.Helper()
	if r.receiver.connCount() <= n {
		t.Fatal("the attempt opened no connection, so it proves nothing about what a connection carries")
	}
	if got := r.receiver.bytesSince(n); got != 0 {
		t.Fatalf("the receiver read %d request bytes after the erasure, want 0", got)
	}
	if state := r.deliveryState(t); state != "visibility_revoked:true" {
		t.Fatalf("the delivery reads %s, want visibility_revoked with its body cleared", state)
	}
}

// An erasure that commits between loading a body and sending it stops the
// send on the first attempt, the retry sweep and a replay. The receiver
// answers 200 before reading anything, and still reads no byte, and the
// delivery is not recorded as delivered.
func TestAnErasureBetweenLoadingAWebhookBodyAndSendingItStopsTheSend(t *testing.T) {
	for name, send := range map[string]func(t *testing.T, r *erasureRace){
		"the first attempt": func(t *testing.T, r *erasureRace) {
			r.contactUpdated(t)
		},
		"a retry": func(t *testing.T, r *erasureRace) {
			r.sweep(t)
		},
		"a replay": func(t *testing.T, r *erasureRace) {
			r.replay(t, ids.MustParse(r.e.WsScalar(t, `SELECT id::text FROM webhook_delivery WHERE subscription_id = $1`, r.subID)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := setupErasureRace(t)
			if name != "the first attempt" {
				r.parkAsRetrying(t)
			}
			before := r.receiver.connCount()
			r.receiver.set(true, http.StatusOK)
			r.arm(r.eraseSubject(t))
			send(t, r)
			r.assertNothingSentSince(t, before)
		})
	}
}

// With no erasure, the first attempt and a retry deliver the body.
func TestAWebhookWithNoErasureIsDelivered(t *testing.T) {
	for name, send := range map[string]func(t *testing.T, r *erasureRace){
		"the first attempt": func(t *testing.T, r *erasureRace) { r.contactUpdated(t) },
		"a retry": func(t *testing.T, r *erasureRace) {
			r.parkAsRetrying(t)
			r.sweep(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := setupErasureRace(t)
			send(t, r)
			got := r.receiver.received()
			if len(got) == 0 || !strings.Contains(got[len(got)-1], r.subject.String()) {
				t.Fatalf("the receiver read %q, want a body about the subject", got)
			}
			if state := r.deliveryState(t); state != "delivered:false" {
				t.Fatalf("the delivery reads %s, want delivered with its body kept", state)
			}
		})
	}
}

// An erasure that has written the delivery but not committed holds its row.
// The send check waits for it, reads the erased row once it commits, and
// refuses. The erasure commits only once the check is seen waiting.
func TestASendCheckWaitsForAnUncommittedErasureAndRefuses(t *testing.T) {
	r := setupErasureRace(t)
	owner := integration.OwnerConn(t)
	committed := make(chan error, 1)
	r.arm(func() {
		locked, release := make(chan struct{}), make(chan struct{})
		go func() {
			ctx := principal.WithWorkspaceID(context.Background(), r.e.WS)
			committed <- database.WithWorkspaceTx(ctx, r.e.Pool, func(tx pgx.Tx) error {
				if err := webhooks.ScrubSubjectDeliveriesTx(ctx, tx, "contact", []ids.UUID{r.subject}); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			})
		}()
		<-locked
		go func() {
			defer close(release)
			waitForALockWaiter(t, owner)
		}()
	})
	r.receiver.set(true, http.StatusOK)
	r.contactUpdated(t)
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	r.assertNothingSentSince(t, 0)
}

// waitForALockWaiter returns once some session in this database waits on a
// row lock. Its bound makes a check that never blocks fail the test instead
// of hanging it.
func waitForALockWaiter(t *testing.T, owner *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := owner.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting); err != nil {
			t.Errorf("no session came to wait on the erasure's row lock: %v", err)
			return
		}
		if waiting > 0 {
			return
		}
	}
}
