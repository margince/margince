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
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

func TestCounterpartyCriteriaNameExactlyTheVerdictKinds(t *testing.T) {
	kinds := verdictKindNames()
	for _, kind := range kinds {
		if strings.TrimSpace(counterpartyDecisionCriteria[kind]) == "" {
			t.Errorf("kind %q has no criterion, so the decision model can never be told when it is right", kind)
		}
	}
	for label := range counterpartyDecisionCriteria {
		if !slices.Contains(kinds, label) {
			t.Errorf("criterion %q names no sender kind; an answer of it would be refused as off-enum every time", label)
		}
	}
	if got := slices.Sorted(maps.Keys(counterpartyDecisionCriteria)); !slices.Equal(got, kinds) {
		t.Errorf("criteria labels %v, want exactly the sender kinds %v", got, kinds)
	}
}

// counterpartyDecisionStateOf reads the state the decision request carries.
func counterpartyDecisionStateOf(t *testing.T, req decision.Request) counterpartyDecisionState {
	t.Helper()
	var state counterpartyDecisionState
	if err := json.Unmarshal(req.State, &state); err != nil {
		t.Fatalf("the decision state is not the sender state: %v", err)
	}
	return state
}

func TestCounterpartyDecisionStateCarriesTheLLMInputs(t *testing.T) {
	const (
		nameCanary    = "qzvNAMEcanary"
		emailCanary   = "qzvemailcanary@prospect.example"
		subjectCanary = "qzvSUBJECTcanary"
		bodyCanary    = "qzvBODYcanary"
		// The domain rides the ledger row and never the LLM prompt, so the state
		// must not carry it either: a decision that saw it would certify a
		// judgment the fallback cannot reproduce.
		domainCanary = "qzvdomaincanary.example"
	)
	base := []string{"message.body", "message.subject", "sender.display_name", "sender.email"}
	cases := []struct {
		name      string
		direction string
		wroteBack bool
		// llmSays is how addressLine tells the model the same fact.
		llmSays       string
		wantDirection string
		wantWroteBack *bool
		wantLeaves    []string
	}{
		{
			name: "outbound and answered", direction: connector.DirectionOutbound, wroteBack: true,
			llmSays: "this address has written back", wantDirection: counterpartyDirectionOutbound,
			wantWroteBack: new(true), wantLeaves: append(slices.Clone(base), "message.direction", "message.wrote_back"),
		},
		{
			name: "outbound and never answered", direction: connector.DirectionOutbound,
			llmSays: "this address has never written back", wantDirection: counterpartyDirectionOutbound,
			wantWroteBack: new(false), wantLeaves: append(slices.Clone(base), "message.direction", "message.wrote_back"),
		},
		{
			// WroteBack set on an inbound row is still not said: addressLine says
			// it only of an address the owner wrote to.
			name: "inbound", direction: connector.DirectionInbound, wroteBack: true,
			llmSays: "this address wrote to the mailbox owner", wantDirection: counterpartyDirectionInbound,
			wantLeaves: append(slices.Clone(base), "message.direction"),
		},
		{
			name: "direction never recorded", llmSays: "Correspondent:", wantLeaves: base,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := capture.PendingCounterparty{
				ID: ids.NewV7(), DisplayName: nameCanary, Email: emailCanary, Domain: domainCanary,
				Subject: subjectCanary, Body: bodyCanary, Direction: tc.direction, WroteBack: tc.wroteBack,
			}
			prompt := verdictRequest(row).Messages[0].Content
			dreq := counterpartyDecision(row)
			for _, canary := range []string{nameCanary, emailCanary, subjectCanary, bodyCanary} {
				if !strings.Contains(prompt, canary) {
					t.Fatalf("the LLM request lost %q, so this test no longer measures its inputs", canary)
				}
				if !strings.Contains(string(dreq.State), canary) {
					t.Errorf("the LLM request carries %q and the decision state does not", canary)
				}
			}
			if strings.Contains(prompt, domainCanary) {
				t.Fatal("the LLM request now carries the domain; the decision state owes it too")
			}
			if strings.Contains(string(dreq.State), domainCanary) {
				t.Error("the decision state carries the domain, which the LLM is never sent")
			}
			if !strings.Contains(prompt, tc.llmSays) {
				t.Fatalf("the LLM request no longer says %q, so this case no longer mirrors it:\n%s", tc.llmSays, prompt)
			}
			if got := slices.Sorted(slices.Values(jsonLeafPaths(t, "", dreq.State))); !slices.Equal(got, slices.Sorted(slices.Values(tc.wantLeaves))) {
				t.Errorf("decision state leaves %v, want exactly %v", got, tc.wantLeaves)
			}
			message := counterpartyDecisionStateOf(t, dreq).Message
			if message.Direction != tc.wantDirection {
				t.Errorf("direction %q, want %q", message.Direction, tc.wantDirection)
			}
			switch {
			case tc.wantWroteBack == nil && message.WroteBack != nil:
				t.Errorf("wrote_back = %v, but the LLM is told nothing about answering back here", *message.WroteBack)
			case tc.wantWroteBack != nil && (message.WroteBack == nil || *message.WroteBack != *tc.wantWroteBack):
				t.Errorf("wrote_back = %v, want %v as the LLM is told", message.WroteBack, *tc.wantWroteBack)
			}
		})
	}
}

func TestCounterpartyDecisionGateUsesEachKindsFloor(t *testing.T) {
	for _, kind := range verdictKindNames() {
		floor := verdictFloorFor(kind)
		if got := counterpartyDecisionGate(decision.Answer{Choice: kind, Confidence: floor - 0.01}); got != ai.DecisionBelowFloor {
			t.Errorf("%s just below its floor %.2f = %v, want below the floor", kind, floor, got)
		}
		if got := counterpartyDecisionGate(decision.Answer{Choice: kind, Confidence: floor}); got != ai.DecisionAccepted {
			t.Errorf("%s at its floor %.2f = %v, want accepted", kind, floor, got)
		}
	}
	// The asymmetry is the point: a creating kind needs more than one that
	// creates nothing, at the same confidence.
	between := (verdictConfidenceFloor + verdictCreateFloor) / 2
	if got := counterpartyDecisionGate(decision.Answer{Choice: capture.KindContact, Confidence: between}); got != ai.DecisionBelowFloor {
		t.Errorf("a contact between the two floors = %v, want below the floor", got)
	}
	if got := counterpartyDecisionGate(decision.Answer{Choice: capture.KindSpam, Confidence: between}); got != ai.DecisionAccepted {
		t.Errorf("spam between the two floors = %v, want accepted", got)
	}
	if got := counterpartyDecisionGate(decision.Answer{Choice: capture.PendingStatusUnsure, Confidence: 0.99}); got != ai.DecisionOffEnum {
		t.Errorf("an unanswerable label = %v, want off-enum however confident", got)
	}
}

func TestTheCounterpartyCaseReportsTheFloorsItsGateApplies(t *testing.T) {
	c := &counterpartyVerdictCase{}
	floors := c.Floors()
	if got := slices.Sorted(maps.Keys(floors)); !slices.Equal(got, verdictKindNames()) {
		t.Fatalf("floors name %v, want every sender kind", got)
	}
	for kind, floor := range floors {
		if c.GateDecision(decision.Answer{Choice: kind, Confidence: floor}) != ai.DecisionAccepted ||
			c.GateDecision(decision.Answer{Choice: kind, Confidence: floor - 0.01}) != ai.DecisionBelowFloor {
			t.Errorf("%s: the case reports floor %.2f and the gate applies another", kind, floor)
		}
	}
}

func TestTheCounterpartyAskTakesAConfidentDecision(t *testing.T) {
	row := capture.PendingCounterparty{ID: ids.NewV7(), Email: "k.bauer@kanzlei.example", Direction: connector.DirectionInbound}
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: capture.KindAdvisor, Confidence: verdictCreateFloor},
		llmReply: verdictReply(row.ID.String(), capture.KindSpam),
	}

	results, _, err := (&CounterpartyVerdictEngine{brain: lane}).ask(context.Background(), row)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.llmCalled {
		t.Error("the LLM was asked although the decision stood")
	}
	if lane.askedSite != counterpartyDecisionSite {
		t.Errorf("asked at site %q, want %q", lane.askedSite, counterpartyDecisionSite)
	}
	want := verdictResult{ID: row.ID.String(), Verdict: capture.KindAdvisor, Confidence: verdictCreateFloor}
	if len(results) != 1 || results[0] != want {
		t.Errorf("results %+v, want the decision's advisor about the asked row", results)
	}
	if !clearsItsFloor(results[0]) {
		t.Error("a decision the gate kept does not clear the engine's own floor")
	}
}

func TestTheCounterpartyAskFallsBackBelowTheFloor(t *testing.T) {
	row := capture.PendingCounterparty{ID: ids.NewV7(), Email: "k.bauer@kanzlei.example"}
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: capture.KindContact, Confidence: verdictCreateFloor - 0.01},
		llmReply: verdictReply(row.ID.String(), capture.KindSpam),
	}

	results, _, err := (&CounterpartyVerdictEngine{brain: lane}).ask(context.Background(), row)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if !lane.llmCalled {
		t.Error("an unsure decision was kept instead of falling back to the LLM")
	}
	if len(results) != 1 || results[0].Verdict != capture.KindSpam {
		t.Errorf("results %+v, want the LLM's own spam answer", results)
	}
}
