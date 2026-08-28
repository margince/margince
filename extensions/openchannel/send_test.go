// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// registeredURL is where a member's own system listens. It is a real address in
// the fixtures only as far as the grammar goes — every test that transmits dials
// its own listener through the injected factory, because the production
// constructor refuses a loopback address by design.
const registeredURL = "https://hooks.example.com/crm"

// staged is the delivery the product hands over: a rep's reply, with the id it
// staged under and the account it resolved.
func staged() extension.OutboundMessage {
	return extension.OutboundMessage{
		Member:         ownerUserID,
		Recipient:      extension.ChannelIdentity{Provider: provider, ChannelUserID: "acct-77"},
		Body:           "On its way today.",
		IdempotencyKey: "5f0b7a91-2c34-4d68-91ae-7b3c0d5e6f28",
		Attempt:        1,
	}
}

// sendableEndpoint is the Runtime a send meets when the member's endpoint is
// open, enabled, addressed and minted.
func sendableEndpoint(url string) *fakeRuntime {
	rt := newRuntime().unattended()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, url, true)}
	rt.secrets.stored[rt.secrets.userKey(ownerUserID, inboundSecretKey)] = []byte(senderSecret)
	return rt
}

// receiver is one member's own system, standing in for whatever they registered.
// It records the one request it was posted and answers what the test scripted.
type receiver struct {
	got    *http.Request
	body   []byte
	status int
}

// listening starts the receiver and hands back the factory a send dials it
// through. The factory is the seam the production constructor's egress guard
// makes necessary: it refuses a loopback address, which is every test's own
// listener.
func listening(tb testing.TB, r *receiver) senderFactory {
	tb.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			tb.Errorf("reading the posted document: %v", err)
		}
		r.got, r.body = req, body
		if r.status == 0 {
			r.status = http.StatusAccepted
		}
		w.WriteHeader(r.status)
	}))
	tb.Cleanup(srv.Close)
	return func(string) (*sender, error) {
		return &sender{url: srv.URL, http: srv.Client()}, nil
	}
}

// fixedClock is the instant a send stamps its signature from. The receiver checks
// that timestamp for freshness and the signature covers it, so a real clock would
// make the material a test asserts about different on every run.
func fixedClock() func() time.Time {
	return func() time.Time { return signedAt }
}

// What leaves is verifiable by a receiver holding the member's secret: the
// assertion recomputes the outbound recipe over the posted bytes and the three
// headers, so a send that re-spelled its material would fail here exactly as it
// would fail at a receiver.
func TestASentMessageVerifiesAgainstTheOutboundRecipe(t *testing.T) {
	t.Parallel()
	got := &receiver{}
	rt := sendableEndpoint(registeredURL)
	if _, err := sendVia(context.Background(), rt, staged(), listening(t, got), fixedClock()); err != nil {
		t.Fatalf("sending: %v", err)
	}
	stamp, nonce, signature := sentSignature(t, got)
	if want := signatureOver([]byte(senderSecret), nonce, time.Unix(stamp, 0), got.body); want != signature {
		t.Fatalf("what left does not verify against the outbound recipe:\n%v", got.got.Header)
	}
}

// AND IT MUST NOT VERIFY AT OUR OWN DOOR. This connector signs what it sends
// with the same member secret it admits arrivals with, so a payload spelled the
// same in both directions would make every message it sends a valid message TO
// ITSELF: the party we send to is trusted to receive, not to speak as the
// sender, and relaying our own bytes back at our own edge is the cheapest
// forgery available to it — no secret required. The scope in the signed
// material is what makes those two different messages, and this is the
// assertion that fails if it is ever taken back out.
func TestASentMessageIsNotAValidArrivalAtOurOwnEdge(t *testing.T) {
	t.Parallel()
	got := &receiver{}
	rt := sendableEndpoint(registeredURL)
	if _, err := sendVia(context.Background(), rt, staged(), listening(t, got), fixedClock()); err != nil {
		t.Fatalf("sending: %v", err)
	}
	stamp, nonce, signature := sentSignature(t, got)
	relayed := extension.InboundRequest{
		Timestamp: time.Unix(stamp, 0),
		Nonce:     nonce,
		Body:      got.body,
		Signature: signature,
	}
	if verified([]byte(senderSecret), relayed) {
		t.Fatal("a message this connector SENT was admitted by the edge that guards what arrives — " +
			"any party we transmit to can relay it back and be authenticated as the sender")
	}
}

// advancingClock answers a later instant on every call, standing in for a real
// clock where marshalling and dialling take real time. It is what makes the
// single-call-vs-two-call difference observable: a fixed clock cannot, because
// every call to it agrees by construction.
func advancingClock(start time.Time) func() time.Time {
	next := start
	return func() time.Time {
		at := next
		next = next.Add(time.Minute)
		return at
	}
}

// The document's own OccurredAt and the instant the signature is stamped from
// must be the SAME clock read, or a receiver comparing the two sees a gap that
// grows with however long marshalling and dialling took.
func TestTheDocumentAndTheSignatureShareOneClockRead(t *testing.T) {
	t.Parallel()
	got := &receiver{}
	rt := sendableEndpoint(registeredURL)
	if _, err := sendVia(context.Background(), rt, staged(), listening(t, got), advancingClock(signedAt)); err != nil {
		t.Fatalf("sending: %v", err)
	}
	stamp, _, _ := sentSignature(t, got)
	var doc departure
	if err := json.Unmarshal(got.body, &doc); err != nil {
		t.Fatalf("decoding what was posted: %v", err)
	}
	if !doc.OccurredAt.Equal(time.Unix(stamp, 0)) {
		t.Fatalf("the document says it occurred at %s but the signature was stamped from %s",
			doc.OccurredAt, time.Unix(stamp, 0))
	}
}

// sentSignature reads the three signed headers off what was posted.
func sentSignature(t *testing.T, got *receiver) (stamp int64, nonce, signature string) {
	t.Helper()
	raw := got.got.Header.Get(extension.InboundHeaderTimestamp)
	stamp, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("the timestamp header is %q: %v", raw, err)
	}
	return stamp, got.got.Header.Get(extension.InboundHeaderNonce), got.got.Header.Get(extension.InboundHeaderSignature)
}

// The nonce is the receiver's REPLAY key, and the delivery id is what makes a
// re-posted attempt recognisable as the same message rather than a second one.
func TestTheSignatureNonceIsTheDeliveryTheProductStaged(t *testing.T) {
	t.Parallel()
	got := &receiver{}
	rt := sendableEndpoint(registeredURL)
	msg := staged()
	if _, err := sendVia(context.Background(), rt, msg, listening(t, got), fixedClock()); err != nil {
		t.Fatalf("sending: %v", err)
	}
	if nonce := got.got.Header.Get(extension.InboundHeaderNonce); nonce != msg.IdempotencyKey {
		t.Fatalf("it signed nonce %q; a value that changes per attempt reaches a receiver as a second message", nonce)
	}
}

// A POST whose answer never came back is the ONE refusal that must not be
// reported as a failure: the core retries every refusal it is not told is
// unanswerable, and this one would deliver the rep's message twice with nothing
// in the system able to detect it.
func TestAPostWithNoAnswerIsUnknownRatherThanFailed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// The connection dies with the request in flight and no status ever
		// written, which is exactly the shape a timeout and a reset share.
		panic(http.ErrAbortHandler)
	}))
	// The abort above is what this test is FOR, so the server's own complaint
	// about it is noise in front of whatever really failed.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(srv.Close)
	rt := sendableEndpoint(registeredURL)
	dial := func(string) (*sender, error) { return &sender{url: srv.URL, http: srv.Client()}, nil }
	_, err := sendVia(context.Background(), rt, staged(), dial, fixedClock())
	if !errors.Is(err, extension.ErrSendOutcomeUnknown) {
		t.Fatalf("an unanswered post was reported as %v, which the core reads as proof nothing was transmitted", err)
	}
	_, args := rt.tx.statementMentioning(t, "ON CONFLICT (endpoint_id, delivery_key, attempt)")
	if args[5] != outcomeUnknown {
		t.Fatalf("the attempt was recorded as %v", args[5])
	}
}

// A receiver that ANSWERED and declined is a definite answer, so nothing was
// transmitted and the product's ladder may try again. Reporting it as unknown
// would stop a delivery that is merely being refused.
func TestAReceiverThatDeclinesIsADefiniteAnswer(t *testing.T) {
	t.Parallel()
	got := &receiver{status: http.StatusInternalServerError}
	rt := sendableEndpoint(registeredURL)
	_, err := sendVia(context.Background(), rt, staged(), listening(t, got), fixedClock())
	if err == nil {
		t.Fatal("a refused message was reported as sent")
	}
	if errors.Is(err, extension.ErrSendOutcomeUnknown) {
		t.Fatalf("a refusal the receiver actually sent was reported as unanswerable: %v", err)
	}
	if !errors.Is(err, errRefused) {
		t.Fatalf("it was reported as %v", err)
	}
}

// The attempt row is written BEFORE the request leaves. A row written afterwards
// does not exist for exactly the case it is most needed in — a worker killed
// mid-flight — and the member would see a message that never left rather than one
// whose outcome nobody knows.
func TestTheAttemptIsRecordedBeforeAnythingLeaves(t *testing.T) {
	t.Parallel()
	got := &receiver{}
	rt := sendableEndpoint(registeredURL)
	if _, err := sendVia(context.Background(), rt, staged(), listening(t, got), fixedClock()); err != nil {
		t.Fatalf("sending: %v", err)
	}
	var outcomes []any
	for at, sql := range rt.tx.statements {
		if len(rt.tx.args[at]) == 7 && strings.HasPrefix(sql, "INSERT") {
			outcomes = append(outcomes, rt.tx.args[at][5])
		}
	}
	if len(outcomes) != 2 {
		t.Fatalf("the send recorded %d attempt row(s); one in flight and one completed is the shape", len(outcomes))
	}
	if outcomes[0] != outcomeUnknown || outcomes[1] != outcomeSent {
		t.Fatalf("the attempt was recorded as %v then %v", outcomes[0], outcomes[1])
	}
}

// The counters move only for a message a receiver ACCEPTED, and in the same
// transaction as the row that justifies them: a counter that included refusals
// would tell a member their connector is busier than it is.
func TestTheOutboundCounterMovesOnlyForAnAcceptedMessage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		want   int
	}{
		{"accepted", http.StatusAccepted, 1},
		{"declined", http.StatusForbidden, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := sendableEndpoint(registeredURL)
			//nolint:errcheck // the outcome is the subject of the sibling tests; this one is about the counter
			_, _ = sendVia(context.Background(), rt, staged(), listening(t, &receiver{status: tc.status}), fixedClock())
			bumped := 0
			for _, sql := range rt.tx.statements {
				if strings.Contains(sql, "outbound_sent = outbound_sent + 1") {
					bumped++
				}
			}
			if bumped != tc.want {
				t.Fatalf("the counter moved %d time(s) for a %d, want %d", bumped, tc.status, tc.want)
			}
		})
	}
}

// The receipt carries NO provider message id, and the surface sanctions the empty
// one: a unit inventing an id would mint a thread key that matches nothing on any
// later reply.
func TestTheReceiptInventsNoMessageId(t *testing.T) {
	t.Parallel()
	rt := sendableEndpoint(registeredURL)
	receipt, err := sendVia(context.Background(), rt, staged(), listening(t, &receiver{}), fixedClock())
	if err != nil {
		t.Fatalf("sending: %v", err)
	}
	if receipt.ProviderMessageID != "" {
		t.Fatalf("the receipt claims message id %q, which nothing at the far end issued", receipt.ProviderMessageID)
	}
}

// A delivery with no id has no replay key to sign, and inventing one would reach
// the receiver as a new message on every attempt. Nothing leaves.
func TestADeliveryWithNoIdIsRefusedBeforeAnythingLeaves(t *testing.T) {
	t.Parallel()
	rt := sendableEndpoint(registeredURL)
	msg := staged()
	msg.IdempotencyKey = ""
	dial := func(string) (*sender, error) {
		t.Fatal("a delivery with no id reached the transport")
		return nil, nil
	}
	if _, err := sendVia(context.Background(), rt, msg, dial, fixedClock()); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("it was answered %v", err)
	}
}

// Every way a member can have nothing to send THROUGH, each of them transmitting
// nothing. They are one answer on purpose: Live reports the same four facts one
// step earlier, and a delivery that reached Send anyway is one whose next
// pre-flight parks it where a human can see it.
func TestAnUnsendableEndpointRefusesWithoutTransmitting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(*fakeRuntime)
	}{
		{"no endpoint at all", func(rt *fakeRuntime) { rt.tx.singleRows = nil; rt.tx.noRows[1] = true }},
		{"an endpoint that is paused", func(rt *fakeRuntime) {
			rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, registeredURL, false)}
		}},
		{"no registered address", func(rt *fakeRuntime) {
			rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}
		}},
		{"no signing secret minted", func(rt *fakeRuntime) {
			delete(rt.secrets.stored, rt.secrets.userKey(ownerUserID, inboundSecretKey))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := sendableEndpoint(registeredURL)
			tc.setup(rt)
			dial := func(string) (*sender, error) {
				t.Fatal("an unsendable endpoint reached the transport")
				return nil, nil
			}
			if _, err := sendVia(context.Background(), rt, staged(), dial, fixedClock()); !errors.Is(err, extension.ErrNotFound) {
				t.Fatalf("it was answered %v", err)
			}
			// The same state, asked one step earlier, is a CONFIRMED no rather
			// than a fault: the core parks on the first and retries on the
			// second, so collapsing them would strand a deliverable message or
			// re-send a refused one.
			live, err := live(context.Background(), rt, ownerUserID)
			if err != nil || live {
				t.Fatalf("liveness answered %t, %v", live, err)
			}
		})
	}
}

// Live answers from stored state ALONE — it takes no transport at all, which is
// the assertion its signature makes and this test's fixture relies on: nothing
// here starts a listener, and a Live that dialled would have nothing to reach.
func TestLiveAnswersWithoutSpendingTheCredential(t *testing.T) {
	t.Parallel()
	rt := sendableEndpoint(registeredURL)
	answered, err := live(context.Background(), rt, ownerUserID)
	if err != nil {
		t.Fatalf("asking whether this member may send: %v", err)
	}
	if !answered {
		t.Fatal("a member with an open, enabled, addressed and minted endpoint was told they may not send")
	}
}

// A failure to FIND OUT is an error rather than a verdict. Asserting a capability
// nobody could read is how a rep learns at transmission what they should have
// been told at the composer.
func TestLiveReportsAFaultRatherThanAVerdict(t *testing.T) {
	t.Parallel()
	rt := sendableEndpoint(registeredURL)
	rt.txErr = errors.New("the pool is gone")
	answered, err := live(context.Background(), rt, ownerUserID)
	if err == nil {
		t.Fatal("a database fault was reported as 'this member may not send', which parks a delivery nothing is wrong with")
	}
	if answered {
		t.Fatal("liveness answered true on a fault")
	}
}
