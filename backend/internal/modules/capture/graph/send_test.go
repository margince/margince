// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sendableAuth is a connected mailbox whose grant carries the send permission.
func sendableAuth(t *testing.T, granted ...string) connector.Auth {
	t.Helper()
	b, err := json.Marshal(graphconn.AuthState{RefreshToken: "r", Owner: owner, Granted: granted})
	if err != nil {
		t.Fatalf("marshal auth: %v", err)
	}
	return b
}

func sendableMessage() connector.EmailMessage {
	return connector.EmailMessage{
		To:        []string{"client@acme.test"},
		Subject:   "Following up",
		Body:      "As discussed.",
		MessageID: "outbound-1@margince.test",
	}
}

func TestSendTransmitsTheRenderedMessage(t *testing.T) {
	api := &fakeAPI{email: owner}
	c := New(&fakeOAuth{access: "a"}, api)

	receipt, err := c.SendEmail(context.Background(), sendableAuth(t, SendScope), sendableMessage())
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if api.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want one transmission", api.sendCalls)
	}
	wire := string(api.sentMIME[0])
	for _, want := range []string{
		"From: " + owner,
		"To: client@acme.test",
		"Subject: Following up",
		"Message-ID: <outbound-1@margince.test>",
		"As discussed.",
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("the transmitted message is missing %q:\n%s", want, wire)
		}
	}
	// An EMPTY receipt is the honest one. sendMail is asynchronous — Graph
	// returns 202 with no id and Exchange files the sent copy afterwards — so
	// there is nothing to name yet, and connector.SendReceipt reads an empty
	// RFC822MessageID as "no re-key owed", which for a MIME submit is the
	// truth: the identity in the receipt IS the one in the message body.
	if receipt.ProviderMessageID != "" || receipt.RFC822MessageID != "" {
		t.Errorf("receipt = %+v, want empty: Microsoft names nothing at submission", receipt)
	}
	// And nothing was looked up: a lookup here would miss almost every time.
	if api.findCalls != 0 {
		t.Errorf("findCalls = %d — a round trip per send that Graph cannot yet answer", api.findCalls)
	}
}

// The at-least-once guard, and the reason this connector has one at all: a
// retry that finds the message already in Sent Items must transmit NOTHING.
func TestARetryThatFindsAPriorSendDoesNotMailAgain(t *testing.T) {
	api := &fakeAPI{
		email:    owner,
		sentByID: map[string]string{"outbound-1@margince.test": "AAMk-sent-1"},
	}
	msg := sendableMessage()
	msg.Attempt = 1

	receipt, err := New(&fakeOAuth{access: "a"}, api).
		SendEmail(context.Background(), sendableAuth(t, SendScope), msg)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if api.sendCalls != 0 {
		t.Fatalf("sendCalls = %d — the recipient was mailed a second time", api.sendCalls)
	}
	if receipt.ProviderMessageID != "AAMk-sent-1" {
		t.Errorf("ProviderMessageID = %q, want the prior send's id", receipt.ProviderMessageID)
	}
	// The identity is asked about UNBRACKETED here and bracketed on the wire by
	// the client; this seam passes the stored form through unchanged.
	if api.seenFindID != "outbound-1@margince.test" {
		t.Errorf("looked up %q, want the unbracketed stored identity", api.seenFindID)
	}
}

// A FIRST attempt must not pay for the lookup at all: it cannot have been sent
// yet, and the call would be a round trip per message for nothing.
func TestAFirstAttemptNeverLooksUpAPriorSend(t *testing.T) {
	api := &fakeAPI{email: owner, sentByID: map[string]string{}}
	if _, err := New(&fakeOAuth{access: "a"}, api).
		SendEmail(context.Background(), sendableAuth(t, SendScope), sendableMessage()); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if api.findCalls != 0 || api.sendCalls != 1 {
		t.Errorf("findCalls=%d sendCalls=%d, want one send and no lookup", api.findCalls, api.sendCalls)
	}
}

// A lookup that FAILS on a retry must stop the delivery rather than fall
// through to a send: "I could not find out" is not "it was never sent", and
// treating it as one mails the recipient twice.
func TestALookupFailureOnARetryStopsRatherThanSending(t *testing.T) {
	api := &fakeAPI{email: owner, findErr: ErrUnreachable}
	msg := sendableMessage()
	msg.Attempt = 2

	_, err := New(&fakeOAuth{access: "a"}, api).SendEmail(context.Background(), sendableAuth(t, SendScope), msg)
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("SendEmail = %v, want the lookup failure surfaced", err)
	}
	if api.sendCalls != 0 {
		t.Fatal("the message was transmitted after a lookup that could not answer whether it already had been")
	}
}

func TestSendRefusesAMailboxWhoseGrantCarriesNoSendPermission(t *testing.T) {
	api := &fakeAPI{email: owner}
	// Connected for capture only — the shape every mailbox connected before the
	// send permission landed has.
	auth := sendableAuth(t, "offline_access", "User.Read", "Mail.Read")

	_, err := New(&fakeOAuth{access: "a"}, api).SendEmail(context.Background(), auth, sendableMessage())
	if !errors.Is(err, ErrSendScopeMissing) {
		t.Fatalf("SendEmail = %v, want the ungranted-permission refusal", err)
	}
	if !errors.Is(err, connector.ErrAuthRejected) {
		t.Error("the refusal must classify as an auth rejection so the delivery parks rather than retrying")
	}
	if api.sendCalls != 0 {
		t.Fatal("a message went out on a grant that does not permit sending")
	}
}

// The precondition the seam's contract states: an identity the prior-send
// lookup cannot search for makes idempotency unkeepable, so the message is
// refused BEFORE any provider I/O rather than sent once per retry.
func TestSendRefusesAMessageWithNoUsableIdentityBeforeAnyProviderCall(t *testing.T) {
	api := &fakeAPI{email: owner}
	for name, id := range map[string]string{
		"empty":          "",
		"no domain":      "outbound-1@",
		"bracketed":      "<outbound-1@margince.test>",
		"with a newline": "outbound\n1@margince.test",
	} {
		t.Run(name, func(t *testing.T) {
			msg := sendableMessage()
			msg.MessageID = id
			_, err := New(&fakeOAuth{access: "a"}, api).
				SendEmail(context.Background(), sendableAuth(t, SendScope), msg)
			if !errors.Is(err, connector.ErrInvalidMessageID) {
				t.Fatalf("SendEmail(%q) = %v, want the identity refusal", id, err)
			}
			if api.sendCalls != 0 || api.findCalls != 0 {
				t.Fatal("the provider was called for a message that could never be retried safely")
			}
		})
	}
}

// A submission the provider refuses is an error, not a silent success: the
// delivery must not be recorded as sent.
func TestASubmissionMicrosoftRefusesIsReportedAsAFailure(t *testing.T) {
	api := &fakeAPI{email: owner, sendErr: ErrUnreachable}
	_, err := New(&fakeOAuth{access: "a"}, api).
		SendEmail(context.Background(), sendableAuth(t, SendScope), sendableMessage())
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("SendEmail = %v, want the submission failure surfaced", err)
	}
}

// The carriage declaration is what the dispatcher parks an over-large message
// against, so its numbers have to be Microsoft's rather than Gmail's.
func TestCarriageDeclaresMicrosoftsOwnCeiling(t *testing.T) {
	got := New(&fakeOAuth{}, &fakeAPI{}).Carriage()
	if !got.Carries {
		t.Fatal("Carries = false; a message with files would park against a connector that can carry them")
	}
	// LITERALS, not the constants Carriage returns. Comparing a value against
	// the constant that produced it catches a copy-paste slip and nothing else
	// — including the one thing this test exists for, which is the numbers
	// silently becoming Gmail's.
	//
	// 10 files is the contract's attachment_ids cap. 2 MiB is what survives
	// Microsoft's 4 MB sendMail ceiling once the payload has been base64'd
	// twice (once per attachment inside the MIME, once for the whole message
	// on the wire) — Gmail's figure is 25 MiB.
	if got.MaxFiles != 10 {
		t.Errorf("MaxFiles = %d, want 10 (the contract's attachment_ids cap)", got.MaxFiles)
	}
	if got.MaxBytesPerFile != 2<<20 {
		t.Errorf("MaxBytesPerFile = %d, want 2 MiB — Gmail's 25 MiB would leave over-large mail to an opaque 413", got.MaxBytesPerFile)
	}
}

// THE REGRESSION THIS SEAM EXISTS FOR.
//
// A mailbox connected before Mail.Send shipped holds a read-only grant.
// Refreshing it against the DEPLOYMENT's configured scope list asks Microsoft
// for a permission that grant never carried, and Microsoft answers by refusing
// the REFRESH — so the mailbox stops CAPTURING rather than merely declining to
// send. Every standing path must refresh for what the grant holds.
func TestAnOlderGrantRefreshesForItsOwnScopesNotTheDeploymentsList(t *testing.T) {
	readOnly := []string{"offline_access", "User.Read", "Mail.Read"}
	auth := sendableAuth(t, readOnly...)

	for name, run := range map[string]func(*fakeOAuth) error{
		"sync": func(o *fakeOAuth) error {
			_, err := New(o, &fakeAPI{email: owner, initDelta: "d"}).
				Sync(context.Background(), auth, nil, &recordingSink{})
			return err
		},
		"health check": func(o *fakeOAuth) error {
			return New(o, &fakeAPI{email: owner}).HealthCheck(context.Background(), auth)
		},
	} {
		t.Run(name, func(t *testing.T) {
			oauth := &fakeOAuth{access: "a"}
			if err := run(oauth); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !slices.Equal(oauth.askedFor, readOnly) {
				t.Fatalf("the refresh asked for %v, want the grant's own %v — asking for more stops this mailbox capturing",
					oauth.askedFor, readOnly)
			}
			if slices.Contains(oauth.askedFor, SendScope) {
				t.Error("the refresh asked an unconsented grant for the send permission")
			}
		})
	}
}
