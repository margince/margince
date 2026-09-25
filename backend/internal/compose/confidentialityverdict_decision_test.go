// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

func TestConfidentialityCriteriaNameExactlyTheThreadKinds(t *testing.T) {
	kinds := confidentialityKindNames()
	for _, kind := range kinds {
		if strings.TrimSpace(confidentialityDecisionCriteria[kind]) == "" {
			t.Errorf("kind %q has no criterion, so the decision model can never be told when it is right", kind)
		}
	}
	for label := range confidentialityDecisionCriteria {
		if !slices.Contains(kinds, label) {
			t.Errorf("criterion %q names no thread kind; an answer of it would be refused as off-enum every time", label)
		}
	}
	if got := slices.Sorted(maps.Keys(confidentialityDecisionCriteria)); !slices.Equal(got, kinds) {
		t.Errorf("criteria labels %v, want exactly the thread kinds %v", got, kinds)
	}
}

// confidentialityDecisionThreadOf reads the thread the decision state carries.
func confidentialityDecisionThreadOf(t *testing.T, req decision.Request) confidentialityDecisionThread {
	t.Helper()
	var state confidentialityDecisionState
	if err := json.Unmarshal(req.State, &state); err != nil {
		t.Fatalf("the decision state is not the thread state: %v", err)
	}
	return state.Thread
}

func TestConfidentialityDecisionStateCarriesTheLLMInputs(t *testing.T) {
	const (
		subjectCanary = "qzvSUBJECTcanary"
		firstCanary   = "qzvAufhebungsvertrag.pdf"
		secondCanary  = "qzvGehalt.xlsx"
		bodyCanary    = "qzvBODYcanary"
		// The thread key rides the ledger row and never the LLM prompt, so the
		// state must not carry it either.
		threadKeyCanary = "qzvTHREADKEYcanary"
	)
	row := capture.PendingThread{
		ID: ids.NewV7(), ThreadKey: threadKeyCanary, Subject: subjectCanary,
		Attachments: []string{firstCanary, secondCanary}, Body: bodyCanary,
	}

	prompt := confidentialityRequest(row).Messages[0].Content
	dreq := confidentialityDecision(row)
	for _, canary := range []string{subjectCanary, firstCanary, secondCanary, bodyCanary} {
		if !strings.Contains(prompt, canary) {
			t.Fatalf("the LLM request lost %q, so this test no longer measures its inputs", canary)
		}
		if !strings.Contains(string(dreq.State), canary) {
			t.Errorf("the LLM request carries %q and the decision state does not", canary)
		}
	}
	if strings.Contains(prompt, threadKeyCanary) {
		t.Fatal("the LLM request now carries the thread key; the decision state owes it too")
	}
	if strings.Contains(string(dreq.State), threadKeyCanary) {
		t.Error("the decision state carries the thread key, which the LLM is never sent")
	}
	if got := slices.Sorted(slices.Values(jsonLeafPaths(t, "", dreq.State))); !slices.Equal(got, []string{"thread.attachments", "thread.body", "thread.subject"}) {
		t.Errorf("decision state leaves %v, want exactly thread.subject, thread.attachments and thread.body", got)
	}
	if got := confidentialityDecisionThreadOf(t, dreq).Attachments; !slices.Equal(got, row.Attachments) {
		t.Errorf("attachments %v, want each name the LLM is sent, in order: %v", got, row.Attachments)
	}
}

func TestTheConfidentialityStateListsNoAttachmentsAsAnEmptyList(t *testing.T) {
	dreq := confidentialityDecision(capture.PendingThread{ID: ids.NewV7(), Subject: "Lunch", Body: "Tuesday?"})
	if !strings.Contains(string(dreq.State), `"attachments":[]`) {
		t.Errorf("state %s, want an empty attachments list rather than null or absent", dreq.State)
	}
}

func TestConfidentialityDecisionGateHoldsEveryKindToTheSiteFloor(t *testing.T) {
	// Every kind, not only the opening one: a decision below the floor falls
	// back to the LLM rather than holding on the decision model's word.
	for _, kind := range confidentialityKindNames() {
		if got := confidentialityDecisionGate(decision.Answer{Choice: kind, Confidence: confidentialityFloor - 0.01}); got != ai.DecisionBelowFloor {
			t.Errorf("%s just below the floor = %v, want below the floor", kind, got)
		}
		if got := confidentialityDecisionGate(decision.Answer{Choice: kind, Confidence: confidentialityFloor}); got != ai.DecisionAccepted {
			t.Errorf("%s at the floor = %v, want accepted", kind, got)
		}
	}
	if got := confidentialityDecisionGate(decision.Answer{Choice: capture.VerdictUnsure, Confidence: 0.99}); got != ai.DecisionOffEnum {
		t.Errorf("an unanswerable label = %v, want off-enum however confident", got)
	}
}

func TestTheConfidentialityCaseReportsTheFloorsItsGateApplies(t *testing.T) {
	c := &confidentialityCase{}
	floors := c.Floors()
	if got := slices.Sorted(maps.Keys(floors)); !slices.Equal(got, confidentialityKindNames()) {
		t.Fatalf("floors name %v, want every thread kind", got)
	}
	for kind, floor := range floors {
		if c.GateDecision(decision.Answer{Choice: kind, Confidence: floor}) != ai.DecisionAccepted ||
			c.GateDecision(decision.Answer{Choice: kind, Confidence: floor - 0.01}) != ai.DecisionBelowFloor {
			t.Errorf("%s: the case reports floor %.2f and the gate applies another", kind, floor)
		}
	}
}

func TestTheConfidentialityAskTakesAConfidentDecision(t *testing.T) {
	row := capture.PendingThread{ID: ids.NewV7(), ActivityID: ids.NewV7(), Subject: "Kündigung"}
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: confidentialityPersonnel, Confidence: 0.93},
		llmReply: verdictReply(row.ID.String(), confidentialityOrdinary),
	}

	results, err := (&ConfidentialityVerdictEngine{brain: lane}).ask(context.Background(), row)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.llmCalled {
		t.Error("the LLM was asked although the decision stood")
	}
	if lane.askedSite != confidentialityDecisionSite {
		t.Errorf("asked at site %q, want %q", lane.askedSite, confidentialityDecisionSite)
	}
	want := confidentialityResult{ID: row.ID.String(), Verdict: confidentialityPersonnel, Confidence: 0.93}
	if len(results) != 1 || results[0] != want {
		t.Errorf("results %+v, want the decision's personnel about the asked thread", results)
	}
}

func TestTheConfidentialityAskFallsBackBelowTheFloor(t *testing.T) {
	row := capture.PendingThread{ID: ids.NewV7(), ActivityID: ids.NewV7(), Subject: "Lunch"}
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: confidentialityLegal, Confidence: confidentialityFloor - 0.01},
		llmReply: verdictReply(row.ID.String(), confidentialityOrdinary),
	}

	results, err := (&ConfidentialityVerdictEngine{brain: lane}).ask(context.Background(), row)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if !lane.llmCalled {
		t.Error("an unsure decision was kept instead of falling back to the LLM")
	}
	if len(results) != 1 || results[0].Verdict != confidentialityOrdinary {
		t.Errorf("results %+v, want the LLM's own ordinary answer", results)
	}
}
