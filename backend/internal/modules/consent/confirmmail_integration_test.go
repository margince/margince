// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The send path for the confirm-details link: it mints, it mails to the
// SUBJECT's own address rather than one a caller named, it never hands the
// token back, and an installation that cannot deliver still records that the
// link exists.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// recordingMailer keeps what it was asked to send, so a test can assert the
// address and the body rather than only that no error came back.
type recordingMailer struct {
	to      string
	subject string
	body    string
	fail    error
	calls   int
}

func (m *recordingMailer) Send(_ context.Context, to, subject, textBody string) error {
	m.calls++
	if m.fail != nil {
		return m.fail
	}
	m.to, m.subject, m.body = to, subject, textBody
	return nil
}

func confirmRequest(t *testing.T, e *channelConsentEnv, h Handlers) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/people/x/consent/confirm-request", nil).
		WithContext(e.ctx)
	h.RequestDetailsConfirmation(rec, req, crmcontracts.Id(e.person.UUID))
	return rec
}

func TestAConfirmRequestMailsTheSubjectTheirOwnLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	relay := &recordingMailer{}
	h := Handlers{store: e.store}.
		WithConfirmMailer(relay).
		WithConfirmLinkBase("https://crm.example.test/")

	rec := confirmRequest(t, e, h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	var got struct {
		DeliveredTo string `json:"delivered_to"`
		Delivered   bool   `json:"delivered"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the issue response: %v", err)
	}

	want := "subject-" + e.person.String() + "@example.test"
	if got.DeliveredTo != want {
		t.Fatalf("delivered_to = %q, want the subject's own address %q", got.DeliveredTo, want)
	}
	if !got.Delivered {
		t.Fatal("delivered = false with a working relay wired")
	}
	// The token is the capability the whole mailbox-as-evidence claim rests on.
	// A caller who could read it here could open the subject's record without
	// ever holding their mailbox.
	if got.Token != "" {
		t.Fatal("the response carried the plaintext token, which only the mailbox may hold")
	}

	if relay.to != want {
		t.Fatalf("mailed to %q, want the subject's own address %q", relay.to, want)
	}
	// The link has to be built on the CANONICAL origin, not on a request Host:
	// it opens one person's record to whoever holds it.
	if !strings.Contains(relay.body, "https://crm.example.test/#/confirm/") {
		t.Fatalf("the mail body carries no confirm link on the canonical origin:\n%s", relay.body)
	}
	// The trailing slash on the configured base must not survive into the link.
	if strings.Contains(relay.body, "test//#/") {
		t.Fatalf("the link doubled the base's trailing slash:\n%s", relay.body)
	}
	if relay.subject == "" {
		t.Fatal("the mail carried no subject line")
	}
}

func TestAContactWithNoAddressIsRefusedRatherThanSilentlyNotMailed(t *testing.T) {
	e := setupChannelConsent(t)
	relay := &recordingMailer{}
	h := Handlers{store: e.store}.
		WithConfirmMailer(relay).
		WithConfirmLinkBase("https://crm.example.test")

	// No seedSubjectAddress: this person has no live mailbox.
	rec := confirmRequest(t, e, h)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
	}
	if relay.calls != 0 {
		t.Fatal("a mail was attempted for a contact with no address")
	}
}

func TestAnInstallationThatCannotDeliverStillMintsTheLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	// No mailer and no base: the ordinary state of an installation that has
	// configured no outbound relay.
	h := Handlers{store: e.store}

	rec := confirmRequest(t, e, h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Delivered bool `json:"delivered"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the issue response: %v", err)
	}
	// The write happened. Reporting it as a failure would invite a second
	// request that mints another token and supersedes the first.
	if got.Delivered {
		t.Fatal("delivered = true with no relay configured")
	}
}

func TestARelayOutageDoesNotFailTheRequestOrHideItself(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	relay := &recordingMailer{fail: errors.New("the relay refused the message")}
	h := Handlers{store: e.store}.
		WithConfirmMailer(relay).
		WithConfirmLinkBase("https://crm.example.test")

	rec := confirmRequest(t, e, h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Delivered bool `json:"delivered"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the issue response: %v", err)
	}
	// The distinction that matters: the token exists, and nobody was sent it.
	// Collapsing this into a 201 that claims delivery leaves a rep believing a
	// contact was asked when they never were.
	if got.Delivered {
		t.Fatal("delivered = true after the relay refused the message")
	}
	if relay.calls != 1 {
		t.Fatalf("the relay was called %d times, want exactly one attempt", relay.calls)
	}
}

func TestAMintedLinkOpensTheSubjectsOwnRecord(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	relay := &recordingMailer{}
	h := Handlers{store: e.store}.
		WithConfirmMailer(relay).
		WithConfirmLinkBase("https://crm.example.test")

	if rec := confirmRequest(t, e, h); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	// The token in the mail must be the one the public page resolves. A link
	// that mails one token and stores another is a link nobody can use, and
	// nothing upstream of the recipient would notice.
	_, token, found := strings.Cut(relay.body, "#/confirm/")
	if !found {
		t.Fatalf("no confirm link in the mail body:\n%s", relay.body)
	}
	token = strings.Fields(token)[0]

	ref, err := e.store.ResolveConfirmToken(context.Background(), token)
	if err != nil {
		t.Fatalf("resolve the token that was actually mailed: %v", err)
	}
	if ref.PersonID != e.person {
		t.Fatalf("the mailed link opens person %s, want the subject %s", ref.PersonID, e.person)
	}
}
