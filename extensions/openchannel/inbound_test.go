// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The anonymous edge, driven through the published handler exactly as the core
// drives it.
//
// There is no clock here and no sleep: the only time this handler reads is the
// one the SENDER signed, which arrives on the request and is checked for
// freshness by the core before the handler is reached. A test that reached for
// a real clock would be testing the core's stage, not this one.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// senderSecret is what a correctly configured sender holds.
const senderSecret = "6f1c2b8d4e7a09f35c81d6b24a0e93f7c5d81a26b409e7f3c1d5a80b62e94f7d"

// signedAt is the instant the fixtures sign at. A fixed value, because the
// signature covers it: a test that signed at time.Now would be a test whose
// input changed every run for no reason anybody could point at.
var signedAt = time.Date(2026, 5, 12, 8, 15, 0, 0, time.UTC)

// signedRequest builds one correctly signed arrival. The signature is computed
// over SignedPayload — the published method the handler also uses — because a
// fixture that re-spelled the concatenation would agree with a verifier that
// re-spelled it the same way, and neither would agree with a real sender.
func signedRequest(secret, nonce, body string) extension.InboundRequest {
	req := extension.InboundRequest{
		Slug:      inboundSlug,
		Timestamp: signedAt,
		Nonce:     nonce,
		Body:      []byte(body),
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(req.SignedPayload())
	req.Signature = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return req
}

// openEdge is the Runtime an arriving request meets when the endpoint is open,
// enabled, and its owner's secret is sealed. The core mints an unattended
// Runtime for this edge, so the fake does too.
func openEdge(pending int64) *fakeRuntime {
	rt := newRuntime().unattended()
	rt.secrets.stored[ownerUserID+"/"+inboundSecretKey] = []byte(senderSecret)
	rt.tx.singleRows = [][]any{
		{endpointID, ownerUserID, true},          // the endpoint lookup
		{pending},                                // the queue depth
		{"7d2e5c14-9a83-4b60-8f21-3c6d0e4a1b95"}, // the inserted row's id
	}
	return rt
}

func TestASignedRequestLandsOneRowStampedFromTheEndpoint(t *testing.T) {
	t.Parallel()
	rt := openEdge(0)

	outcome, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", `{"text":"hello"}`))
	if err != nil {
		t.Fatalf("a correctly signed request failed: %v", err)
	}
	if outcome != extension.InboundAccepted {
		t.Fatalf("a correctly signed request answered outcome %d, want accepted", outcome)
	}
	sql, args := rt.tx.statementMentioning(t, "ON CONFLICT (endpoint_id, nonce) DO NOTHING")
	if args[0] != endpointID {
		t.Fatalf("the row was queued against %v, not the endpoint the slug resolved to", args[0])
	}
	// THE OWNER COMES FROM THE ENDPOINT ROW. The body below names somebody
	// else, and it must make no difference at all.
	if args[1] != ownerUserID {
		t.Fatalf("the row was stamped %v; the owner comes from the endpoint, never the payload", args[1])
	}
	if args[2] != "n-1" {
		t.Fatalf("the replay key stored is %v, not the nonce the sender signed", args[2])
	}
	if !strings.Contains(sql, "sent_at") {
		t.Fatalf("the queued row does not record what the sender signed:\n%s", sql)
	}
	rt.tx.statementMentioning(t, "inbound_received = inbound_received + 1")
}

func TestTheOwnerIsNeverTakenFromThePayload(t *testing.T) {
	t.Parallel()
	rt := openEdge(0)
	body := `{"user_id":"` + colleagueUserID + `","text":"hello"}`

	if _, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", body)); err != nil {
		t.Fatalf("receiving: %v", err)
	}
	_, args := rt.tx.statementMentioning(t, "ON CONFLICT (endpoint_id, nonce) DO NOTHING")
	for _, arg := range args {
		if arg == colleagueUserID {
			t.Fatalf("a user id the anonymous sender chose reached the insert: %v", args)
		}
	}
}

// The five refusals, and the point of the table is that every row answers the
// SAME thing. A test per reason would still pass if one of them started
// answering differently; asserting them together is what fails when one does.
func TestEveryRefusalIsTheSameRefusal(t *testing.T) {
	t.Parallel()
	for name, build := range map[string]func() (*fakeRuntime, extension.InboundRequest){
		"a slug nobody opened": func() (*fakeRuntime, extension.InboundRequest) {
			rt := newRuntime().unattended()
			rt.tx.noRows = map[int]bool{1: true}
			return rt, signedRequest(senderSecret, "n-1", "{}")
		},
		"an endpoint that is paused": func() (*fakeRuntime, extension.InboundRequest) {
			rt := newRuntime().unattended()
			rt.secrets.stored[ownerUserID+"/"+inboundSecretKey] = []byte(senderSecret)
			rt.tx.singleRows = [][]any{{endpointID, ownerUserID, false}}
			return rt, signedRequest(senderSecret, "n-1", "{}")
		},
		"an endpoint whose secret was never minted": func() (*fakeRuntime, extension.InboundRequest) {
			rt := newRuntime().unattended()
			rt.tx.singleRows = [][]any{{endpointID, ownerUserID, true}}
			return rt, signedRequest(senderSecret, "n-1", "{}")
		},
		"a signature under the wrong secret": func() (*fakeRuntime, extension.InboundRequest) {
			return openEdge(0), signedRequest("0f0f0f0f", "n-1", "{}")
		},
		"a body edited after signing": func() (*fakeRuntime, extension.InboundRequest) {
			req := signedRequest(senderSecret, "n-1", "{}")
			req.Body = []byte(`{"text":"tampered"}`)
			return openEdge(0), req
		},
		"a nonce this endpoint has already taken": func() (*fakeRuntime, extension.InboundRequest) {
			rt := openEdge(0)
			// The endpoint and the depth answer; the insert matches nothing.
			rt.tx.noRows = map[int]bool{3: true}
			return rt, signedRequest(senderSecret, "n-1", "{}")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rt, req := build()
			outcome, err := receive(context.Background(), rt, req)
			if err != nil {
				t.Fatalf("a refusal must not be an error the core logs: %v", err)
			}
			if outcome != extension.InboundUnauthenticated {
				t.Fatalf("answered outcome %d; every one of these is one opaque refusal", outcome)
			}
		})
	}
}

// The constant-shape half of the refusal, and the half a reader is most likely
// to delete as pointless. Every path that has no real secret behind it still
// gets one to compare against, so a probe does not come back before a wrong
// signature does and make the installation's endpoints enumerable by clock.
func TestEveryRefusalHasAKeyToVerifyAgainst(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()

	// A slug nobody opened resolves to no endpoint at all.
	key, err := admittingSecret(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("an unresolved slug must still yield a key: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("an unresolved slug yielded an empty key, so its refusal skips the comparison every other refusal makes")
	}
	if verified(key, signedRequest(senderSecret, "n-1", "{}")) {
		t.Fatal("a request signed with a real secret verified against the stand-in")
	}

	// An endpoint that exists but was never minted for takes the same path,
	// and it takes it AFTER reading the namespace, so it is refused by the
	// comparison rather than by returning early.
	unminted, err := admittingSecret(context.Background(), rt,
		&inboundTarget{id: endpointID, owner: ownerUserID})
	if err != nil {
		t.Fatalf("an unminted endpoint must still yield a key: %v", err)
	}
	if rt.secrets.gets != 1 {
		t.Fatalf("the namespace was read %d times, want once", rt.secrets.gets)
	}
	if len(unminted) == 0 {
		t.Fatal("an unminted endpoint yielded an empty key")
	}
}

func TestAReplayLandsNothing(t *testing.T) {
	t.Parallel()
	rt := openEdge(0)
	rt.tx.noRows = map[int]bool{3: true}

	if _, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", "{}")); err != nil {
		t.Fatalf("receiving: %v", err)
	}
	for _, sql := range rt.tx.statements {
		if strings.Contains(sql, "inbound_received = inbound_received + 1") {
			t.Fatal("a replay moved the traffic counters, so the screen counts an arrival the queue does not hold")
		}
	}
	if len(rt.tx.audited) != 0 {
		t.Fatalf("an anonymous arrival wrote %d ledger rows; the trail holds decisions, not traffic", len(rt.tx.audited))
	}
}

func TestAFullQueueRefusesWithoutHidingTheEndpoint(t *testing.T) {
	t.Parallel()
	rt := openEdge(maxPendingInbound)

	outcome, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", "{}"))
	if err != nil {
		t.Fatalf("a full queue is not a fault: %v", err)
	}
	// Over capacity and NOT the opaque refusal: the sender's credentials were
	// good, and telling it so is what lets it back off rather than re-issue.
	if outcome != extension.InboundOverCapacity {
		t.Fatalf("a full queue answered outcome %d, want over-capacity", outcome)
	}
	for _, sql := range rt.tx.statements {
		if strings.Contains(sql, "ON CONFLICT (endpoint_id, nonce) DO NOTHING") {
			t.Fatalf("the request was queued past the cap:\n%s", sql)
		}
	}
}

func TestOneRequestBelowTheCapStillLands(t *testing.T) {
	t.Parallel()
	rt := openEdge(maxPendingInbound - 1)

	outcome, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", "{}"))
	if err != nil {
		t.Fatalf("receiving: %v", err)
	}
	if outcome != extension.InboundAccepted {
		t.Fatalf("the last slot in the queue answered outcome %d, want accepted", outcome)
	}
}

// A database that will not answer is a fault the sender should retry, not a
// refusal it should treat as bad credentials.
func TestADatabaseFaultIsTransientRatherThanARefusal(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	rt.txErr = extension.ErrRuntimeExpired

	outcome, err := receive(context.Background(), rt, signedRequest(senderSecret, "n-1", "{}"))
	if outcome != extension.InboundTransient {
		t.Fatalf("a fault answered outcome %d, want transient", outcome)
	}
	if err == nil {
		t.Fatal("a transient outcome carries the cause, or the core logs nothing to debug from")
	}
}

// The material verified is the published SignedPayload and nothing else. This
// is the one property a wrong implementation would still pass its own tests on,
// because a verifier and a fixture that re-spell the concatenation the same way
// agree with each other and with no real sender.
func TestTheVerifiedMaterialIsThePublishedSignedPayload(t *testing.T) {
	t.Parallel()
	req := extension.InboundRequest{
		Slug: inboundSlug, Timestamp: signedAt, Nonce: "n-1", Body: []byte(`{"text":"hi"}`),
	}
	mac := hmac.New(sha256.New, []byte(senderSecret))
	mac.Write(req.SignedPayload())
	req.Signature = "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verified([]byte(senderSecret), req) {
		t.Fatal("a signature over SignedPayload did not verify")
	}
	// Each of the three parts is covered: change one and the signature is no
	// longer the signature.
	for name, edit := range map[string]func(extension.InboundRequest) extension.InboundRequest{
		"the timestamp": func(r extension.InboundRequest) extension.InboundRequest {
			r.Timestamp = signedAt.Add(time.Second)
			return r
		},
		"the nonce": func(r extension.InboundRequest) extension.InboundRequest {
			r.Nonce = "n-2"
			return r
		},
		"the body": func(r extension.InboundRequest) extension.InboundRequest {
			r.Body = []byte(`{"text":"ho"}`)
			return r
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if verified([]byte(senderSecret), edit(req)) {
				t.Fatalf("%s is not covered by the signature", name)
			}
		})
	}
}

// A bare hex digest with no `sha256=` in front of it is a different string, and
// the comparison is what refuses it — not a parse this code would otherwise
// have to write and get right.
func TestASignatureWithoutItsAlgorithmPrefixDoesNotVerify(t *testing.T) {
	t.Parallel()
	req := signedRequest(senderSecret, "n-1", "{}")
	req.Signature = strings.TrimPrefix(req.Signature, signaturePrefix)
	if verified([]byte(senderSecret), req) {
		t.Fatal("a bare digest verified, so the header's declared shape is not what is checked")
	}
}
