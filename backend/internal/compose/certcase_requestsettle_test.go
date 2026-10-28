// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// The thing owed may be named in more than one way, and each names it.
//
// The check was one lowered substring. An Angebot asked for in a German thread
// came back as "Angebot für die zweite Charge schicken" from one model, as
// "Send offer for the second batch" from another and as "Send the quote" from a
// third — three honest names for one document — and whichever single token the
// scenario pinned marked the other two wrong.
//
// This is not the "accept either finding" relaxation that account_scan did NOT
// want: there the two answers were different FINDINGS and picking between them
// was the thing under test. Here they are the same finding under different
// words.
func TestRemainingMayBeNamedInSeveralRenderings(t *testing.T) {
	t.Parallel()
	const accepted = "angebot|offer|quote"

	for name, tc := range map[string]struct {
		remaining string
		want      bool
	}{
		"the German noun":        {"Angebot für die zweite Charge schicken", true},
		"an English translation": {"Send offer for the second batch", true},
		"the other translation":  {"Send the quote", true},
		"case does not matter":   {"SEND THE QUOTE", true},
		"empty names nothing":    {"", false},
		"a different thing":      {"Confirm the November dates", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := remainingNamesIt(tc.remaining, accepted); got != tc.want {
				t.Errorf("remainingNamesIt(%q, %q) = %v, want %v", tc.remaining, accepted, got, tc.want)
			}
		})
	}
}

// A single rendering still means that one, so the loosening reaches only the
// scenarios that ask for it.
func TestASingleRenderingIsStillDemanded(t *testing.T) {
	t.Parallel()
	if remainingNamesIt("Send the quote", "processing") {
		t.Error("a phrase naming something else satisfied a single-rendering expectation")
	}
	if !remainingNamesIt("Send the data-processing agreement", "processing") {
		t.Error("a phrase naming the expected thing was refused")
	}
}

// A rendering names the document, not a verb that shares a word with it.
//
// `remainingNamesIt` is substring matching, so a bare "offer" is satisfied by
// "Offer a meeting next week" — a phrase naming something this conversation
// never owed. The renderings are written to identify the thing.
func TestARenderingDoesNotMatchAnUnrelatedUseOfTheWord(t *testing.T) {
	t.Parallel()
	const accepted = "angebot|offer for|the offer|an offer|quote"

	for name, tc := range map[string]struct {
		remaining string
		want      bool
	}{
		"the German noun":        {"Angebot für die zweite Charge schicken", true},
		"an English rendering":   {"Send offer for the second batch", true},
		"the other rendering":    {"Send the quote", true},
		"an unrelated offer":     {"Offer a meeting next week", false},
		"offering to help":       {"Offer support during onboarding", false},
		"a different obligation": {"Confirm the November dates", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := remainingNamesIt(tc.remaining, accepted); got != tc.want {
				t.Errorf("remainingNamesIt(%q) = %v, want %v", tc.remaining, got, tc.want)
			}
		})
	}
}

// The certification lane checks the remaining quote against the request the
// way production does, so the first message must carry the request's id.
func TestACertificationCaseChecksTheQuoteAgainstItsRequest(t *testing.T) {
	t.Parallel()
	prepared, err := (requestSettleCases{}).Prepare(
		json.RawMessage(`[{"messages":[{"direction":"inbound","body":"Please send the report."},{"direction":"outbound","body":"I will send it tomorrow."}]}]`),
		json.RawMessage(`[{"verdict":"still_owed","remaining_names":"report"}]`))
	if err != nil {
		t.Fatal(err)
	}
	settleCase, ok := prepared.(*requestSettleCase)
	if !ok {
		t.Fatalf("Prepare returned %T", prepared)
	}
	result := settleResult{ID: settleCase.batch[0].Request.RequestID.String(), Verdict: activities.RequestStillOwed, Remaining: "send the report", Confidence: 1}
	if msg := validateSettlePayload(settlePayload{Results: []settleResult{result}}, settleCase.batch); msg != "" {
		t.Fatalf("a quote of the request was refused: %s", msg)
	}
}

// A scenario cannot expect a word their request never used: the answer quotes
// the request, so no model could satisfy it.
func TestAScenarioCannotExpectAWordTheRequestNeverUsed(t *testing.T) {
	t.Parallel()
	_, err := (requestSettleCases{}).Prepare(
		json.RawMessage(`[{"messages":[{"direction":"inbound","body":"Bitte schick die Rechnung."},{"direction":"outbound","body":"Kommt später."}]}]`),
		json.RawMessage(`[{"verdict":"still_owed","remaining_names":"invoice"}]`))
	if err == nil || !strings.Contains(err.Error(), "no rendering appears") {
		t.Fatalf("Prepare = %v, want a refusal of an expectation the request cannot meet", err)
	}
}
