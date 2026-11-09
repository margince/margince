// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bodyReceiver keeps every body it read in full, and answers the next status
// it was told to.
type bodyReceiver struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
	status int
}

func newBodyReceiver(t *testing.T) *bodyReceiver {
	r := &bodyReceiver{status: http.StatusOK}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		r.mu.Lock()
		defer r.mu.Unlock()
		if err == nil {
			r.bodies = append(r.bodies, string(body))
		}
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *bodyReceiver) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies...)
}

// erasingTransport is the delivery's real HTTP client, pointed at the
// receiver. Armed, it runs the erasure once inside Do: after the attempt has
// loaded its body and before the transport sends it.
type erasingTransport struct {
	client *http.Client
	target *url.URL
	mu     sync.Mutex
	erase  func()
}

func (e *erasingTransport) arm(erase func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.erase = erase
}

func (e *erasingTransport) Do(req *http.Request) (*http.Response, error) {
	e.mu.Lock()
	erase := e.erase
	e.erase = nil
	e.mu.Unlock()
	if erase != nil {
		erase()
	}
	req.URL.Scheme, req.URL.Host = e.target.Scheme, e.target.Host
	return e.client.Do(req)
}

type erasureRace struct {
	e         *integration.Env
	subject   ids.UUID
	subID     ids.UUID
	receiver  *bodyReceiver
	transport *erasingTransport
	deliverer *webhooks.Deliverer
	now       time.Time
}

func setupErasureRace(t *testing.T) *erasureRace {
	t.Helper()
	e := integration.Setup(t)
	r := &erasureRace{
		e: e, subject: keyedRecipient(t, e, "Raced Subject", "raced@webhook.test"),
		receiver: newBodyReceiver(t), now: homeReadTime,
	}
	target, err := url.Parse(r.receiver.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.transport = &erasingTransport{client: r.receiver.server.Client(), target: target}
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
	r.deliverer = webhooks.NewDeliverer(store, r.transport, func() time.Time { return r.now }, agendaWebhookAuthority{}, slog.Default())
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

// parkAsRetrying makes the first attempt fail, so the delivery waits as a
// retry whose backoff has elapsed. The failed attempt's body went out before
// any erasure, so the receiver forgets it.
func (r *erasureRace) parkAsRetrying(t *testing.T) ids.UUID {
	t.Helper()
	r.receiver.mu.Lock()
	r.receiver.status = http.StatusInternalServerError
	r.receiver.mu.Unlock()
	r.contactUpdated(t)
	r.receiver.mu.Lock()
	r.receiver.status, r.receiver.bodies = http.StatusOK, nil
	r.receiver.mu.Unlock()
	r.now = r.now.Add(time.Minute)
	return ids.MustParse(r.e.WsScalar(t, `SELECT id::text FROM webhook_delivery WHERE subscription_id = $1 AND status = 'retrying'`, r.subID))
}

func (r *erasureRace) deliveryState(t *testing.T) string {
	t.Helper()
	return r.e.WsScalar(t, `SELECT status || ':' || (payload = '')::text FROM webhook_delivery WHERE subscription_id = $1`, r.subID)
}

// An erasure that commits between loading a body and sending it stops the send.
// That holds for the first attempt, the retry sweep and a replay. With no
// erasure, the body goes out.
func TestAnErasureBetweenLoadingAWebhookBodyAndSendingItStopsTheSend(t *testing.T) {
	for name, tc := range map[string]struct {
		send  func(t *testing.T, r *erasureRace)
		erase bool
	}{
		"the first attempt": {erase: true, send: func(t *testing.T, r *erasureRace) {
			r.transport.arm(r.eraseSubject(t))
			r.contactUpdated(t)
		}},
		"a retry": {erase: true, send: func(t *testing.T, r *erasureRace) {
			r.parkAsRetrying(t)
			r.transport.arm(r.eraseSubject(t))
			if err := r.deliverer.SweepOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
		}},
		"a replay": {erase: true, send: func(t *testing.T, r *erasureRace) {
			delivery := r.parkAsRetrying(t)
			r.transport.arm(r.eraseSubject(t))
			if _, err := r.deliverer.Replay(r.webhookAdmin(), r.subID, delivery); err != nil {
				t.Fatal(err)
			}
		}},
		"a first attempt with no erasure": {send: func(t *testing.T, r *erasureRace) { r.contactUpdated(t) }},
		"a retry with no erasure": {send: func(t *testing.T, r *erasureRace) {
			r.parkAsRetrying(t)
			if err := r.deliverer.SweepOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			r := setupErasureRace(t)
			tc.send(t, r)
			got := r.receiver.received()
			if tc.erase {
				if len(got) != 0 {
					t.Fatalf("the receiver read %d body(ies) sent after the erasure committed: %q", len(got), got)
				}
				if state := r.deliveryState(t); state != "visibility_revoked:true" {
					t.Fatalf("the delivery reads %s, want visibility_revoked with its body cleared", state)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], r.subject.String()) {
				t.Fatalf("the receiver read %q, want the one body about the subject", got)
			}
			if state := r.deliveryState(t); state != "delivered:false" {
				t.Fatalf("the delivery reads %s, want delivered with its body kept", state)
			}
		})
	}
}
