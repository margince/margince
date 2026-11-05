// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// The mapping both transports decode through, and the link builder.
//
// These are pure functions carrying judgments a reader cannot recover from the
// types: which addresses are admissible, what a missing capability defaults to,
// and where a credential sits in the URL that carries it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnInviteRefusesAnAddressItCouldNeverReach(t *testing.T) {
	// An address that survives to the row takes the one live seat its room
	// allows for it, fails every send, and — once anyone has signed in — can no
	// longer be corrected. Refusing at the door is the only cheap moment.
	for _, tc := range []struct{ name, email string }{
		{"empty", ""},
		{"blank", "   "},
		{"no domain", "buyer"},
		{"no local part", "@example.com"},
		{"a display name, not a plain address", "Buyer <buyer@example.com>"},
		{"two addresses", "a@example.com, b@example.com"},
		{"a header smuggled in", "buyer@example.com\nBcc: someone@else.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := inviteInput(crmcontracts.InviteDealRoomParticipantRequest{
				FullName: "Buyer",
				Email:    openapi_types.Email(tc.email),
				Source:   "manual",
			})
			if err == nil {
				t.Fatalf("%q must be refused before it reaches a row", tc.email)
			}
		})
	}
}

func TestAnInviteNormalizesTheAddressItAccepts(t *testing.T) {
	// The schema CHECK requires email = lower(email). Normalizing in the mapping
	// rather than the store keeps one spelling for both transports, and means
	// the store never has to re-derive what the validator already settled.
	in, err := inviteInput(crmcontracts.InviteDealRoomParticipantRequest{
		FullName: "Buyer",
		Email:    openapi_types.Email("  Buyer@Example.COM  "),
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("a well-formed address must be accepted: %v", err)
	}
	if in.Email != "buyer@example.com" {
		t.Errorf("Email = %q, want it trimmed and lowercased", in.Email)
	}
}

func TestAnInviteWithoutACapabilityGetsTheLeastOne(t *testing.T) {
	// Defaulting up would let somebody nobody granted a voice write into the
	// room's conversation under their own name.
	in, err := inviteInput(crmcontracts.InviteDealRoomParticipantRequest{
		FullName: "Buyer",
		Email:    openapi_types.Email("buyer@example.com"),
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("invite must be accepted: %v", err)
	}
	if in.Capability != capabilityView {
		t.Errorf("Capability = %q, want %q — an omitted capability must default to the least", in.Capability, capabilityView)
	}
}

func TestACorrectionValidatesTheAddressToo(t *testing.T) {
	// The correction path is where a bad address does the most harm, because it
	// replaces one that may already have been delivered.
	bad := openapi_types.Email("not an address")
	if _, err := participantUpdateInput(crmcontracts.UpdateDealRoomParticipantRequest{
		Email: &bad,
	}); err == nil {
		t.Error("a malformed correction must be refused")
	}

	good := openapi_types.Email("Corrected@Example.com")
	in, err := participantUpdateInput(crmcontracts.UpdateDealRoomParticipantRequest{Email: &good})
	if err != nil {
		t.Fatalf("a well-formed correction must be accepted: %v", err)
	}
	if in.Email == nil || *in.Email != "corrected@example.com" {
		t.Errorf("Email = %v, want it lowercased", in.Email)
	}
}

func TestTheBuyerLinkCarriesTheCredentialInTheFragment(t *testing.T) {
	// A browser does not put a fragment on the wire, so the credential stays out
	// of access logs and out of the Referer a click sends onward. If this ever
	// moves into the path or the query, it starts appearing in both.
	h := Handlers{}.WithInviteLinkBase("https://example.test/")
	link := h.buyerLink("mdr_secret")

	fragment := strings.Index(link, "#")
	if fragment < 0 {
		t.Fatalf("the link carries no fragment: %q", link)
	}
	if strings.Contains(link[:fragment], "mdr_secret") {
		t.Errorf("the credential appears before the fragment, so a browser would send it: %q", link)
	}
	if !strings.HasPrefix(link, "https://example.test/#") {
		t.Errorf("the trailing slash must be trimmed exactly once: %q", link)
	}
}

func TestAnInviteRefusesAnUnboundedName(t *testing.T) {
	_, err := inviteInput(crmcontracts.InviteDealRoomParticipantRequest{
		FullName: strings.Repeat("n", nameLimit+1),
		Email:    openapi_types.Email("buyer@example.com"),
		Source:   "manual",
	})
	var fault interface {
		FieldFault() (string, string, string)
	}
	if !errors.As(err, &fault) {
		t.Fatalf("an overlong name must be refused as a field fault, got %v", err)
	}
	if field, _, _ := fault.FieldFault(); field != "full_name" {
		t.Errorf("the refusal must name full_name, got %q", field)
	}
}

func TestAttributionFailureDoesNotDiscardABuyersAccessLink(t *testing.T) {
	relay := &invitationMailCapture{}
	handler := Handlers{}.WithInviteMailer(relay).WithInviteLinkBase("https://example.test").WithInviteContext(func(context.Context) (string, string, error) {
		return "", "", errors.New("company settings unavailable")
	})
	invitation := IssuedInvitation{RoomTitle: "Northstar proposal", Credential: "fictional", ExpiresAt: time.Date(2026, 11, 15, 12, 37, 0, 0, time.UTC), Participant: crmcontracts.DealRoomParticipant{Email: openapi_types.Email("buyer@example.test")}}
	if err := handler.sendInvite(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	if relay.to != "buyer@example.test" || !strings.Contains(relay.body, "https://example.test/#/room?c=fictional") || !strings.Contains(relay.subject, "Northstar proposal") {
		t.Fatal("optional attribution prevented a useful access message")
	}
}

type invitationMailCapture struct{ to, subject, body string }

func (m *invitationMailCapture) Send(_ context.Context, to, subject, body string) error {
	m.to, m.subject, m.body = to, subject, body
	return nil
}

// silentRelay never answers, and gives up when the send's deadline passes, as
// the SMTP relay does.
type silentRelay struct{}

func (silentRelay) Send(ctx context.Context, _, _, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

// deafRelay ignores the send's deadline and takes the mail only when the test
// releases it: a relay whose answer comes after the invite stopped waiting.
type deafRelay struct {
	release chan struct{}
	sends   *int
}

func (m deafRelay) Send(context.Context, string, string, string) error {
	*m.sends++
	<-m.release
	return nil
}

// stubInviteRecords answers whether the invitation may still be mailed, and
// keeps the outcomes the send recorded.
type stubInviteRecords struct {
	mailable bool
	recorded []error
}

func (s *stubInviteRecords) InvitationMailable(context.Context, string, string) (bool, error) {
	return s.mailable, nil
}

func (s *stubInviteRecords) RecordInvitationSend(_ context.Context, _ ids.DealRoomParticipantID, _ string, sendErr error) error {
	s.recorded = append(s.recorded, sendErr)
	return nil
}

func heldInvite() IssuedInvitation {
	return IssuedInvitation{RoomTitle: "Northstar proposal", Credential: "fictional", ExpiresAt: time.Date(2026, 11, 15, 12, 37, 0, 0, time.UTC), Participant: crmcontracts.DealRoomParticipant{Email: openapi_types.Email("buyer@example.test")}}
}

// inviteHandler is a handler on a relay, with the invitation records stubbed
// and a channel closed once the send and its record are over.
func inviteHandler(relay interface {
	Send(context.Context, string, string, string) error
}, records *stubInviteRecords) (Handlers, chan struct{}) {
	finished := make(chan struct{})
	handler := Handlers{}.WithInviteMailer(relay).WithInviteLinkBase("https://example.test")
	handler.inviteRecords = records
	handler.sendDone = func() { close(finished) }
	return handler, finished
}

// A relay that takes the mail within the wait reads as emailed.
func TestAnInviteReportsTheMailTheRelayTook(t *testing.T) {
	relay := &invitationMailCapture{}
	records := &stubInviteRecords{mailable: true}
	handler, finished := inviteHandler(relay, records)

	body := handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), heldInvite())
	<-finished
	if !body.Queued {
		t.Error("an invitation the relay took reads as not emailed")
	}
	if relay.to != "buyer@example.test" {
		t.Errorf("mailed to %q", relay.to)
	}
	if len(records.recorded) != 1 || records.recorded[0] != nil {
		t.Errorf("recorded %v, want one sent outcome", records.recorded)
	}
}

// A relay that refuses the mail reads as not emailed, so the seller copies the
// link, and the refusal lands on the roster.
func TestAnInviteTheRelayRefusedReadsAsNotEmailed(t *testing.T) {
	records := &stubInviteRecords{mailable: true}
	handler, finished := inviteHandler(refusingRelay{}, records)

	body := handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), heldInvite())
	<-finished
	if body.Queued {
		t.Error("an invitation the relay refused reads as emailed")
	}
	if len(records.recorded) != 1 || records.recorded[0] == nil {
		t.Errorf("recorded %v, want one failed outcome", records.recorded)
	}
}

type refusingRelay struct{}

func (refusingRelay) Send(context.Context, string, string, string) error {
	return errors.New("the relay refused the envelope")
}

// A relay that has not answered when the wait ends reads as not emailed. The
// send stops at the same moment and records why it failed.
func TestARelayThatDoesNotAnswerInTimeReadsAsNotEmailed(t *testing.T) {
	records := &stubInviteRecords{mailable: true}
	handler, finished := inviteHandler(silentRelay{}, records)
	handler.sendBudget = 50 * time.Millisecond

	body := handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), heldInvite())
	<-finished
	if body.Queued {
		t.Error("an invitation the relay never answered reads as emailed")
	}
	if len(records.recorded) != 1 || records.recorded[0] == nil ||
		!strings.Contains(records.recorded[0].Error(), "did not answer within") {
		t.Errorf("recorded %v, want one failure that says the relay did not answer in time", records.recorded)
	}
}

// A relay that ignores the deadline and takes the mail after the invite
// answered does not turn that answer, and the mail goes out once. The roster
// then says what the relay did.
func TestALateRelayAnswerKeepsTheInvitesAnswerAndSendsOnce(t *testing.T) {
	sends := 0
	relay := deafRelay{release: make(chan struct{}), sends: &sends}
	records := &stubInviteRecords{mailable: true}
	handler, finished := inviteHandler(relay, records)
	handler.sendBudget = 50 * time.Millisecond

	body := handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), heldInvite())
	close(relay.release)
	<-finished
	if body.Queued {
		t.Error("the invite waited past its budget for a relay that did not answer")
	}
	if sends != 1 {
		t.Errorf("the relay was asked %d times, want once", sends)
	}
	if len(records.recorded) != 1 {
		t.Errorf("recorded %v, want the relay's one answer", records.recorded)
	}
}

// An invitation revoked, resent, readdressed or archived after it was issued is
// no longer mailable. Its old link must not go out, and nothing is recorded.
func TestAnInvitationThatChangedBeforeTheSendIsNotMailed(t *testing.T) {
	relay := &invitationMailCapture{}
	records := &stubInviteRecords{mailable: false}
	handler, finished := inviteHandler(relay, records)

	body := handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), heldInvite())
	<-finished
	if body.Queued {
		t.Error("an invitation that was not mailed reads as emailed")
	}
	if relay.to != "" {
		t.Errorf("the old link was mailed to %q", relay.to)
	}
	if len(records.recorded) != 0 {
		t.Errorf("recorded %v for a mail that never went out", records.recorded)
	}
}

// With no relay nothing is handed over, and the answer says so.
func TestAnInviteWithNoRelayReadsAsNotQueued(t *testing.T) {
	handler := Handlers{}.WithInviteLinkBase("https://example.test")
	handler.sendDone = func() { t.Error("a send started with no relay configured") }
	invitation := IssuedInvitation{Credential: "fictional", Participant: crmcontracts.DealRoomParticipant{Email: openapi_types.Email("buyer@example.test")}}
	if handler.issuedBody(httptest.NewRequest(http.MethodPost, "/", nil), invitation).Queued {
		t.Error("an installation with no relay reads as queued")
	}
}
