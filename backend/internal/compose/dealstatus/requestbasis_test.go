// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestHistoricalReviewHasEvidenceWithoutAReplyObligation(t *testing.T) {
	source := inboundMail(testNow.AddDate(0, 0, -200))
	source.EmailSummary = &crmcontracts.EmailSummary{Move: crmcontracts.EmailSummaryMoveNeedsReply}
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{source}}
	mv := decideMove(f)
	card := composeDeterministic(f, mv)
	if card.ReplyTo != nil || card.RequestBasis != nil || mv.Action != ActionCreateTask || mv.Basis == nil || mv.Basis.Kind != basisRequestReview {
		t.Fatalf("historical review became current work: %+v", card)
	}
	in := project(f, mv)
	if in.ReplyTo != "" || in.RequestStates[source.Id.String()] != basisRequestReview || !citableIDs(in)[source.Id.String()] {
		t.Fatal("the model lost the review classification or its evidence")
	}
	model := WrittenStatus{MoveReason: WrittenLine{Text: "The buyer has waited 200 days. Reply now.", Evidence: []string{source.Id.String()}}}
	if got := writtenMove(mv, model, f); got == nil || got.Reason != mv.Reason {
		t.Fatal("the model promoted historical review into a waiting claim")
	}
}

func TestCurrentRequestOutranksTheReviewPageAndCarriesItsOwnClock(t *testing.T) {
	review := inboundMail(testNow.Add(-time.Hour))
	current := inboundMail(testNow.AddDate(0, 0, -200))
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{review}, currentRequests: []crmcontracts.Activity{current}}
	mv := decideMove(f)
	card := composeDeterministic(f, mv)
	if card.ReplyTo == nil || *card.ReplyTo != current.Id || card.RequestBasis == nil || card.RequestBasis.SourceAt != current.OccurredAt {
		t.Fatalf("current request was replaced by review: %+v", card)
	}
	if mv.Basis == nil || mv.Basis.Kind != basisCurrentRequest {
		t.Fatalf("current request lost its basis: %+v", mv.Basis)
	}
}

func TestEvaluationTimeDoesNotChangeCacheOrDismissalIdentity(t *testing.T) {
	source := inboundMail(testNow.Add(-time.Hour))
	source.Version = new(crmcontracts.RowVersion(1))
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{source}}
	first := decideMove(f)
	f.now = f.now.Add(time.Minute)
	second := decideMove(f)
	if moveKey(first) != moveKey(second) || fingerprintOf(first) != fingerprintOf(second) {
		t.Fatal("a fresh evaluation resurrected the same dismissed move or invalidated its cache")
	}
	f.currentRequests = f.requests
	current := decideMove(f)
	if moveKey(first) == moveKey(current) || fingerprintOf(first) != fingerprintOf(current) {
		t.Fatal("classification must invalidate cached advice while preserving the same action dismissal")
	}
	f.currentRequests[0].Version = new(crmcontracts.RowVersion(2))
	revised := decideMove(f)
	if moveKey(revised) == moveKey(current) || fingerprintOf(revised) != fingerprintOf(current) {
		t.Fatal("source revision must invalidate cached wording without undoing the action dismissal")
	}
}

func TestHistoricalRequestCannotGroundAModelBlocker(t *testing.T) {
	in := inputWithTimeline()
	in.RequestStates = map[string]string{"act-2": basisRequestReview}
	got, err := ParseStatus(reply(draft{
		story:    []map[string]any{line("The customer asked for prices in July.", "act-2")},
		buyer:    []map[string]any{line("The buyer is still waiting on an urgent reply.", "act-2")},
		blocker:  []map[string]any{line("The old request is blocking the deal.", "act-2")},
		standing: "blocked",
		because:  []map[string]any{line("The buyer is still waiting.", "act-2")},
	}), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Story) != 1 || len(got.Blocker) != 0 || len(got.Buyer) != 0 || len(got.Verdict.Because) != 0 {
		t.Fatalf("review evidence became current urgency: %+v", got)
	}
}

func TestEveryCacheReaderRejectsAnEarlierProjection(t *testing.T) {
	for _, version := range []string{"", "deal-status-projection-7"} {
		payload, err := json.Marshal(stored{
			ProjectionVersion: version,
			Card: crmcontracts.DealStatusCard{
				Next:    &crmcontracts.DealStatusCardMove{Action: ActionDraftEmail},
				Verdict: &crmcontracts.DealStatusCardVerdict{Standing: "blocked", Because: crmcontracts.DealStatusCardSection{Sentences: []sentence{{Text: "Still waiting"}}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		var decoded stored
		if decodeStored(payload, &decoded) {
			t.Fatalf("main card reader admitted old projection %q", version)
		}
		if _, ok := cardFromPayload(payload); ok {
			t.Fatalf("standing reader admitted old projection %q", version)
		}
	}
}

func TestCoveredHistoricalReviewKeepsItsExistingActionAndDismissal(t *testing.T) {
	source := inboundMail(testNow.AddDate(0, 0, -200))
	source.EmailSummary = &crmcontracts.EmailSummary{RequestHasReminder: new(true)}
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{source}}
	mv := decideMove(f)
	card := composeDeterministic(f, mv)
	if mv.Action != ActionDraftEmail || mv.Basis == nil || mv.Basis.Kind != basisRequestReview || card.ReplyTo != nil {
		t.Fatalf("covered review offered duplicate work or current urgency: %+v", card)
	}
	legacy := mv
	legacy.Basis = nil
	legacy.Fingerprint = nil
	if fingerprintOf(mv) != moveFingerprint(legacy) {
		t.Fatal("review classification resurrected the same covered action")
	}
}

func TestReviewEvidenceCannotRewriteAnotherMovesReason(t *testing.T) {
	source := inboundMail(testNow.AddDate(0, 0, -200))
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{source}}
	mv := stamped(crmcontracts.DealStatusCardMove{Action: ActionOpenTask, Reason: "Complete the agreed task"})
	w := WrittenStatus{MoveReason: WrittenLine{Text: "The buyer has waited 200 days. Reply now.", Evidence: []string{source.Id.String()}}}
	if got := writtenMove(mv, w, f); got == nil || got.Reason != mv.Reason {
		t.Fatal("review evidence became another move's urgency")
	}
}

func TestRepeatedRequestEvidenceUsesTheLatestOwnerRead(t *testing.T) {
	source := inboundMail(testNow.Add(-time.Hour))
	current := source
	current.Subject = new("Revised request")
	f := facts{timeline: []crmcontracts.Activity{source}, requests: []crmcontracts.Activity{source}, currentRequests: []crmcontracts.Activity{current}}
	rows := factActivities(f)
	if len(rows) != 1 || rows[0].Subject == nil || *rows[0].Subject != "Revised request" {
		t.Fatalf("request evidence was repeated or replaced by the earlier timeline read: %+v", rows)
	}
}
