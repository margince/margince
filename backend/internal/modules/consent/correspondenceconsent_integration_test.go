// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What an explicit answer about ordinary correspondence is worth.
//
// An integration test because the defect these pin was invisible to every unit
// test in the tree: the arm READ the wrong table. Only the real query against a
// real row shows that a `granted` person_consent row now reaches the verdict,
// and that a withdrawal still beats it.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// grant records the subject's own answer through the production writer.
//
// Store.Record rather than an INSERT: the row this arm reads is the one the
// endpoint writes, and a hand-inserted row would prove the query works against
// a shape nothing in production produces.
func (e *qualifyingEnv) grant(t *testing.T, state string) {
	t.Helper()
	source := "verbal"
	if _, err := e.store.Record(e.ctx, RecordInput{
		PersonID:  e.person,
		PurposeID: ids.From[ids.PurposeKind](ids.MustParse(e.correspondence.ID)),
		NewState:  state,
		Source:    &source,
	}); err != nil {
		t.Fatalf("recording consent %q: %v", state, err)
	}
}

// TestAnExplicitYesAuthorizesCorrespondence is the defect this file exists for.
//
// A person who has said in as many words that we may write to them was refused
// until they happened to send an inbound message, because the arm read
// qualifying events and never the recorded answer. Mutation: drop the
// `granted` branch from correspondenceVerdict and this fails on the first
// assertion, with the verdict still unknown.
func TestAnExplicitYesAuthorizesCorrespondence(t *testing.T) {
	e := setupQualifying(t)

	// Nothing on the timeline: no inbound, no deal, no recorded exchange. The
	// ONLY thing that will change is the person's own answer.
	if before := e.verdict(t); before.State != VerdictUnknown {
		t.Fatalf("the starting verdict = %q, want unknown", before.State)
	}

	e.grant(t, "granted")

	after := e.verdict(t)
	if after.State != VerdictAllowed {
		t.Fatalf("the verdict after an explicit yes = %q (%s), want allowed",
			after.State, after.Reason)
	}
	// The reason has to name the answer, not a qualifying event: there is no
	// event here, and a verdict citing one would be describing something that
	// never happened.
	if after.Qualifying != nil {
		t.Errorf("the verdict cites the qualifying event %+v, want the recorded answer",
			after.Qualifying)
	}
}

// TestAWithdrawalStillBeatsAQualifyingEvent holds the direction the fix must
// not break. objectionStands runs before the class arms, so reading the grant
// first must not give a withdrawn person a way back in through the timeline.
//
// Mutation: move the objectionStands call below the class switch and this
// fails — the inbound message answers instead.
func TestAWithdrawalStillBeatsAQualifyingEvent(t *testing.T) {
	e := setupQualifying(t)

	met := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if _, err := e.store.RecordQualifyingEvent(e.ctx, e.person, RecordQualifyingEventInput{
		Kind:       "in_person",
		Note:       "Met at the Frankfurt trade fair, stand B12.",
		OccurredAt: met,
	}); err != nil {
		t.Fatalf("recording the exchange: %v", err)
	}
	if allowed := e.verdict(t); allowed.State != VerdictAllowed {
		t.Fatalf("the verdict on the exchange alone = %q, want allowed", allowed.State)
	}

	e.grant(t, "withdrawn")

	after := e.verdict(t)
	if after.State != VerdictBlocked {
		t.Fatalf("the verdict after a withdrawal = %q (%s), want blocked",
			after.State, after.Reason)
	}
	if after.Code != BlockObjection {
		t.Errorf("the block code = %q, want %q", after.Code, BlockObjection)
	}
}

// TestTheTimelineStillAnswersForSomebodyWhoNeverSaid keeps the implied arm
// reachable. It answers for the overwhelming majority of people, who record no
// answer either way, and reading the grant first must not have shadowed it.
//
// Mutation: return early from correspondenceVerdict on any non-granted state
// and this fails.
func TestTheTimelineStillAnswersForSomebodyWhoNeverSaid(t *testing.T) {
	e := setupQualifying(t)

	met := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	if _, err := e.store.RecordQualifyingEvent(e.ctx, e.person, RecordQualifyingEventInput{
		Kind:       "in_person",
		Note:       "They asked for a quote at the stand.",
		OccurredAt: met,
	}); err != nil {
		t.Fatalf("recording the exchange: %v", err)
	}

	after := e.verdict(t)
	if after.State != VerdictAllowed {
		t.Fatalf("the verdict = %q (%s), want allowed", after.State, after.Reason)
	}
	if after.Qualifying == nil || after.Qualifying.Kind != "in_person" {
		t.Errorf("the verdict cites %+v, want the in-person exchange", after.Qualifying)
	}
}

// TestAnInboundStopsAuthorizingWhenItLapses is the rule Lars chose: an inbound
// message opens broader contact for a bounded window, then it lapses back to
// needing its own reason.
//
// It used to open everything, permanently. A person who asked one question in
// 2024 authorized unrelated outreach in 2026, because the arm read "is there
// any inbound from them" with no bound at all — which is not what the
// relationship they started actually says.
//
// What this does NOT touch is a reply. resolveCategory answers
// CategoryReplyToInbound from the anchor thread before the legacy verdict is
// consulted, so a rep answering a two-year-old thread is unaffected.
//
// Mutation: drop `occurred_at >= $2` from inboundQualifyingEvent and the
// lapsed case allows, failing the second assertion.
func TestAnInboundStopsAuthorizingWhenItLapses(t *testing.T) {
	e := setupQualifying(t)

	fresh := time.Now().Add(-30 * 24 * time.Hour)
	e.inbound(t, fresh)

	within := e.verdictSince(t, time.Now().Add(-defaultReplyWindow))
	if within.State != VerdictAllowed {
		t.Fatalf("a month-old inbound = %q (%s), want allowed — the window is twelve months",
			within.State, within.Reason)
	}

	// The SAME row, judged from a window that has moved past it. Nothing about
	// the record changed; only how far back the rule is willing to look.
	lapsed := e.verdictSince(t, fresh.Add(24*time.Hour))
	if lapsed.State != VerdictUnknown {
		t.Fatalf("an inbound older than the window = %q (%s), want unknown — a lapsed contact is not standing permission",
			lapsed.State, lapsed.Reason)
	}
}

// TestARecordedExchangeLapsesToo holds the other source to the same rule.
//
// The typed row and the derived inbound are two spellings of "something on
// file says we may write to them", and a window that bound only one of them
// would leave a two-year-old trade-fair note authorizing today's cold mail
// while a two-year-old email did not.
//
// Mutation: drop `occurred_at >= $2` from recordedQualifyingEvent and this
// fails while TestAnInboundStopsAuthorizingWhenItLapses still passes, which is
// what makes it a separate test rather than a second assertion.
func TestARecordedExchangeLapsesToo(t *testing.T) {
	e := setupQualifying(t)

	met := time.Now().Add(-60 * 24 * time.Hour).Truncate(time.Second)
	if _, err := e.store.RecordQualifyingEvent(e.ctx, e.person, RecordQualifyingEventInput{
		Kind:       "in_person",
		Note:       "Met at the Frankfurt trade fair, stand B12.",
		OccurredAt: met,
	}); err != nil {
		t.Fatalf("recording the exchange: %v", err)
	}

	if within := e.verdictSince(t, time.Now().Add(-defaultReplyWindow)); within.State != VerdictAllowed {
		t.Fatalf("a two-month-old exchange = %q, want allowed", within.State)
	}
	if lapsed := e.verdictSince(t, met.Add(24*time.Hour)); lapsed.State != VerdictUnknown {
		t.Fatalf("an exchange older than the window = %q (%s), want unknown",
			lapsed.State, lapsed.Reason)
	}
}

// TestAnExplicitYesOutlivesTheWindow is the direction the window must not
// break. A recorded grant is the subject's own answer about being written to,
// and it does not expire because they have not been in touch lately — reading
// it before the timeline is what #4045 fixed, and a window applied to it would
// undo that fix from the other side.
//
// Mutation: move the recordedState branch below latestQualifyingEvent in
// correspondenceVerdict and this fails.
func TestAnExplicitYesOutlivesTheWindow(t *testing.T) {
	e := setupQualifying(t)

	e.grant(t, "granted")

	// A window that closed years ago. The grant is not evidence read off the
	// timeline, so nothing here should consult it at all.
	after := e.verdictSince(t, time.Now())
	if after.State != VerdictAllowed {
		t.Fatalf("an explicit yes under a closed window = %q (%s), want allowed",
			after.State, after.Reason)
	}
}

// TestTheGuardAnswersOnTheSameWindowAsTheSend is why the window is resolved by
// the store rather than by each caller.
//
// The guard endpoint is the preview a composer shows before a rep writes
// anything. If it read a different span than the send path binds a decision
// to, it would tell them a message is allowed and the engine would then refuse
// it — a preview that is wrong in the permissive direction is worse than none,
// because the rep only finds out after writing the mail.
//
// The pin is that the guard reports the SAME verdict as VerdictForPerson for
// the same person under the same window, across the lapse boundary. Mutation:
// replace the guard's `since` with a zero time and the lapsed case reports
// allowed while the send path says unknown, failing the second assertion.
func TestTheGuardAnswersOnTheSameWindowAsTheSend(t *testing.T) {
	e := setupQualifying(t)

	// Old enough that the default twelve-month window has closed on it.
	e.inbound(t, time.Now().Add(-400*24*time.Hour))

	guard, err := e.store.PersonConsentGuard(e.ctx, e.person)
	if err != nil {
		t.Fatalf("reading the guard: %v", err)
	}
	var entry *crmcontracts.PersonConsentGuardEntry
	for i := range guard.Entries {
		if guard.Entries[i].PurposeKey == "business_correspondence" {
			entry = &guard.Entries[i]
		}
	}
	if entry == nil {
		t.Fatal("the guard has no business_correspondence entry — the fixture's purpose is not reaching it")
	}
	if entry.Verdict != crmcontracts.PersonConsentGuardEntryVerdictUnknown {
		t.Errorf("the guard reports %q for a 400-day-old inbound, want unknown; the send path refuses it, so the preview would promise a send that is then refused",
			entry.Verdict)
	}

	// And the send path, asked directly, agrees. Both assertions together are
	// the point: either alone would pass against a build where BOTH had lost
	// the window.
	if v := e.verdict(t); v.State != VerdictUnknown {
		t.Errorf("the send path verdict on the same lapsed inbound = %q, want unknown", v.State)
	}
}
