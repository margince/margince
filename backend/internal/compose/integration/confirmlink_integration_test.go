// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Granting a double-opt-in purpose over the composed server, the only way the
// product still allows it.
//
// The operator-held double-opt-in token is gone (#3807): it was minted for the
// authenticated operator, who could paste it straight back into recordConsent,
// so the round trip whose whole evidentiary value is that the SUBJECT completed
// it from their own mailbox could be closed without the subject taking part.
// What remains is the confirm-details link — mailed to the person's own live
// primary address, single-use, and the thing that earns the mailbox proof
// recordConsent now requires.
//
// So a suite that needs a granted marketing consent has to reach for the
// wiring: an operator relay, and the link the product actually mailed. One
// helper rather than one per suite, because two spellings of "grant marketing"
// would be two readings of what the product accepts, and only one of them would
// be corrected the next time that path moves.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// confirmRelay is the operator relay as a test holds it: what the product
// handed over, kept so the test can follow the link a subject would.
//
// Locked because the relay is called on the server's own goroutine while the
// test reads from its own, which the race detector sees even when the two
// happen to be ordered by the request/response round trip.
type confirmRelay struct {
	mu      sync.Mutex
	to      string
	subject string
	body    string
}

func (m *confirmRelay) Send(_ context.Context, to, subject, textBody string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.to, m.subject, m.body = to, subject, textBody
	return nil
}

// lastBody answers what the relay was last handed.
func (m *confirmRelay) lastBody() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.body
}

// lastRecipient answers who the relay was last asked to write to.
func (m *confirmRelay) lastRecipient() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.to
}

// withConfirmRelay composes an installation that can send operator mail, and
// hands back what it sent. Both halves are needed: WithOperatorMail alone gives
// an installation a relay, and the confirm link is only built when a canonical
// public base is configured too — every caller of this already sets one.
func withConfirmRelay() (*confirmRelay, compose.Option) {
	mail := &confirmRelay{}
	return mail, compose.WithOperatorMail(mail)
}

// confirmLinkToken reads the single-use token out of the mail the product sent.
//
// The response to confirm-request deliberately does NOT carry it — that is the
// property the whole mailbox-as-evidence claim rests on — so a test that wants
// to follow the link has to read it where the subject would.
func confirmLinkToken(t *testing.T, mail *confirmRelay) string {
	t.Helper()
	const marker = "/#/confirm/"
	body := mail.lastBody()
	at := strings.Index(body, marker)
	if at < 0 {
		t.Fatalf("the confirm mail carries no link on the canonical origin:\n%s", body)
	}
	token := strings.TrimSpace(body[at+len(marker):])
	if cut := strings.IndexAny(token, " \n\r"); cut >= 0 {
		token = token[:cut]
	}
	if token == "" {
		t.Fatalf("the confirm link carries an empty token:\n%s", body)
	}
	return token
}

// answerMarketingByConfirmLink takes the subject through the round trip: the
// workspace mails them a link, and they answer from their own mailbox.
//
// `choice` is the contract's own vocabulary — "granted" or "withdrawn". The
// wording is stored verbatim as the proof of what they were shown, and the
// contract requires one with a grant.
func answerMarketingByConfirmLink(t *testing.T, e *apptest.AppEnv, mail *confirmRelay, personID, choice string) {
	t.Helper()
	requestConfirmLink(t, e, mail, personID)
	spendConfirmLink(t, e, confirmLinkToken(t, mail), choice)
}

// requestConfirmLink mints and mails one link, and proves the relay took it —
// an installation that quietly sent nothing would leave every later step
// failing for a reason that has nothing to do with what it was testing.
func requestConfirmLink(t *testing.T, e *apptest.AppEnv, mail *confirmRelay, personID string) {
	t.Helper()
	var issued struct {
		DeliveredTo      string `json:"delivered_to"`
		ProviderAccepted bool   `json:"provider_accepted"`
		Sendable         bool   `json:"sendable"`
	}
	if status := e.Call(t, "POST", "/v1/people/"+personID+"/consent/confirm-request",
		nil, nil, &issued); status != http.StatusCreated {
		t.Fatalf("request a confirm link → %d", status)
	}
	if !issued.Sendable || !issued.ProviderAccepted {
		t.Fatalf("the confirm link was not mailed (sendable=%v provider_accepted=%v)",
			issued.Sendable, issued.ProviderAccepted)
	}
	if got := mail.lastRecipient(); got != issued.DeliveredTo {
		t.Fatalf("the link was mailed to %q and the response named %q", got, issued.DeliveredTo)
	}
}

// spendConfirmLink is the subject's own half: anonymous, token-authed, and the
// only thing that earns the mailbox proof a double-opt-in grant now needs.
func spendConfirmLink(t *testing.T, e *apptest.AppEnv, token, choice string) {
	t.Helper()
	body := AnyMap{"marketing_choice": choice}
	if choice == "granted" {
		body["marketing_wording"] = "Yes, send me product news by email."
	}
	if status := publicCall(t, e, "POST", "/v1/public/confirm/"+token, body, nil, nil); status != http.StatusNoContent {
		t.Fatalf("spend the confirm link with marketing_choice=%q → %d", choice, status)
	}
}
