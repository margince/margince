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
