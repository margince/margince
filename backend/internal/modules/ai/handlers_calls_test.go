// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWireAiCallDetailMapsPayloadAndAttempts(t *testing.T) {
	sentinel := "provider_unavailable"
	detail := CallDetail{
		CallSummary: CallSummary{
			ID: ids.NewV7(), OccurredAt: time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
			Task: "capture_classify", Tier: "cheap_cloud", Provider: "gemini",
			ModelID: "gemini-2.5-flash", ServedModel: "gemini-2.5-flash",
			Attempt: 2, TokensIn: 100, TokensOut: 20, LatencyMS: 900,
			ErrorSentinel: &sentinel, HasPayload: true,
		},
		ServedIdentitySource: "configured",
		ContextScopes:        []string{"identity"},
		ContextFingerprint:   "abc",
		Attempts: []CallAttempt{
			{Attempt: 1, TokensIn: 100, OccurredAt: time.Date(2026, 7, 20, 9, 59, 0, 0, time.UTC)},
			{
				Attempt: 2, IsTerminal: true, AttemptReason: "retry_on_5xx", TokensIn: 100,
				TokensOut: 20, LatencyMS: 900,
				OccurredAt: time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
			},
		},
		Payload: &Payload{
			Request:  json.RawMessage(`{"system":"s","messages":[]}`),
			Response: json.RawMessage(`"ok"`),
		},
	}
	wire := wireAiCall(detail)
	if !wire.PayloadCaptured || wire.Payload == nil {
		t.Fatalf("payload_captured/payload not mapped: %+v", wire)
	}
	if len(wire.Attempts) != 2 || !wire.Attempts[1].IsTerminal {
		t.Fatalf("attempt ladder not mapped: %+v", wire.Attempts)
	}
	if wire.ErrorSentinel == nil || *wire.ErrorSentinel != sentinel {
		t.Fatal("error_sentinel not mapped")
	}
	if wire.CallsAttempted != 2 {
		t.Fatalf("calls_attempted = %d, want 2", wire.CallsAttempted)
	}
}

func TestWireAiCallSummaryCarriesPayloadPresenceOnly(t *testing.T) {
	wire := wireAiCallSummary(CallSummary{ID: ids.NewV7(), Task: "enrich", HasPayload: true})
	if !wire.HasPayload {
		t.Fatal("has_payload lost in mapping")
	}
}

// The list row and the detail header both read decision_attempted, so each
// wire carries it.
func TestBothCallWiresSayADecisionModelWasAsked(t *testing.T) {
	summary := CallSummary{ID: ids.NewV7(), Task: "site_triage", Kind: callKindCompletion, DecisionAttempted: true}
	if !wireAiCallSummary(summary).DecisionAttempted {
		t.Error("the summary wire lost decision_attempted")
	}
	if !wireAiCall(CallDetail{CallSummary: summary}).DecisionAttempted {
		t.Error("the detail wire lost decision_attempted")
	}
}

// A logical call that asked a decision model and then a chat tier shows both
// rungs for what they were; a rung that reached no tier omits it rather than
// sending an empty string.
func TestWireAiCallNamesEachAttemptsKindAndBinding(t *testing.T) {
	wire := wireAiCall(CallDetail{
		CallSummary: CallSummary{ID: ids.NewV7(), Task: "site_triage", Kind: callKindCompletion, Tier: "cheap_cloud"},
		Attempts: []CallAttempt{
			{Attempt: 1, Kind: callKindDecision, Tier: string(TierDecideLane), Provider: providerJevCompatible, ModelID: "typesafe/jev-1.13"},
			{Attempt: 2, IsTerminal: true, Kind: callKindCompletion, AttemptReason: attemptReasonDecisionBelowFloor},
		},
	})
	if wire.Kind != callKindCompletion {
		t.Errorf("kind = %q", wire.Kind)
	}
	first, second := wire.Attempts[0], wire.Attempts[1]
	if first.Kind != callKindDecision || first.Tier == nil || *first.Tier != "decide" ||
		first.Provider == nil || *first.Provider != providerJevCompatible || first.ModelId == nil || *first.ModelId != "typesafe/jev-1.13" {
		t.Errorf("decision attempt = %+v", first)
	}
	if second.Kind != callKindCompletion || second.Tier != nil || second.Provider != nil || second.ModelId != nil {
		t.Errorf("an attempt with no binding sent one: %+v", second)
	}
}

// A reader of one call sees which model each attempt was served by, and what a
// decision attempt answered even when the ladder answered after it.
func TestWireAiCallCarriesEachAttemptsServedIdentityAndAnswer(t *testing.T) {
	logical := ids.NewV7()
	sentinel := "provider_error"
	wire := wireAiCall(CallDetail{
		CallSummary:   CallSummary{ID: ids.NewV7(), Task: "site_triage", Kind: callKindCompletion},
		LogicalCallID: logical,
		Attempts: []CallAttempt{
			{
				Attempt: 1, Kind: callKindDecision, ServedModel: "typesafe/jev-1.13-20260917", ServedProvider: "TypeSafe",
				DecisionAnswer: &DecisionAnswer{Choice: "company", Confidence: 0.62},
			},
			{Attempt: 2, IsTerminal: true, Kind: callKindCompletion, ErrorSentinel: &sentinel},
		},
	})
	if wire.LogicalCallId != openapi_types.UUID(logical) {
		t.Errorf("logical_call_id = %v, want %v", wire.LogicalCallId, logical)
	}
	decided, completed := wire.Attempts[0], wire.Attempts[1]
	if decided.ServedModel == nil || *decided.ServedModel != "typesafe/jev-1.13-20260917" ||
		decided.ServedProvider == nil || *decided.ServedProvider != "TypeSafe" {
		t.Errorf("decision attempt served = %v / %v", decided.ServedModel, decided.ServedProvider)
	}
	if decided.DecisionChoice == nil || *decided.DecisionChoice != "company" ||
		decided.DecisionConfidence == nil || *decided.DecisionConfidence != 0.62 {
		t.Errorf("decision attempt answer = %v at %v, want company at 0.62", decided.DecisionChoice, decided.DecisionConfidence)
	}
	if completed.ServedModel != nil || completed.ServedProvider != nil ||
		completed.DecisionChoice != nil || completed.DecisionConfidence != nil {
		t.Errorf("an attempt with nothing reported sent something: %+v", completed)
	}
	if completed.ErrorSentinel == nil || *completed.ErrorSentinel != sentinel {
		t.Errorf("error_sentinel = %v, want %s", completed.ErrorSentinel, sentinel)
	}
}
