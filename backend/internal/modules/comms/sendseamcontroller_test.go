// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The lane the installation's own mail rides: that it goes out at all, that the
// one-time link reaches the wire and nothing else, and that the link stops being
// live the moment the message can no longer be sent.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// fakeRelay is the operator relay boundary: it records the message it was
// handed, which is the only place the substituted link is observable.
type fakeRelay struct {
	calls int
	seen  ControllerMessage
	err   error
}

func (f *fakeRelay) SendControllerMail(_ context.Context, msg ControllerMessage) error {
	f.calls++
	f.seen = msg
	return f.err
}

// fakeVault is the key vault boundary. It counts reads and destructions
// separately, because the two answer different questions: whether the link ever
// reached the wire, and whether it stopped being live afterwards.
type fakeVault struct {
	secret   string
	gets     int
	deletes  int
	getErr   error
	deleteEr error
}

func (f *fakeVault) Get(context.Context, string) (string, error) {
	f.gets++
	return f.secret, f.getErr
}

func (f *fakeVault) Delete(context.Context, string) error {
	f.deletes++
	return f.deleteEr
}

// controllerDelivery is the installation's own message: no sending user, no
// consent purpose, one recipient, and a body carrying the link placeholder.
func controllerDelivery() Delivery {
	return Delivery{
		ID: ids.NewV7(), SenderKind: SenderController, Provider: ProviderOperatorRelay,
		MessageID: "confirm-abc@margince.invalid", Recipients: []string{"person@example.com"},
		Subject:     "Your details, and whether we may stay in touch",
		Body:        "Check what we hold:\n\n  " + LinkPlaceholder + "\n",
		TemplateKey: "record_confirmation", TemplateVersion: 1,
		PayloadRef: "vault-ref-1",
		Status:     StatusPending, Attempts: 1, CreatedAt: testNow.Add(-time.Minute),
	}
}

// TestTheLinkReachesTheWireAndNeverTheRow is the lane's central claim: the
// message a person receives carries a working link, and the row it was built
// from never held one.
func TestTheLinkReachesTheWireAndNeverTheRow(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	relay := &fakeRelay{}
	vault := &fakeVault{secret: "https://margince.test/#/confirm/tok123"}
	d := newTestDispatcher(store, fakeResolver{}, &stubConsent{}).
		WithControllerRelay(relay, vault)

	outcome, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID)
	if err != nil || outcome != OutcomeSent {
		t.Fatalf("DispatchWithWait = %v, %v; want %v, nil", outcome, err, OutcomeSent)
	}
	if relay.calls != 1 {
		t.Fatalf("relay called %d times, want exactly 1", relay.calls)
	}
	if !strings.Contains(relay.seen.TextBody, vault.secret) {
		t.Errorf("the transmitted body does not carry the link:\n%s", relay.seen.TextBody)
	}
	if strings.Contains(relay.seen.TextBody, LinkPlaceholder) {
		t.Errorf("the transmitted body still carries the placeholder, so the recipient sees "+
			"%q where the link should be:\n%s", LinkPlaceholder, relay.seen.TextBody)
	}
	// The row is what an audit, an export and a database dump all read. A link
	// that reached it would be a live credential sitting in every one of them.
	if strings.Contains(store.delivery.Body, vault.secret) {
		t.Error("the delivery row holds the plaintext link; it must carry only the placeholder")
	}
}

// TestTheLinkStopsBeingLiveOnceTheMessageIsAccepted holds the retirement to the
// send, not to a later sweep.
func TestTheLinkStopsBeingLiveOnceTheMessageIsAccepted(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	vault := &fakeVault{secret: "https://margince.test/#/confirm/tok123"}
	d := newTestDispatcher(store, fakeResolver{}, &stubConsent{}).
		WithControllerRelay(&fakeRelay{}, vault)

	if _, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID); err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if vault.deletes != 1 {
		t.Errorf("the vault entry was destroyed %d times, want exactly 1: a link that outlives "+
			"its message is a live credential nobody is watching", vault.deletes)
	}
	if store.payloadsCleared != 1 {
		t.Errorf("the row's payload reference was cleared %d times, want exactly 1", store.payloadsCleared)
	}
}

// TestAFailedRetirementDoesNotResendTheMessage pins the rule the cleanup must
// obey: the message really was accepted, so a stumble here may not turn that
// into a retry that mails a second copy.
func TestAFailedRetirementDoesNotResendTheMessage(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	relay := &fakeRelay{}
	d := newTestDispatcher(store, fakeResolver{}, &stubConsent{}).
		WithControllerRelay(relay, &fakeVault{
			secret:   "https://margince.test/#/confirm/tok123",
			deleteEr: errors.New("vault unreachable"),
		})

	outcome, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID)
	if err != nil {
		t.Fatalf("a failed retirement surfaced as an error: %v", err)
	}
	if outcome != OutcomeSent {
		t.Fatalf("DispatchWithWait = %v, want %v: the relay accepted the message, and reporting "+
			"a retry to protect a credential would put a second copy on the wire", outcome, OutcomeSent)
	}
	if relay.calls != 1 {
		t.Errorf("relay called %d times, want exactly 1", relay.calls)
	}
}

// TestAnUnconfiguredRelayParksRatherThanRetries holds the disposition for a
// deployment fact. Retrying would leave a person's link expiring in a queue.
func TestAnUnconfiguredRelayParksRatherThanRetries(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	d := newTestDispatcher(store, fakeResolver{}, &stubConsent{})

	outcome, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID)
	if err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if outcome != OutcomeParked {
		t.Fatalf("DispatchWithWait = %v, want %v", outcome, OutcomeParked)
	}
	if !strings.Contains(store.parked, "relay") {
		t.Errorf("the park reason %q does not name the missing relay, so an operator is not "+
			"told what to configure", store.parked)
	}
}

// TestAParkedNoticeAlsoRetiresItsLink covers the other terminal outcome. A
// message that will never be sent must not leave a working link behind.
func TestAParkedNoticeAlsoRetiresItsLink(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	vault := &fakeVault{secret: "https://margince.test/#/confirm/tok123"}
	// Refused at transmit: the engine says no, so the message is parked.
	consent := &stubConsent{armed: true}
	d := newTestDispatcher(store, fakeResolver{}, consent).
		WithControllerRelay(&fakeRelay{}, vault)

	outcome, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID)
	if err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if outcome != OutcomeParked {
		t.Fatalf("DispatchWithWait = %v, want %v", outcome, OutcomeParked)
	}
	if vault.deletes != 1 {
		t.Errorf("a parked notice destroyed its link %d times, want exactly 1: the message will "+
			"never be sent, so the link must stop working", vault.deletes)
	}
}

// TestTheEngineIsAskedAboutTheInstallationsOwnMail is the "not a side door"
// claim. The lane exists to give this message a decision, not to skip one.
func TestTheEngineIsAskedAboutTheInstallationsOwnMail(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	consent := &stubConsent{}
	d := newTestDispatcher(store, fakeResolver{}, consent).
		WithControllerRelay(&fakeRelay{}, &fakeVault{secret: "https://margince.test/#/confirm/t"})

	if _, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID); err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if consent.authzSeen != 1 {
		t.Errorf("the transmit authority was asked %d times, want exactly 1: a controller message "+
			"that transmits without a decision is exactly the bypass this lane replaced", consent.authzSeen)
	}
}

// legacyPurposeGate answers the way the REAL legacy gate answers: a purpose it
// cannot find is not granted. Its whole value here is that it is not lenient —
// a stub that said yes would let the controller lane pass a question it must
// never be asked.
type legacyPurposeGate struct {
	stubConsent
	askedWithPurpose []string
}

func (g *legacyPurposeGate) RequireGrantedForRecipients(_ context.Context, _ []connector.Recipient, purpose string) error {
	g.askedWithPurpose = append(g.askedWithPurpose, purpose)
	if purpose == "" {
		// Exactly what consent.Gate does: the lookup finds no row, and an
		// undefined purpose grants nothing.
		return apperrors.ErrConsentNotGranted
	}
	return nil
}

// TestTheInstallationsOwnMailIsNotAskedForAPurposeGrant is the case that would
// have shipped a lane that never delivered.
//
// A controller row carries no consent_purpose — it is not sent under a
// permission somebody gave — so putting it to the legacy purpose gate asks a
// question with no possible answer, and the honest "not granted" that comes back
// parks the message. Every confirmation mail, on an installation that looks
// correctly configured.
func TestTheInstallationsOwnMailIsNotAskedForAPurposeGrant(t *testing.T) {
	store := &fakeStore{delivery: controllerDelivery()}
	gate := &legacyPurposeGate{}
	d := newTestDispatcher(store, fakeResolver{}, gate).
		WithControllerRelay(&fakeRelay{}, &fakeVault{secret: "https://margince.test/#/confirm/t"})

	outcome, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID)
	if err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if outcome != OutcomeSent {
		t.Fatalf("DispatchWithWait = %v, want %v (park reason: %q)", outcome, OutcomeSent, store.parked)
	}
	if len(gate.askedWithPurpose) != 0 {
		t.Errorf("the legacy purpose gate was asked about a controller message (purposes: %q). "+
			"That row has no consent_purpose, so the answer can only be 'not granted' and the "+
			"message parks forever", gate.askedWithPurpose)
	}
	// And the engine WAS asked. Skipping the legacy question must not become
	// skipping the decision — that would make the lane the side door it exists
	// not to be.
	if gate.authzSeen != 1 {
		t.Errorf("the transmit authority was asked %d times, want exactly 1", gate.authzSeen)
	}
}

// TestAUserSendIsStillAskedForItsPurposeGrant is the other half. Without it the
// exemption above could widen to every message and nothing would fail.
func TestAUserSendIsStillAskedForItsPurposeGrant(t *testing.T) {
	store := &fakeStore{delivery: liveDelivery()}
	gate := &legacyPurposeGate{}
	d := newTestDispatcher(store, fakeResolver{sender: &fakeSender{}, granted: []string{sendScope}}, gate)

	if _, _, err := d.DispatchWithWait(context.Background(), store.delivery.ID); err != nil {
		t.Fatalf("DispatchWithWait: %v", err)
	}
	if len(gate.askedWithPurpose) != 1 || gate.askedWithPurpose[0] != "marketing" {
		t.Errorf("a rep's send put %q to the legacy gate, want exactly one ask for %q: the "+
			"controller exemption must not widen to ordinary mail",
			gate.askedWithPurpose, "marketing")
	}
}
