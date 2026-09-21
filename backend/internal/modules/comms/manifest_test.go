// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The requirement gate, asked of the message on its way to the provider.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// stubRequirements answers a fixed set and records what it was asked about.
type stubRequirements struct {
	findings []RequirementFinding
	err      error
	asked    []FinalMessage
}

func (s *stubRequirements) CheckMessage(
	_ context.Context, m FinalMessage,
) ([]RequirementFinding, error) {
	s.asked = append(s.asked, m)
	return s.findings, s.err
}

// dispatcherWithRequirements is the unit harness the file beside this uses,
// plus the checker under test.
func dispatcherWithRequirements(
	store *fakeStore, sender *fakeSender, c RequirementChecker,
) *Dispatcher {
	return NewDispatcher(store, fakeResolver{sender: sender, granted: []string{sendScope}},
		liveSeat(), nil, &stubConsent{}, nil, func() time.Time { return testNow },
		time.Hour, 5).WithRequirementChecker(c)
}

// TestAMessageMissingARequirementParksAndNamesIt.
//
// PARKED, NOT RETRIED. The next attempt composes the same bytes from the same
// row, so retrying would burn the ladder and park anyway, having sent nothing
// and said nothing useful in between.
func TestAMessageMissingARequirementParksAndNamesIt(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	d := dispatcherWithRequirements(store, sender, &stubRequirements{
		findings: []RequirementFinding{{
			Requirement: "subject_prefix",
			Detail:      `an advertising subject carries "[QC]" in vn`,
		}},
	})

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeParked {
		t.Errorf("outcome = %v, want parked — a message missing what its jurisdiction "+
			"requires must not reach a provider", got)
	}
	if sender.calls != 0 {
		t.Errorf("the provider was called %d times for a message that owes a requirement",
			sender.calls)
	}
	if !strings.Contains(store.parked, "subject_prefix") {
		t.Errorf("the parked reason is %q and names no requirement — an operator reading it "+
			"cannot tell what is missing", store.parked)
	}
}

// TestAMessageMeetingEveryRequirementStillSends is the positive control: the
// park above must be about the finding, not about the gate refusing everything.
func TestAMessageMeetingEveryRequirementStillSends(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	d := dispatcherWithRequirements(store, sender, &stubRequirements{})

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeSent || sender.calls != 1 {
		t.Errorf("outcome=%v calls=%d, want sent/1", got, sender.calls)
	}
}

// TestNoCheckerWiredSendsExactlyAsBefore. Every test store and the channel seam
// have no opinion about compliance packs, and a nil checker must leave them
// working rather than parking their mail.
func TestNoCheckerWiredSendsExactlyAsBefore(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	d := dispatcherWithRequirements(store, sender, nil)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != OutcomeSent || sender.calls != 1 {
		t.Errorf("outcome=%v calls=%d, want sent/1 — a dispatcher with no checker must "+
			"behave as it did before this gate existed", got, sender.calls)
	}
}

// TestTheCheckerSeesTheMessageAsTheProviderWould.
//
// Both body alternatives travel, because a requirement met in one and not the
// other is met for whichever readers happen to fall back — which is not a
// standard anybody writes down.
func TestTheCheckerSeesTheMessageAsTheProviderWould(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	stub := &stubRequirements{}
	d := dispatcherWithRequirements(store, sender, stub)

	if _, err := dispatch(context.Background(), d, store.delivery.ID); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(stub.asked) != 1 {
		t.Fatalf("the checker was asked %d times, want once", len(stub.asked))
	}
	asked := stub.asked[0]
	if asked.Subject != store.delivery.Subject {
		t.Errorf("subject asked = %q, want %q", asked.Subject, store.delivery.Subject)
	}
	if asked.Body != store.delivery.Body {
		t.Errorf("body asked = %q, want the composed body", asked.Body)
	}
	if asked.DeliveryID != store.delivery.ID.String() {
		t.Errorf("delivery asked = %q, want %q — a finding that cannot name its delivery "+
			"is unactionable", asked.DeliveryID, store.delivery.ID)
	}
}

// TestACheckerThatCannotAnswerDoesNotSend.
//
// A failed check is not a passed one. Treating an error as "nothing owed" would
// send the message the gate could not vouch for, which is the direction that
// must never be the quiet default.
func TestACheckerThatCannotAnswerDoesNotSend(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{delivery: liveDelivery()}
	d := dispatcherWithRequirements(store, sender, &stubRequirements{
		err: errors.New("the settings table is unreachable"),
	})

	if _, err := dispatch(context.Background(), d, store.delivery.ID); err == nil {
		t.Fatal("a checker that could not answer reported no error, so the send proceeded " +
			"on a check nobody performed")
	}
	if sender.calls != 0 {
		t.Errorf("the provider was called %d times despite an unanswerable check", sender.calls)
	}
}

// TestAChannelMessageIsNotAskedForASubjectPrefix is Codex's finding, and it
// would have parked every channel message on a Vietnamese installation.
//
// A channel delivery has no subject: staging writes NULL and the provider's own
// message carries no such field. Asking the check about one hands it an empty
// string, finds the label missing, and parks a message that has no way to carry
// a label and was never under an obligation to.
func TestAChannelMessageIsNotAskedForASubjectPrefix(t *testing.T) {
	channel := &stubMessageSender{}
	store := &fakeStore{delivery: channelDelivery()}
	stub := &stubRequirements{
		findings: []RequirementFinding{{Requirement: "subject_prefix"}},
	}
	d := newTestDispatcher(store, fakeResolver{channel: channel},
		&stubConsent{}).WithRequirementChecker(stub)

	got, err := dispatch(context.Background(), d, store.delivery.ID)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got == OutcomeParked {
		t.Errorf("a channel message was parked for a subject it cannot carry: %q", store.parked)
	}
	if channel.calls != 1 {
		t.Errorf("the provider was called %d time(s), want 1", channel.calls)
	}
	if len(stub.asked) != 0 {
		t.Errorf("the checker was asked about a channel message %d times — it has no subject "+
			"to judge and the empty string is not one", len(stub.asked))
	}
}
