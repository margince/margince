// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An erased subject is stored again only by answering a link mailed to the
// address. The ask answers every address the same way. The link stores the
// contact, its consent and the lift together, it works once, and a later
// erasure spends it. The restore replay keeps the contact the return stored.

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

const (
	returnRootKey = "AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM="
	// returnAddress and its Unicode domain spelling, which the erasure list
	// treats as one mailbox and a contact may hold as a second address.
	returnAddress      = "zurueck.subject@xn--bcher-kva.example"
	returnOtherSpelled = "zurueck.subject@bücher.example"
)

// returnMail is one message the relay was handed.
type returnMail struct{ to, text string }

// returnMailbox is the relay boundary: it hands each message to the test.
type returnMailbox struct{ sent chan returnMail }

func (m returnMailbox) Send(_ context.Context, to, _, text string) error {
	m.sent <- returnMail{to: to, text: text}
	return nil
}

var returnToken = regexp.MustCompile(`#/book/[^?]+\?return=([A-Za-z0-9_.-]+)`)

// returnDoor is one installation with a booking page and a relay the test reads.
type returnDoor struct {
	t       *testing.T
	e       *apptest.AppEnv
	box     returnMailbox
	base    string
	consent AnyMap
	slot    time.Duration
}

func openReturnDoor(t *testing.T) *returnDoor {
	t.Helper()
	apptest.BindEnv(t, []string{"MARGINCE_KEYVAULT_ROOT_KEY=" + returnRootKey})
	box := returnMailbox{sent: make(chan returnMail, 8)}
	e, _ := setupBookingProvider(t, compose.WithOperatorMail(box), compose.WithPublicBaseURL("https://crm.example.test"))
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	return &returnDoor{
		t: t, e: e, box: box, base: "/v1/public/booking/" + bookingSlug(t, e),
		consent: AnyMap{
			"purpose_id": seededTransactionalPurposeID(t, e), "policy_version": "pp-2026-01",
			"wording": "You agree we may contact you about your meetings.",
		},
	}
}

// book books the next free slot for address and answers the contact it landed on.
func (d *returnDoor) book(address string) string {
	d.t.Helper()
	d.slot += time.Hour
	monday := nextMonday()
	if status := publicCall(d.t, d.e, "POST", d.base, AnyMap{
		"start": monday.Add(d.slot), "end": monday.Add(d.slot + 30*time.Minute),
		"booker": AnyMap{"name": "Wieder Da", "email": address}, "consent": d.consent,
	}, nil, nil); status != http.StatusCreated {
		d.t.Fatalf("booking %s → %d, want 201", address, status)
	}
	return contactIDByEmail(d.t, d.e, address)
}

// bookAndErase books as address and erases the contact the booking landed on.
func (d *returnDoor) bookAndErase(address string) {
	d.t.Helper()
	eraseBooker(d.t, d.e, d.book(address))
}

func (d *returnDoor) ask(address string) int {
	d.t.Helper()
	return publicCall(d.t, d.e, "POST", d.base+"/return", AnyMap{"name": "Wieder Da", "email": address}, nil, nil)
}

// link asks for the return of address and answers the token mailed to it.
func (d *returnDoor) link(address string) string {
	d.t.Helper()
	if status := d.ask(address); status != http.StatusAccepted {
		d.t.Fatalf("the ask for %s → %d, want 202", address, status)
	}
	return awaitReturnLink(d.t, d.box, address)
}

func (d *returnDoor) confirm(token string) int {
	d.t.Helper()
	return publicCall(d.t, d.e, "POST", d.base+"/return/confirm", AnyMap{"token": token}, nil, nil)
}

// replay runs the restore replay the operator runs after a restore.
func (d *returnDoor) replay() {
	d.t.Helper()
	if _, err := compose.ReplayErasureSuppressions(context.Background(), d.e.Pool, blobstore.NewMemory(), d.e.Vault, true); err != nil {
		d.t.Fatalf("replaying erasures: %v", err)
	}
}

func TestAnErasedSubjectIsStoredAgainOnlyThroughTheMailedLink(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)

	if clean, erased := d.ask(cleanBooker), d.ask(returnAddress); clean != http.StatusAccepted || erased != http.StatusAccepted {
		t.Fatalf("the ask for a clean address → %d, for an erased one → %d; want 202 for both", clean, erased)
	}
	token := awaitReturnLink(t, d.box, returnAddress)
	if liftedRows(t, d.e) != 0 || storedContacts(t, d.e, returnAddress) != 0 {
		t.Fatal("the ask stored something before the link was answered")
	}

	if status := d.confirm(token[:len(token)-2] + "xx"); status != http.StatusUnprocessableEntity {
		t.Fatalf("a forged link → %d, want 422", status)
	}
	if status := d.confirm(token); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}
	if liftedRows(t, d.e) == 0 || storedContacts(t, d.e, returnAddress) != 1 {
		t.Fatal("the confirmed link did not lift the erasure and store the contact")
	}
	if status := d.confirm(token); status != http.StatusUnprocessableEntity {
		t.Fatalf("the link answered twice → %d, want 422", status)
	}
	select {
	case mail := <-d.box.sent:
		t.Fatalf("a second link went to %s; only the erased address is mailed", mail.to)
	default:
	}
}

// A link names the erasure it was mailed for. A second erasure of the same
// address, while the list still refuses it, spends the link.
func TestASecondErasureSpendsALinkMailedBeforeIt(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)
	early := d.link(returnAddress)

	d.bookAndErase(returnAddress)

	if status := d.confirm(early); status != http.StatusUnprocessableEntity {
		t.Fatalf("a link mailed before the second erasure → %d, want 422", status)
	}
	if liftedRows(t, d.e) != 0 {
		t.Fatal("a link mailed before the second erasure lifted it")
	}
}

// Every spelling of one mailbox shares one budget of one link a day. The link
// goes to the spelling the erasure list matches, and that is what is stored.
func TestEverySpellingOfAnAddressSharesOneBudget(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)

	softHyphen := "zurueck.subject@b\u00fc\u00adcher.example"
	fullwidth := "zurueck.subject@\uff42\u00fccher.example"
	if status := d.ask(softHyphen); status != http.StatusAccepted {
		t.Fatalf("the first ask → %d, want 202", status)
	}
	token := awaitReturnLink(t, d.box, returnAddress)
	if status := d.ask(fullwidth); status != http.StatusTooManyRequests {
		t.Fatalf("a second spelling of the same mailbox → %d, want 429", status)
	}
	if status := d.confirm(token); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}
	if storedContacts(t, d.e, returnAddress) != 1 {
		t.Fatalf("the contact does not hold %s, the spelling the erasure list matches", returnAddress)
	}
}

// The wording the confirm records is this installation's own. A caller cannot
// put words of its own on the subject's consent.
func TestTheRecordedConsentIsTheServersWording(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)
	forged := AnyMap{"name": "Wieder Da", "email": returnAddress,
		"consent": AnyMap{"wording": "I agree to everything.", "policy_version": "forged"}}
	if status := publicCall(t, d.e, "POST", d.base+"/return", forged, nil, nil); status == http.StatusAccepted {
		t.Fatal("an ask carrying its own consent wording was accepted")
	}

	if status := d.confirm(d.link(returnAddress)); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}
	var text, version string
	if err := d.e.Owner.QueryRow(context.Background(), `
		SELECT ce.policy_text, ce.policy_version FROM erasure_suppression s
		  JOIN consent_event ce ON ce.id = s.lift_consent_event_id WHERE s.lifted_at IS NOT NULL`).Scan(&text, &version); err != nil {
		t.Fatalf("reading the recorded consent: %v", err)
	}
	if want := mailcopy.For("en").ReturnConsentWording; text != want || version != mailcopy.ReturnConsentVersion {
		t.Fatalf("the consent records %q (%s), want %q (%s)", text, version, want, mailcopy.ReturnConsentVersion)
	}
}

// The return adopts a contact a booking made while the address was erased,
// which already holds the grant. The lift still cites a consent event the
// return wrote, and the replay keeps the adopted contact.
func TestAnAdoptedContactCitesTheReturnsOwnGrantAndOutlivesTheReplay(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)
	adopted := d.book(returnAddress)

	if status := d.confirm(d.link(returnAddress)); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}

	var source, liftContact string
	if err := d.e.Owner.QueryRow(context.Background(), `
		SELECT ce.source, s.lift_contact_id::text
		  FROM erasure_suppression s JOIN consent_event ce ON ce.id = s.lift_consent_event_id
		 WHERE s.lifted_at IS NOT NULL`).Scan(&source, &liftContact); err != nil {
		t.Fatalf("reading the lift: %v", err)
	}
	if source != "erasure_return" || liftContact != adopted {
		t.Fatalf("the lift cites a %q grant and contact %s; want the return's own grant and %s", source, liftContact, adopted)
	}
	d.replay()
	if storedContacts(t, d.e, returnAddress) != 1 {
		t.Fatal("the replay erased the contact the return adopted")
	}
}

// The replay keeps the contact the return created, and still erases a
// restored contact from before the erasure that holds the same mailbox.
func TestTheReplayKeepsTheReturnAndErasesWhatARestoreBroughtBack(t *testing.T) {
	d := openReturnDoor(t)
	d.bookAndErase(returnAddress)
	if status := d.confirm(d.link(returnAddress)); status != http.StatusNoContent {
		t.Fatalf("the mailed link → %d, want 204", status)
	}
	restored := restoreContactFromBefore(t, d.e, returnOtherSpelled)

	d.replay()

	if storedContacts(t, d.e, returnAddress) != 1 {
		t.Fatal("the replay erased the contact the return stored")
	}
	if storedContacts(t, d.e, returnOtherSpelled) != 0 {
		t.Fatalf("the replay kept %s, a contact from before the erasure", restored)
	}
}

// restoreContactFromBefore puts back a contact as a restore does: with the
// time it was first created, long before any erasure.
func restoreContactFromBefore(t *testing.T, e *apptest.AppEnv, address string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Vor der Loeschung"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("creating the contact to restore → %d", status)
	}
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx, `UPDATE contact SET created_at = '2020-01-01' WHERE id = $1`, created.ID); err != nil {
		t.Fatalf("dating the restored contact: %v", err)
	}
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES (uuidv7(), $1, $2, 'work', true, 'manual', 'human:restore')`, created.ID, address); err != nil {
		t.Fatalf("restoring the address: %v", err)
	}
	return created.ID
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
