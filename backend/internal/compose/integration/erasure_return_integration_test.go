// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An erased subject is stored again only by answering a link mailed to the
// address. The ask answers every address the same way, the link stores the
// contact, its consent and the lift together, and it works once.

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const returnRootKey = "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM="

// returnMail is one message the relay was handed.
type returnMail struct{ to, text string }

// returnMailbox is the relay boundary: it hands each message to the test.
type returnMailbox struct{ sent chan returnMail }

func (m returnMailbox) Send(_ context.Context, to, _, text string) error {
	m.sent <- returnMail{to: to, text: text}
	return nil
}

var returnToken = regexp.MustCompile(`#/book/[^?]+\?return=([A-Za-z0-9_.-]+)`)

func TestAnErasedSubjectIsStoredAgainOnlyThroughTheMailedLink(t *testing.T) {
	apptest.BindEnv(t, []string{"MARGINCE_KEYVAULT_ROOT_KEY=" + returnRootKey})
	box := returnMailbox{sent: make(chan returnMail, 8)}
	e, _ := setupBookingProvider(t, compose.WithOperatorMail(box), compose.WithPublicBaseURL("https://crm.example.test"))
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	slug := bookingSlug(t, e)
	base := "/v1/public/booking/" + slug
	transactional := seededTransactionalPurposeID(t, e)
	consent := AnyMap{
		"purpose_id": transactional, "policy_version": "pp-2026-01",
		"wording": "You agree we may contact you about your meetings.",
	}
	monday := nextMonday()
	if status := publicCall(t, e, "POST", base, AnyMap{
		"start": monday.Add(time.Hour), "end": monday.Add(time.Hour + 30*time.Minute),
		"booker": AnyMap{"name": "Wieder Da", "email": erasedBooker}, "consent": consent,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("the first booking → %d, want 201", status)
	}
	eraseBooker(t, e, contactIDByEmail(t, e, erasedBooker))

	ask := func(email string) int {
		return publicCall(t, e, "POST", base+"/return", AnyMap{"name": "Wieder Da", "email": email, "consent": consent}, nil, nil)
	}
	if clean, erased := ask(cleanBooker), ask(erasedBooker); clean != http.StatusAccepted || erased != http.StatusAccepted {
		t.Fatalf("the ask for a clean address → %d, for an erased one → %d; want 202 for both", clean, erased)
	}
	token := awaitReturnLink(t, box, erasedBooker)
	if liftedRows(t, e) != 0 || storedContacts(t, e, erasedBooker) != 0 {
		t.Fatal("the ask stored something before the link was answered")
	}

	confirm := func(token string) int {
		return publicCall(t, e, "POST", base+"/return/confirm", AnyMap{"token": token}, nil, nil)
	}
	if status := confirm(token[:len(token)-2] + "xx"); status != http.StatusUnprocessableEntity {
		t.Fatalf("a forged link → %d, want 422", status)
	}
	if status := confirm(token); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}
	if liftedRows(t, e) == 0 || storedContacts(t, e, erasedBooker) != 1 {
		t.Fatal("the confirmed link did not lift the erasure and store the contact")
	}
	var evidence string
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT ce.source FROM erasure_suppression s JOIN consent_event ce ON ce.id = s.lift_consent_event_id
		 WHERE s.lifted_at IS NOT NULL`).Scan(&evidence); err != nil || evidence != "erasure_return" {
		t.Fatalf("the lift names no consent from the return (source %q): %v", evidence, err)
	}
	if status := confirm(token); status != http.StatusUnprocessableEntity {
		t.Fatalf("the link answered twice → %d, want 422", status)
	}
	select {
	case mail := <-box.sent:
		t.Fatalf("a second link went to %s; only the erased address is mailed", mail.to)
	default:
	}
}

// awaitReturnLink reads the relay until the link for address arrives. No other
// address is mailed on the way: an ask for an address nobody erased sends nothing.
func awaitReturnLink(t *testing.T, box returnMailbox, address string) string {
	t.Helper()
	select {
	case mail := <-box.sent:
		if mail.to != address {
			t.Fatalf("a link went to %s, which was never erased", mail.to)
		}
		found := returnToken.FindStringSubmatch(mail.text)
		if found == nil {
			t.Fatalf("the mail carries no return link: %q", mail.text)
		}
		return found[1]
	case <-time.After(30 * time.Second):
		t.Fatal("no return link was mailed to the erased address")
	}
	return ""
}

func liftedRows(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM erasure_suppression WHERE lifted_at IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func storedContacts(t *testing.T, e *apptest.AppEnv, email string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM contact_email ce JOIN contact c ON c.id = ce.contact_id
		 WHERE ce.email = $1 AND c.archived_at IS NULL`, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
