// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The send path for the confirm-details link: it mints, it queues to the
// SUBJECT's own address rather than one a caller named, it never hands the
// token back, and an installation that cannot deliver still records that the
// link exists.
//
// These cases used to assert against a mailer this module called directly. The
// obligations did not change when the lane did — the same address, the same
// canonical origin, the same withheld token — so they are asserted here against
// the stager the message is now handed to. What IS new is where the plaintext
// goes: the staged message carries a placeholder, and the link itself goes to
// the vault, so a test that found the link on the message would be finding a
// defect.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingStager keeps what it was asked to stage, so a test can assert the
// address, the wording and the material rather than only that no error came
// back. It stands where compose's controllerMailQueue stands in production.
type recordingStager struct {
	calls int
	seen  ConfirmationSend
	fail  error
}

func (r *recordingStager) QueueConfirmationTx(_ context.Context, _ pgx.Tx, in ConfirmationSend) (ids.UUID, error) {
	r.calls++
	if r.fail != nil {
		return ids.UUID{}, r.fail
	}
	r.seen = in
	return ids.NewV7(), nil
}

// recordingVault keeps the sealed link, which is the only place the plaintext
// legitimately exists between minting and dispatch.
type recordingVault struct {
	sealed string
	calls  int
	fail   error
}

func (v *recordingVault) Put(_ context.Context, secret string) (string, error) {
	v.calls++
	if v.fail != nil {
		return "", v.fail
	}
	v.sealed = secret
	return "vault-ref-" + secret[len(secret)-6:], nil
}

func confirmRequest(t *testing.T, e *channelConsentEnv, h Handlers) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/people/x/consent/confirm-request", nil).
		WithContext(e.ctx)
	h.RequestDetailsConfirmation(rec, req, crmcontracts.Id(e.person.UUID))
	return rec
}

// withLane is the ordinary wiring: a stager, a vault and a canonical origin.
func withLane(e *channelConsentEnv, stager ConfirmationSender, vault ConfirmLinkVault) Handlers {
	return Handlers{store: e.store}.
		WithConfirmationLane(stager, vault, "https://crm.example.test/")
}

func TestAConfirmRequestQueuesTheSubjectTheirOwnLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager, vault := &recordingStager{}, &recordingVault{}

	rec := confirmRequest(t, e, withLane(e, stager, vault))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	var got struct {
		DeliveredTo string `json:"delivered_to"`
		Queued      bool   `json:"queued"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the issue response: %v", err)
	}

	want := "subject-" + e.person.String() + "@example.test"
	if got.DeliveredTo != want {
		t.Fatalf("delivered_to = %q, want the subject's own address %q", got.DeliveredTo, want)
	}
	if !got.Queued {
		t.Fatal("queued = false with a working lane wired")
	}
	// The token is the capability the whole mailbox-as-evidence claim rests on.
	// A caller who could read it here could open the subject's record without
	// ever holding their mailbox.
	if got.Token != "" {
		t.Fatal("the response carried the plaintext token, which only the mailbox may hold")
	}

	if stager.seen.Recipient != want {
		t.Fatalf("queued to %q, want the subject's own address %q", stager.seen.Recipient, want)
	}
	// The link has to be built on the CANONICAL origin, not on a request Host:
	// it opens one person's record to whoever holds it.
	if !strings.Contains(vault.sealed, "https://crm.example.test/#/confirm/") {
		t.Fatalf("the sealed link is not on the canonical origin: %q", vault.sealed)
	}
	// The trailing slash on the configured base must not survive into the link.
	if strings.Contains(vault.sealed, "test//#/") {
		t.Fatalf("the link doubled the base's trailing slash: %q", vault.sealed)
	}
	if stager.seen.Rendered.Subject == "" {
		t.Fatal("the message carried no subject line")
	}
}

// TestTheStagedMessageCarriesAPlaceholderAndNotTheLink is the property the
// vault exists for, and it is new: the message body is copied onto the delivery
// row, the timeline, the audit entry and the outbox event, so a link on it would
// be a live credential in all four.
func TestTheStagedMessageCarriesAPlaceholderAndNotTheLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager, vault := &recordingStager{}, &recordingVault{}

	if rec := confirmRequest(t, e, withLane(e, stager, vault)); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	if strings.Contains(stager.seen.Rendered.Body, "#/confirm/") {
		t.Fatalf("the staged body carries the plaintext link, which then lands on the delivery "+
			"row, the timeline and the audit entry:\n%s", stager.seen.Rendered.Body)
	}
	if !strings.Contains(stager.seen.Rendered.Body, linkPlaceholder) {
		t.Fatalf("the staged body carries no placeholder, so dispatch has nowhere to put the "+
			"link:\n%s", stager.seen.Rendered.Body)
	}
	if stager.seen.LinkRef == "" {
		t.Fatal("the staged message names no vault reference, so the link can never be recovered")
	}
}

func TestAContactWithNoAddressIsRefusedRatherThanSilentlyNotMailed(t *testing.T) {
	e := setupChannelConsent(t)
	stager := &recordingStager{}

	// No seedSubjectAddress: this person has no live mailbox.
	rec := confirmRequest(t, e, withLane(e, stager, &recordingVault{}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
	}
	if stager.calls != 0 {
		t.Fatal("a message was staged for a contact with no address")
	}
}

func TestAnInstallationThatCannotDeliverStillMintsTheLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	// No lane at all: the ordinary state of an installation that has configured
	// no outbound relay.
	h := Handlers{store: e.store}

	rec := confirmRequest(t, e, h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Queued   bool `json:"queued"`
		Sendable bool `json:"sendable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the issue response: %v", err)
	}
	// The write happened. Reporting it as a failure would invite a second
	// request that mints another token and supersedes the first.
	if got.Queued {
		t.Fatal("queued = true with no lane configured")
	}
	// And the reason is NOT a failed send. A reader told "the mail did not go
	// out" would press again forever against an installation that cannot send
	// at all.
	if got.Sendable {
		t.Fatal("sendable = true with no lane wired")
	}
}

// TestAStagingFailureFailsTheRequestRatherThanMintingASilentToken is the one
// answer that CHANGED with the lane, deliberately.
//
// The old direct call swallowed a relay error: the token was already committed
// by then, so failing would have left it minted and unusable. Staging happens
// INSIDE that transaction, so a failure rolls the token back too — and a caller
// who retries gets a fresh link rather than a second one superseding a first
// that was never sent.
func TestAStagingFailureFailsTheRequestRatherThanMintingASilentToken(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := &recordingStager{fail: errors.New("the queue refused the message")}

	rec := confirmRequest(t, e, withLane(e, stager, &recordingVault{}))
	if rec.Code == http.StatusCreated {
		t.Fatalf("status = 201 after staging failed: the token would exist with no message "+
			"carrying it (body: %s)", rec.Body.String())
	}

	// And nothing was left behind. The token and the message share a
	// transaction precisely so this cannot half-happen.
	var tokens int
	if err := e.store.db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM confirm_token WHERE person_id = $1`, e.person).Scan(&tokens)
	}); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Errorf("%d confirm token(s) survived a failed staging; the link and the mail that "+
			"carries it must commit together or not at all", tokens)
	}
}

func TestAMintedLinkOpensTheSubjectsOwnRecord(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager, vault := &recordingStager{}, &recordingVault{}

	if rec := confirmRequest(t, e, withLane(e, stager, vault)); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	// The token that was SEALED must be the one the public page resolves. A
	// lane that seals one token and stores another is a link nobody can use,
	// and nothing upstream of the recipient would notice.
	_, token, found := strings.Cut(vault.sealed, "#/confirm/")
	if !found {
		t.Fatalf("no confirm link was sealed: %q", vault.sealed)
	}
	token = strings.Fields(token)[0]

	ref, err := e.store.ResolveConfirmToken(context.Background(), token)
	if err != nil {
		t.Fatalf("resolve the token that was actually sealed: %v", err)
	}
	if ref.PersonID != e.person {
		t.Fatalf("the sealed link opens person %s, want the subject %s", ref.PersonID, e.person)
	}
}
