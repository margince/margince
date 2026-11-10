// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// agendaWebhookReceiver counts the deliveries each path was sent. Its client
// is the real delivery client, with every host dialing this one test server.
type agendaWebhookReceiver struct {
	server *httptest.Server
	mu     sync.Mutex
	paths  map[string]int
}

func newAgendaWebhookReceiver(t *testing.T) *agendaWebhookReceiver {
	t.Helper()
	r := &agendaWebhookReceiver{paths: map[string]int{}}
	r.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.paths[req.URL.Path]++
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.server.Close)
	return r
}

// client dials this server for any host. The test certificate names
// example.com, so the handshake verifies against that name.
func (r *agendaWebhookReceiver) client(t *testing.T) webhooks.HTTPDoer {
	t.Helper()
	transport, ok := r.server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the test server's client has no *http.Transport")
	}
	tlsConfig := transport.TLSClientConfig.Clone()
	tlsConfig.ServerName = "example.com"
	addr := r.server.Listener.Addr().String()
	return webhooks.NewDeliveryClient(func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}, tlsConfig)
}

func (r *agendaWebhookReceiver) sent(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paths[path]
}

func assertCaptureReviewDelivery(t *testing.T, e *integration.Env, approval ids.UUID) {
	t.Helper()
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	store := webhooks.NewStore(e.DB(), cipher)
	for _, seat := range []ids.UUID{e.Rep1, e.AdminUser} {
		perms := integration.AdminPerms
		perms.Objects = maps.Clone(perms.Objects)
		perms.Objects["webhook_subscription"] = principal.ObjectGrant{Create: true, Read: true}
		ctx := e.As(seat, nil, perms)
		if _, _, err := store.CreateSubscription(ctx, webhooks.CreateSubscriptionInput{TargetURL: "https://agenda.example/" + seat.String(), EventTypes: []string{"approval.requested"}}); err != nil {
			t.Fatal(err)
		}
	}
	receiver := newAgendaWebhookReceiver(t)
	deliverer := webhooks.NewDeliverer(store, receiver.client(t), func() time.Time { return homeReadTime }, agendaWebhookAuthority{}, slog.Default())
	event := kevents.Envelope{
		EventID: ids.NewV7(), Type: "approval.requested", Version: 1, OccurredAt: homeReadTime,
		Actor: kevents.Actor{Type: "system", ID: "system"}, Entity: kevents.EntityRef{Type: "approval", ID: approval}, Payload: []byte(`{}`),
	}
	if err := deliverer.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if receiver.sent("/"+e.Rep1.String()) != 1 || receiver.sent("/"+e.AdminUser.String()) != 0 {
		t.Fatalf("review deliveries: importer %d, admin %d; want only the importing member's one",
			receiver.sent("/"+e.Rep1.String()), receiver.sent("/"+e.AdminUser.String()))
	}
}

func TestCounterpartyStagingKeepsEachImportOwnersDecision(t *testing.T) {
	e := integration.Setup(t)
	svc := approvals.NewService(e.DB())
	activity := seedCapturedMail(t, e, "staging@agenda.example", "A possible contact")
	stage := func(owner ids.UUID, name string) ids.ApprovalID {
		t.Helper()
		body, err := json.Marshal(approvalsubject.Counterparty{OwnerID: owner, Email: "staging@agenda.example", DisplayName: name, ActivityID: activity})
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: approvalsubject.KindCounterparty, ProposedChange: body, DiffHash: hex.EncodeToString(digest[:]), TargetType: "activity", TargetID: activity, Identity: json.RawMessage(`{"email":"staging@agenda.example"}`), JoinPending: true})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := stage(e.Rep1, "Initial")
	second := stage(e.AdminUser, "Initial")
	if first == second {
		t.Fatal("two import owners joined one proposal")
	}
	if retry := stage(e.Rep1, "Initial"); retry != first {
		t.Fatal("same import retry did not join its pending proposal")
	}
	stage(e.AdminUser, "Revised")
	mine, err := svc.GetWire(e.As(e.Rep1, nil, integration.AdminPerms), first)
	if err != nil || mine.Status != "pending" {
		t.Fatalf("another import superseded the first decision: %v, %v", mine, err)
	}
}

// The delivery boundary resolves both subscribers to the same broad grants;
// responsibility must still narrow which one receives the persisted proposal.
type agendaWebhookAuthority struct{}

func (agendaWebhookAuthority) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: integration.AdminPerms}, nil
}

func (agendaWebhookAuthority) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

func (agendaWebhookAuthority) AdmittedAuthority(context.Context, ids.UUID, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authz.RBAC{}, "", apperrors.ErrPermissionDenied
}
