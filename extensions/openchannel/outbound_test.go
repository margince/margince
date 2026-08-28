// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// attemptRow scripts one row of outboundColumns, in that order.
func attemptRow(id, key string, attempt int, outcome, class string) []any {
	return []any{id, key, attempt, "acct-77", outcome, class, signedAt}
}

// The listing reports the CALLER's own endpoint. The entries say who this member
// has been messaging and when, which is not a fact this unit hands to a colleague
// because they hold the same RBAC object.
func TestTheOutboundListingReadsOnlyTheCallersOwnEndpoint(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, registeredURL, true)}
	rt.tx.queryRows = [][]any{attemptRow(firstRequestID, "d-1", 1, outcomeSent, "")}
	if _, err := listOutbound(context.Background(), rt, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if _, args := rt.tx.statementMentioning(t, "endpoint_id = $1::uuid ORDER BY created_at DESC"); args[0] != endpointID {
		t.Fatalf("the listing read endpoint %v; the caller's own is %v", args[0], endpointID)
	}
	// The endpoint is resolved from the INVOCATION, never from the arguments:
	// this operation takes none at all.
	if _, args := rt.tx.statementMentioning(t, "user_id = $1::uuid AND slug = $2"); args[0] != ownerUserID {
		t.Fatalf("the caller's endpoint was resolved for %v", args[0])
	}
}

// The listing does not repeat the message body. The member wrote it and the CRM
// keeps it on the record it was sent from; a second copy here would be a second
// place to erase it from.
func TestTheOutboundListingNeverReturnsTheMessage(t *testing.T) {
	t.Parallel()
	if strings.Contains(outboundColumns, "body") {
		t.Fatalf("the listing projection selects a body:\n%s", outboundColumns)
	}
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, registeredURL, true)}
	rt.tx.queryRows = [][]any{attemptRow(firstRequestID, "d-1", 2, outcomeUnknown, classDeliveryUnanswered.Class)}
	raw, err := listOutbound(context.Background(), rt, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	answer := jsonOf[struct {
		Attempts []outboundAttempt `json:"attempts"`
	}](t, raw)
	if len(answer.Attempts) != 1 {
		t.Fatalf("the listing answered %d attempt(s)", len(answer.Attempts))
	}
	if answer.Attempts[0].Outcome != outcomeUnknown || answer.Attempts[0].Attempt != 2 {
		t.Fatalf("the attempt reads %+v", answer.Attempts[0])
	}
	if answer.Attempts[0].ErrorClass != classDeliveryUnanswered.Class {
		t.Fatalf("the attempt names class %q, which is not one this unit declared", answer.Attempts[0].ErrorClass)
	}
}

// Not having opened an endpoint is the ordinary state of this screen, not an
// error — and it must answer an empty list rather than reading somebody else's.
func TestTheOutboundListingAnswersNothingBeforeAnEndpointIsOpened(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows[1] = true
	raw, err := listOutbound(context.Background(), rt, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a member with no endpoint was answered %v", err)
	}
	if got := strings.Count(string(raw), "["); got != 1 || !strings.Contains(string(raw), "[]") {
		t.Fatalf("the answer is %s; an empty list is what a screen renders", raw)
	}
	if len(rt.tx.statements) != 1 {
		t.Fatalf("it read a second table for a member with no endpoint:\n%s", strings.Join(rt.tx.statements, "\n"))
	}
}

// An unattended invocation has nobody whose sent messages these would be. A job
// tick and a bus delivery both answer the zero Caller.
func TestTheOutboundListingRefusesAnInvocationWithNobodyBehindIt(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	if _, err := listOutbound(context.Background(), rt, json.RawMessage(`{}`)); err == nil {
		t.Fatal("an invocation with nobody behind it was answered a member's sent messages")
	}
}

// The declared vocabulary is what a screen renders and what a row records, so
// every class this unit can WRITE has to be one it declared: an undeclared class
// reaches the operator surface as the unvetted substitute.
func TestEveryClassTheOutboundPathRecordsIsDeclared(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, class := range New().FailureClasses {
		if err := class.Validate(); err != nil {
			t.Fatalf("declared class %q is not usable: %v", class.Class, err)
		}
		declared[class.Class] = true
	}
	if err := extension.ValidateFailureClasses(New().FailureClasses); err != nil {
		t.Fatalf("the declared set is not usable: %v", err)
	}
	for _, class := range []extension.FailureClass{
		attemptClass(errRefused), attemptClass(errUnanswered),
		classPayloadUnusable, classRefusedByTheCore, classMemberNotPermitted,
		classCaptureNotDeclared, classCaptureUnavailable, classEveryRequestFailed,
		classDrainFailed,
	} {
		if !declared[class.Class] {
			t.Fatalf("class %q is written by this unit and not declared by it", class.Class)
		}
	}
	// A successful attempt records NO class, which the column stores as absent
	// rather than as an empty token.
	if attemptClass(nil).Class != "" {
		t.Fatalf("a message that was accepted was given class %q", attemptClass(nil).Class)
	}
}

// The egress guard's refusal must reach the ledger as its OWN class, and it is
// exercised through the real guard rather than a stand-in error: a class
// nothing produces is a class an operator never sees, and the previous
// outbound-undeliverable class was exactly that — declared, tested for its
// declaration, and unreachable.
//
// The distinction is not cosmetic. `unanswered` means the message may be at the
// recipient, so the product's ladder STOPS rather than resend. A blocked dial
// transmitted nothing at all, so parking it would strand a delivery that
// certainly never left, under a sentence blaming a system this installation
// declined to call.
func TestTheEgressGuardsRefusalIsItsOwnClass(t *testing.T) {
	t.Parallel()
	// The real constructor, so the real Control hook is attached, and a NAME
	// rather than an address literal — the grammar refuses those earlier, and
	// this test is about the guard that runs on what the name RESOLVES to,
	// which is the only check a member cannot see coming.
	post, err := newSender("https://localhost/hook")
	if err != nil {
		t.Skipf("the address grammar refused before the dial guard could run: %v", err)
	}
	sendErr := post.post(context.Background(), []byte("secret"), "nonce", time.Now(), []byte(`{}`))
	if sendErr == nil {
		t.Fatal("a loopback destination was posted to")
	}
	if !errors.Is(sendErr, errBlocked) {
		t.Fatalf("the guard's refusal came back as %v, which is not the blocked class — "+
			"an operator reads it as the receiver failing to answer", sendErr)
	}
	if got := attemptClass(sendErr); got != classDeliveryBlocked {
		t.Fatalf("the ledger would record %q for a dial this installation refused", got.Class)
	}
	if got := outcomeOf(sendErr); got != outcomeRefused {
		t.Fatalf("a blocked dial was recorded as %q — nothing left, so the outcome is definite", got)
	}
}
